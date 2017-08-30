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

type jsonObject struct {
	name string
	reg  int
	obj  map[string]interface{}
}

// ProxyList will proxy the given request to the registries by aggregating the
// results along the way. It should be used for list endpoints.
func ProxyList(req *http.Request, registries []*url.URL) (json.RawMessage, error) {
	ref, err := url.Parse(req.RequestURI)
	if err != nil {
		return nil, err
	}

	refNoCursor := removeQueries(ref, "cursor")

	var sortBy string
	var sortReverse bool
	var limit int
	var cursors []string

	q := ref.Query()
	if v, ok := q["cursor"]; ok {
		cursors = strings.Split(v[0], "|")
	}
	diff := len(registries) - len(cursors)
	for i := 0; i < diff; i++ {
		cursors = append(cursors, "")
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

	list := make([]jsonObject, 0)
	for registryIndex, registry := range registries {
		registryRef := addQueries(refNoCursor, "cursor", cursors[registryIndex])
		list, err = fetchListUnique(registry, registryRef, registryIndex, list, uniques)
		if err != nil {
			return nil, err
		}
	}

	sortList(list, sortBy, sortReverse)

	l := limit
	if l > len(list) {
		l = len(list)
	}

	list = list[:l]
	result := make([]map[string]interface{}, l)
	for i, el := range list {
		cursors[el.reg] = el.name
		el.obj["cursor"] = strings.Join(cursors, "|")
		result[i] = el.obj
	}

	return json.Marshal(result)
}

func fetchListUnique(registry, ref *url.URL, index int, list []jsonObject, uniques map[string]struct{}) (result []jsonObject, err error) {
	result = list
	rc, ok, err := fetch(registry, ref)
	if err != nil {
		return
	}
	if !ok {
		return
	}
	defer rc.Close()
	var resp []map[string]interface{}
	if err = json.NewDecoder(rc).Decode(&resp); err != nil {
		return
	}
	if len(resp) == 0 {
		return
	}
	for _, obj := range resp {
		name := obj["name"].(string)
		if _, ok = uniques[name]; !ok {
			result = append(result, jsonObject{name, index, obj})
			uniques[name] = struct{}{}
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

func sortList(list []jsonObject, sortBy string, reverse bool) {
	sort.Slice(list, func(i, j int) bool {
		var less bool
		switch valA := list[i].obj[sortBy].(type) {
		case string:
			valB := list[j].obj[sortBy].(string)
			less = valA < valB
		case int:
			valB := list[j].obj[sortBy].(int)
			less = valA < valB
		}
		if reverse {
			return !less
		}
		return less
	})
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
