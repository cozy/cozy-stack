package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo"
)

const defaultLimit = 50

// A Version describes a specific release of an application.
type Version struct {
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	URL       string          `json:"url"`
	Sha256    string          `json:"sha256"`
	CreatedAt time.Time       `json:"created_at"`
	Size      string          `json:"size"`
	Manifest  json.RawMessage `json:"manifest"`
	TarPrefix string          `json:"tar_prefix"`
}

var errVersionNotFound = errors.New("Version not found")

var proxyClient = &http.Client{
	Timeout: 10 * time.Second,
}

// GetLatestVersion returns the latest version available from the list of
// registries by resolving them in sequence using the specified application
// name and channel name.
func GetLatestVersion(appName, channel string, registries []*url.URL) (*Version, error) {
	requestURI := fmt.Sprintf("/registry/%s/%s/latest",
		url.PathEscape(appName),
		url.PathEscape(channel))
	rc, ok, err := fetchUntilFound(registries, requestURI)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errVersionNotFound
	}
	defer rc.Close()
	var v *Version
	if err = json.NewDecoder(rc).Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// Proxy will proxy the given request to the registries in sequence and return
// the response as io.ReadCloser when finding a registry returning a HTTP 200OK
// response.
func Proxy(req *http.Request, registries []*url.URL) (io.ReadCloser, error) {
	rc, ok, err := fetchUntilFound(registries, req.RequestURI)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, echo.NewHTTPError(http.StatusNotFound)
	}
	return rc, nil
}

type jsonObject map[string]interface{}

type appsList struct {
	ref        *url.URL
	list       []jsonObject
	registries []*registryFetchState
	names      map[string]struct {
		*registryFetchState
		int
	}
	limit int
}

