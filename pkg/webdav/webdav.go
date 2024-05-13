// Package webdav is a webdav client library.
package webdav

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/safehttp"
	"github.com/labstack/echo/v4"
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

func (c *Client) Delete(path string) error {
	res, err := c.req("DELETE", path, nil, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case 204:
		return nil
	case 401, 403:
		return ErrInvalidAuth
	case 404:
		return ErrNotFound
	default:
		return ErrInternalServerError
	}
}

func (c *Client) Copy(oldPath, newPath string) error {
	u := url.URL{
		Scheme: c.Scheme,
		Host:   c.Host,
		User:   url.UserPassword(c.Username, c.Password),
		Path:   c.BasePath + fixSlashes(newPath),
	}
	headers := map[string]string{
		"Destination": u.String(),
		"Overwrite":   "F",
	}
	res, err := c.req("COPY", oldPath, headers, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case 201, 204:
		return nil
	case 401, 403:
		return ErrInvalidAuth
	case 404, 409:
		return ErrNotFound
	case 412:
		return ErrAlreadyExist
	default:
		return ErrInternalServerError
	}
}

func (c *Client) Put(path string, headers map[string]string, body io.Reader) error {
	res, err := c.req("PUT", path, headers, body)
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
	case 404, 409:
		return ErrParentNotFound
	default:
		return ErrInternalServerError
	}
}

func (c *Client) Get(path string) (*Download, error) {
	res, err := c.req("GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == 200 {
		return &Download{
			Content:      res.Body,
			ETag:         res.Header.Get("Etag"),
			Length:       res.Header.Get(echo.HeaderContentLength),
			Mime:         res.Header.Get(echo.HeaderContentType),
			LastModified: res.Header.Get(echo.HeaderLastModified),
		}, nil
	}

	defer res.Body.Close()
	switch res.StatusCode {
	case 401, 403:
		return nil, ErrInvalidAuth
	case 404:
		return nil, ErrNotFound
	default:
		return nil, ErrInternalServerError
	}
}

type Download struct {
	Content      io.ReadCloser
	ETag         string
	Length       string
	Mime         string
	LastModified string
}

func (c *Client) List(path string) ([]Item, error) {
	path = fixSlashes(path)
	headers := map[string]string{
		"Content-Type": "application/xml;charset=UTF-8",
		"Accept":       "application/xml",
		"Depth":        "1",
	}
	payload := strings.NewReader(ListFilesPayload)
	res, err := c.req("PROPFIND", path, headers, payload)
	if err != nil {
		return nil, err
	}
	defer func() {
		// Flush the body, so that the connection can be reused by keep-alive
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	switch res.StatusCode {
	case 200, 207:
		// OK continue the work
	case 401, 403:
		return nil, ErrInvalidAuth
	case 404:
		return nil, ErrNotFound
	default:
		return nil, ErrInternalServerError
	}

	// https://docs.nextcloud.com/server/20/developer_manual/client_apis/WebDAV/basic.html#requesting-properties
	var multistatus multistatus
	if err := xml.NewDecoder(res.Body).Decode(&multistatus); err != nil {
		return nil, err
	}

	var items []Item
	for _, response := range multistatus.Responses {
		// We want only the children, not the directory itself
		if response.Href == c.BasePath+path {
			continue
		}
		for _, props := range response.Props {
			// Only looks for the HTTP/1.1 200 OK status
			parts := strings.Split(props.Status, " ")
			if len(parts) < 2 || parts[1] != "200" {
				continue
			}
			item := Item{
				ID:           props.FileID,
				Type:         "directory",
				Name:         props.Name,
				LastModified: props.LastModified,
				ETag:         props.ETag,
			}
			if props.Type.Local == "" {
				item.Type = "file"
				if props.Size != "" {
					if size, err := strconv.ParseUint(props.Size, 10, 64); err == nil {
						item.Size = size
					}
				}
			}
			items = append(items, item)
		}
	}
	return items, nil
}

type Item struct {
	ID           string
	Type         string
	Name         string
	Size         uint64
	ContentType  string
	LastModified string
	ETag         string
}

type multistatus struct {
	XMLName   xml.Name   `xml:"multistatus"`
	Responses []response `xml:"response"`
}

type response struct {
	Href  string  `xml:"DAV: href"`
	Props []props `xml:"DAV: propstat"`
}

type props struct {
	Status       string   `xml:"status"`
	Type         xml.Name `xml:"prop>resourcetype>collection"`
	Name         string   `xml:"prop>displayname"`
	Size         string   `xml:"prop>getcontentlength"`
	ContentType  string   `xml:"prop>getcontenttype"`
	LastModified string   `xml:"prop>getlastmodified"`
	ETag         string   `xml:"prop>getetag"`
	FileID       string   `xml:"prop>fileid"`
}

const ListFilesPayload = `<?xml version="1.0"?>
<d:propfind  xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns" xmlns:nc="http://nextcloud.org/ns">
  <d:prop>
        <d:resourcetype />
        <d:displayname />
        <d:getlastmodified />
        <d:getetag />
        <d:getcontentlength />
        <d:getcontenttype />
        <oc:fileid />
  </d:prop>
</d:propfind>
`

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
	start := time.Now()
	res, err := safehttp.ClientWithKeepAlive.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		c.Logger.Warnf("%s %s %s: %s (%s)", method, c.Host, path, err, elapsed)
		return nil, err
	}
	c.Logger.Infof("%s %s %s: %d (%s)", method, c.Host, path, res.StatusCode, elapsed)
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
