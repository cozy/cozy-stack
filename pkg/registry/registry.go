package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/echo"
	"github.com/cozy/httpcache"
)

const defaultLimit = 100

// A Version describes a specific release of an application.
type Version struct {
	Slug      string          `json:"slug"`
	Version   string          `json:"version"`
	URL       string          `json:"url"`
	Sha256    string          `json:"sha256"`
	CreatedAt time.Time       `json:"created_at"`
	Size      string          `json:"size"`
	Manifest  json.RawMessage `json:"manifest"`
	TarPrefix string          `json:"tar_prefix"`
}

// A MaintenanceOptions defines options about a maintenance
type MaintenanceOptions struct {
	FlagInfraMaintenance   bool `json:"flag_infra_maintenance"`
	FlagShortMaintenance   bool `json:"flag_short_maintenance"`
	FlagDisallowManualExec bool `json:"flag_disallow_manual_exec"`
}

// An Application describe an application on the registry
type Application struct {
	Slug                 string             `json:"slug"`
	Type                 string             `json:"type"`
	MaintenanceActivated bool               `json:"maintenance_activated,omitempty"`
	MaintenanceOptions   MaintenanceOptions `json:"maintenance_options"`
}

var errVersionNotFound = errors.New("registry: version not found")
var errApplicationNotFound = errors.New("registry: application not found")

var (
	proxyClient = &http.Client{
		Timeout:   10 * time.Second,
		Transport: httpcache.NewMemoryCacheTransport(32),
	}

	appClient = &http.Client{
		Timeout:   5 * time.Second,
		Transport: httpcache.NewMemoryCacheTransport(256),
	}

	latestVersionClient = &http.Client{
		Timeout:   5 * time.Second,
		Transport: httpcache.NewMemoryCacheTransport(256),
	}
)

// CacheControl defines whether or not to use caching for the request made to
// the registries.
type CacheControl int

const (
	// WithCache specify caching
	WithCache CacheControl = iota
	// NoCache disables any caching
	NoCache
)

// GetVersion returns a specific version from a slug name
func GetVersion(slug, version string, registries []*url.URL) (*Version, error) {
	requestURI := fmt.Sprintf("/registry/%s/%s",
		url.PathEscape(slug),
		url.PathEscape(version))
	resp, ok, err := fetchUntilFound(latestVersionClient, registries, requestURI, WithCache)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errVersionNotFound
	}
	defer resp.Body.Close()
	var v *Version
	if err = json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// GetLatestVersion returns the latest version available from the list of
// registries by resolving them in sequence using the specified application
// slug and channel name.
func GetLatestVersion(slug, channel string, registries []*url.URL) (*Version, error) {
	requestURI := fmt.Sprintf("/registry/%s/%s/latest",
		url.PathEscape(slug),
		url.PathEscape(channel))
	resp, ok, err := fetchUntilFound(latestVersionClient, registries, requestURI, WithCache)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errVersionNotFound
	}
	defer resp.Body.Close()
	var v *Version
	if err = json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// GetApplication returns an application from his slug
func GetApplication(slug string, registries []*url.URL) (*Application, error) {
	requestURI := fmt.Sprintf("/registry/%s/", slug)
	resp, ok, err := fetchUntilFound(appClient, registries, requestURI, WithCache)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errApplicationNotFound
	}
	defer resp.Body.Close()
	var app *Application
	if err = json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return nil, err
	}
	return app, nil
}

// Proxy will proxy the given request to the registries in sequence and return
// the response as io.ReadCloser when finding a registry returning a HTTP 200OK
// response.
func Proxy(req *http.Request, registries []*url.URL, cache CacheControl) (*http.Response, error) {
	resp, ok, err := fetchUntilFound(proxyClient, registries, req.RequestURI, cache)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, echo.NewHTTPError(http.StatusNotFound)
	}
	return resp, nil
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
		sortBy = "slug"
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

type jsonObject map[string]interface{}

type appsList struct {
	ref        *url.URL
	list       []jsonObject
	registries []*registryFetchState
	slugs      map[string][]int
	limit      int
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
		ref:        ref,
		limit:      limit,
		list:       make([]jsonObject, 0),
		slugs:      make(map[string][]int),
		registries: regStates,
	}
}

func (a *appsList) FetchAll() error {
	l := len(a.registries)
	for i, r := range a.registries {
		// We fetch the entire registry except for the last one. In practice, the
		// "high-priority" registries should be small and the last one contain the
		// vast majority of the applications.
		fetchAll := i < l-1
		if err := a.fetch(r, fetchAll); err != nil {
			return err
		}
	}
	return nil
}

