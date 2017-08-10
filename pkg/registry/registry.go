package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/cozy/echo"
)

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

var ErrVersionNotFound = errors.New("Version not found")

var proxyClient = &http.Client{
	Timeout: 10 * time.Second,
}

func GetLatestVersion(slug, channel string, registries []*url.URL) (*Version, error) {
	requestURI := fmt.Sprintf("/registry/%s/%s/latest",
		url.PathEscape(slug),
		url.PathEscape(channel))
	rc, ok, err := resolveInRegistries(registries, requestURI)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrVersionNotFound
	}
	defer rc.Close()
	var v *Version
	if err = json.NewDecoder(rc).Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func Proxy(req *http.Request, registries []*url.URL) (io.ReadCloser, error) {
	rc, ok, err := resolveInRegistries(registries, req.RequestURI)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, echo.NewHTTPError(http.StatusNotFound)
	}
	return rc, nil
}

func resolveInRegistries(registries []*url.URL, requestURI string) (rc io.ReadCloser, ok bool, err error) {
	ref, err := url.Parse(requestURI)
	if err != nil {
		return
	}
	for _, registry := range registries {
		rc, ok, err = resolveInRegistry(registry, ref)
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

func resolveInRegistry(registry *url.URL, ref *url.URL) (rc io.ReadCloser, ok bool, err error) {
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
