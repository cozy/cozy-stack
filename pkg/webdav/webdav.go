// Package webdav is a webdav client library.
package webdav

import (
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/safehttp"
)

type Client struct {
	Scheme   string
	Host     string
	Username string
	Password string
	BasePath string
	Logger   *logger.Entry
}

func (c *Client) Mkcol(path string) error {
	res, err := c.req("MKCOL", path, nil, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case 201:
		return nil
	case 401, 403:
		return ErrInvalidAuth
	case 405:
		return ErrAlreadyExist
	case 409:
		return ErrParentNotFound
	default:
		return ErrInternalServerError
	}
}

func (c *Client) req(method, path string, headers map[string]string, body io.Reader) (*http.Response, error) {
	path = c.BasePath + fixSlashes(path)
	u := url.URL{
		Scheme: c.Scheme,
		Host:   c.Host,
		User:   url.UserPassword(c.Username, c.Password),
		Path:   path,
	}
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "cozy-stack "+build.Version+" ("+runtime.Version()+")")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := safehttp.ClientWithKeepAlive.Do(req)
	if err != nil {
		return nil, err
	}
	c.Logger.Infof("%s %s %s: %d", method, c.Host, path, res.StatusCode)
	return res, nil
}

func fixSlashes(s string) string {
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}