func (a *appsList) fetch(r *registryFetchState, fetchAll bool) error {
	slugs := a.slugs
	minCursor := r.cursor
	maxCursor := r.cursor + a.limit

	var cursor, limit int
	if fetchAll {
		cursor = 0
		limit = defaultLimit
	} else {
		cursor = r.cursor
		limit = a.limit
	}

	// A negative dimension of the cursor means we already reached the end of the
	// list. There is no need to fetch anymore in that case.
	if !fetchAll && r.cursor < 0 {
		return nil
	}

	added := 0
	for {
		ref := addQueries(removeQueries(a.ref, "cursor", "limit"),
			"cursor", strconv.Itoa(cursor),
			"limit", strconv.Itoa(limit),
		)
		resp, ok, err := fetch(proxyClient, r.url, ref, NoCache)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		defer resp.Body.Close()
		var page appsPaginated
		if err = json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return err
		}

		for i, obj := range page.List {
			objCursor := cursor + i

			objInRange := r.cursor >= 0 &&
				objCursor >= minCursor &&
				objCursor <= maxCursor

			// if an object with same slug has already been fetched, we skip it
			slug := obj["slug"].(string)
			offsets, ok := slugs[slug]
			if !ok {
				offsets = make([]int, len(a.registries))
				slugs[slug] = offsets
			}
			if objInRange {
				offsets[r.index] = objCursor + 1
				if !ok {
					a.list = append(a.list, obj)
					added++
				}
			}
		}

		nextCursor := page.PageInfo.NextCursor
		if nextCursor == "" {
			r.ended = cursor + len(page.List)
			break
		}

		cursor, _ = strconv.Atoi(nextCursor)
		if !fetchAll && limit-added <= 0 {
			break
		}
	}

	return nil
}

func (a *appsList) Paginated(sortBy string, reverse bool, limit int) *appsPaginated {
	sortBySlug := sortBy == "slug"
	sort.Slice(a.list, func(i, j int) bool {
		vi := a.list[i]
		vj := a.list[j]
		var less, equal bool
		switch valA := vi[sortBy].(type) {
		case string:
			valB := vj[sortBy].(string)
			less = valA < valB
			if !sortBySlug && !less {
				equal = valA == valB
			}
		case int:
			valB := vj[sortBy].(int)
			less = valA < valB
			if !sortBySlug && !less {
				equal = valA == valB
			}
		}
		if equal {
			slugA := vi["slug"].(string)
			slugB := vj["slug"].(string)
			less = slugA < slugB
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

	// Copy the original cursor
	cursors := make([]int, len(a.registries))
	for i, reg := range a.registries {
		cursors[i] = reg.cursor
	}

	// Calculation of the next multi-cursor by iterating through the sorted and
	// truncated list and incrementing the dimension of the multi-cursor
	// associated with the objects registry.
	//
	// In the end, we also check if the end value of each dimensions of the
	// cursor reached the end of the list. If so, the dimension is set to -1.
	l := len(a.registries)
	for _, o := range list {
		slug := o["slug"].(string)
		offsets := a.slugs[slug]

		i := 0
		// This first loop checks the first element >= 0 in the offsets associated
		// to the object. This first non null element is set as the cursor of the
		// dimension.
		for ; i < l; i++ {
			if c := offsets[i]; c > 0 {
				cursors[i] = c
				break
			}
		}
		// We continue the iteration to the next lower-priority dimensions and for
		// non-null ones, we can increment their value by at-most one. This
		// correspond to values that where rejected by having the same slugs as
		// prioritized objects.
		i++
		for ; i < l; i++ {
			if c := offsets[i]; c > 0 && cursors[i] == c-1 {
				cursors[i] = c
			}
		}
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

func fetchUntilFound(client *http.Client, registries []*url.URL, requestURI string, cache CacheControl) (resp *http.Response, ok bool, err error) {
	ref, err := url.Parse(requestURI)
	if err != nil {
		return
	}
	for _, registry := range registries {
		resp, ok, err = fetch(client, registry, ref, cache)
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

func fetch(client *http.Client, registry, ref *url.URL, cache CacheControl) (resp *http.Response, ok bool, err error) {
	u := registry.ResolveReference(ref)
	u.Path = path.Join(registry.Path, ref.Path)
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return
	}
	if cache == NoCache {
		req.Header.Set("cache-control", "no-cache")
	}
	resp, err = client.Do(req)
	if err != nil {
		// Try again for temporary errors
		if ne, ok := err.(net.Error); ok && ne.Temporary() {
			resp, err = client.Do(req)
		}
		if err != nil {
			return
		}
	}
	defer func() {
		if !ok {
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
	return resp, true, nil
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