type pageInfo struct {
	Count      int    `json:"count"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type appsPaginated struct {
	List     []jsonObject `json:"data"`
	PageInfo pageInfo     `json:"meta"`
}

type registryFetchState struct {
	url    *url.URL
	index  int // index in the registries array
	cursor int // cursor used to fetch the registry
	ended  int // cursor of the last element in the regitry (-1 if unknown)
}

func newAppsList(ref *url.URL, registries []*url.URL, cursors []int, limit int) *appsList {
	if len(registries) != len(cursors) {
		panic("should have same length")
	}
	regStates := make([]*registryFetchState, len(registries))
	for i := range regStates {
		regStates[i] = &registryFetchState{
			index:  i,
			url:    registries[i],
			cursor: cursors[i],
			ended:  -1,
		}
	}
	return &appsList{
		ref:   ref,
		limit: limit,
		list:  make([]jsonObject, 0),
		names: make(map[string]struct {
			*registryFetchState
			int
		}),
		registries: regStates,
	}
}

func (a *appsList) FetchAll() error {
	for _, r := range a.registries {
		if err := a.fetch(r); err != nil {
			return err
		}
	}
	return nil
}

func (a *appsList) fetch(r *registryFetchState) error {
	limit := a.limit

	cursor := r.cursor
	// A negative dimension of the cursor means we already reached the end of the
	// list. There is no need to fetch anymore in that case.
	if cursor < 0 {
		return nil
	}

	for {
		ref := addQueries(removeQueries(a.ref, "cursor", "limit"),
			"cursor", strconv.Itoa(cursor),
			"limit", strconv.Itoa(limit),
		)
		rc, ok, err := fetch(r.url, ref)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		defer rc.Close()
		var resp appsPaginated
		if err = json.NewDecoder(rc).Decode(&resp); err != nil {
			return err
		}
		added := 0
		for i, obj := range resp.List {
			name := obj["name"].(string)
			if _, ok := a.names[name]; ok {
				continue
			}
			a.list = append(a.list, obj)
			a.names[name] = struct {
				*registryFetchState
				int
			}{r, cursor + i}
			added++
		}
		if added >= limit {
			break
		}
		nextCursor := resp.PageInfo.NextCursor
		if len(resp.List) < limit || nextCursor == "" {
			r.ended = cursor + len(resp.List)
			break
		}
		cursor, _ = strconv.Atoi(nextCursor)
		limit -= added
	}

	return nil
}

func (a *appsList) Paginated(sortBy string, reverse bool, limit int) *appsPaginated {
	sort.Slice(a.list, func(i, j int) bool {
		vi := a.list[i]
		vj := a.list[j]
		var less bool
		switch valA := vi[sortBy].(type) {
		case string:
			valB := vj[sortBy].(string)
			less = valA < valB
		case int:
			valB := vj[sortBy].(int)
			less = valA < valB
		}
		if reverse {
			return !less
		}
		return less
	})

	if limit > len(a.list) {
		limit = len(a.list)
	}

	list := a.list[:limit]

	// Calculation of the next multi-cursor by iterating through the sorted and
	// truncated list and incrementing the dimension of the multi-cursor
	// associated with the objects registry.
	//
	// In the end, we also check if the end value of each dimensions of the
	// cursor reached the end of the list. If so, the dimension is set to -1.
	cursors := make([]int, len(a.registries))
	for i, reg := range a.registries {
		cursors[i] = reg.cursor
	}
	for _, o := range list {
		name := o["name"].(string)
		reg := a.names[name]
		cursors[reg.index] = reg.int + 1
	}
	for i, reg := range a.registries {
		if e := reg.ended; e >= 0 && cursors[i] >= e {
			cursors[i] = -1
		}
	}

	return &appsPaginated{
		List: list,
		PageInfo: pageInfo{
			Count:      len(list),
			NextCursor: printMutliCursor(cursors),
		},
	}
}

// ProxyList will proxy the given request to the registries by aggregating the
// results along the way. It should be used for list endpoints.
func ProxyList(req *http.Request, registries []*url.URL) (json.RawMessage, error) {
	ref, err := url.Parse(req.RequestURI)
	if err != nil {
		return nil, err
	}

	var sortBy string
	var sortReverse bool
	var limit int

	cursors := make([]int, len(registries))

	q := ref.Query()
	if v, ok := q["cursor"]; ok {
		splits := strings.Split(v[0], "|")
		for i, s := range splits {
			if i >= len(registries) {
				break
			}
			cursors[i], _ = strconv.Atoi(s)
		}
	}
	if v, ok := q["sort"]; ok {
		sortBy = v[0]
	}
	if len(sortBy) > 0 && sortBy[0] == '-' {
		sortReverse = true
		sortBy = sortBy[1:]
	}
	if sortBy == "" {
		sortBy = "name"
	}
	if v, ok := q["limit"]; ok {
		limit, _ = strconv.Atoi(v[0])
	}
	if limit <= 0 {
		limit = defaultLimit
	}

	list := newAppsList(ref, registries, cursors, limit)
	if err := list.FetchAll(); err != nil {
		return nil, err
	}
	return json.Marshal(list.Paginated(sortBy, sortReverse, limit))
}

func fetchUntilFound(registries []*url.URL, requestURI string) (rc io.ReadCloser, ok bool, err error) {
	ref, err := url.Parse(requestURI)
	if err != nil {
		return
	}
	for _, registry := range registries {
		rc, ok, err = fetch(registry, ref)
		if err != nil {
			return
		}
		if !ok {
			continue
		}
		return
	}
	return nil, false, nil
}

func fetch(registry, ref *url.URL) (rc io.ReadCloser, ok bool, err error) {
	u := registry.ResolveReference(ref)
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return
	}
	resp, err := proxyClient.Do(req)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			resp.Body.Close()
		}
	}()
	if resp.StatusCode == 404 {
		return
	}
	if resp.StatusCode != 200 {
		var msg struct {
			Message string `json:"message"`
		}
		if err = json.NewDecoder(resp.Body).Decode(&msg); err != nil {
			err = echo.NewHTTPError(resp.StatusCode)
		} else {
			err = echo.NewHTTPError(resp.StatusCode, msg.Message)
		}
		return
	}
	return resp.Body, true, nil
}

func printMutliCursor(c []int) string {
	// if all dimensions of the multi-cursor are -1, we print the empty string
	sum := 0
	for _, i := range c {
		sum += i
	}
	if sum == -len(c) {
		return ""
	}
	var a []string
	for _, i := range c {
		a = append(a, strconv.Itoa(i))
	}
	return strings.Join(a, "|")
}

func removeQueries(u *url.URL, filter ...string) *url.URL {
	u, _ = url.Parse(u.String())
	q1 := u.Query()
	q2 := make(url.Values)
	for k, v := range q1 {
		if len(v) == 0 {
			continue
		}
		var remove bool
		for _, f := range filter {
			if f == k {
				remove = true
				break
			}
		}
		if !remove {
			q2.Add(k, v[0])
		}
	}
	u.RawQuery = q2.Encode()
	return u
}

func addQueries(u *url.URL, queries ...string) *url.URL {
	u, _ = url.Parse(u.String())
	q := u.Query()
	for i := 0; i < len(queries); i += 2 {
		q.Add(queries[i], queries[i+1])
	}
	u.RawQuery = q.Encode()
	return u
}
