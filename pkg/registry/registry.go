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

// ProxyList will proxy the given request to the registries by aggregating the
// results along the way. It should be used for list endpoints.
func ProxyList(req *http.Request, registries []*url.URL) ([]json.RawMessage, error) {
	ref, err := url.Parse(req.RequestURI)
	if err != nil {
		return nil, err
	}

	refNoLimit := cloneURLWithoutQuery(ref, "cursor", "limit")

	var sortBy string
	var sortReverse bool
	var cursor, limit int

	q := ref.Query()
	if v, ok := q["cursor"]; ok {
		cursor, _ = strconv.Atoi(v[0])
	}
	if cursor <= 0 {
		cursor = 0
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

	uniques := make(map[string]struct{})

	list := make([]json.RawMessage, 0)
	for _, registry := range registries[:len(registries)-1] {
		list, err = fetchListUnique(registry, refNoLimit, list, uniques)
		if err != nil {
			return nil, err
		}
	}

	var ok bool
	list, ok = sortedList(list, cursor, sortBy, limit, sortReverse, false)
	if ok {
		return list, nil
	}

	rest := limit - (len(list) - cursor)

	lastRegistry := registries[len(registries)-1]
	refNewLimit, _ := url.Parse(refNoLimit.String())
	refQ := refNewLimit.Query()
	refQ.Add("limit", strconv.Itoa(rest))

	list, err = fetchListUnique(lastRegistry, ref, list, uniques)
	if err != nil {
		return nil, err
	}

	list, _ = sortedList(list, cursor, sortBy, limit, sortReverse, true)
	return list, nil
}

func fetchListUnique(registry, ref *url.URL, list []json.RawMessage, uniques map[string]struct{}) (result []json.RawMessage, err error) {
	result = list
	rc, ok, err := fetch(registry, ref)
	if err != nil {
		return
	}
	if !ok {
		return
	}
	defer rc.Close()
	var resp []json.RawMessage
	if err = json.NewDecoder(rc).Decode(&resp); err != nil {
		return
	}
	if len(resp) == 0 {
		return
	}
	var el struct {
		Name string `json:"name"`
	}
	for _, b := range resp {
		if err = json.Unmarshal(b, &el); err != nil {
			return
		}
		if _, ok = uniques[el.Name]; !ok {
			result = append(result, b)
			uniques[el.Name] = struct{}{}
		}
	}
	return
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

func sortedList(list []json.RawMessage, cursor int, sortBy string, limit int, reverse, starved bool) ([]json.RawMessage, bool) {
	if cursor-len(list) < limit {
		if !starved {
			return list, false
		}
		if cursor > len(list) {
			cursor = len(list)
		}
		list = list[cursor:]
	}
	sort.Slice(list, func(i, j int) bool {
		var a, b map[string]interface{}
		json.Unmarshal(list[i], &a)
		json.Unmarshal(list[j], &b)
		var less bool
		switch valA := a[sortBy].(type) {
		case string:
			valB := b[sortBy].(string)
			less = valA < valB
		case int:
			valB := b[sortBy].(int)
			less = valA < valB
		}
		if reverse {
			return !less
		}
		return less
	})
	offset := cursor + limit
	if offset > len(list) {
		offset = len(list)
	}
	fmt.Printf("len(list)=%d cursor=%d offset=%d limit=%d\n", len(list), cursor, offset, limit)
	return list[cursor:offset], true
}

func cloneURLWithoutQuery(u *url.URL, filter ...string) *url.URL {
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
