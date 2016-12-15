package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	log "github.com/Sirupsen/logrus"
)

type apiErrors struct {
	Errors []struct {
		Status string `json:"status"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
}

type client struct {
	c *http.Client

	addr string
	pass string
}

func clientCreateRequest(c *client, method, path string, q url.Values, r io.Reader) (*http.Request, error) {
	u := url.URL{
		Scheme: "http",
		Host:   c.addr,
		Path:   path,
	}
	if q != nil {
		u.RawQuery = q.Encode()
	}
	log.Debugf("%s %s", method, u.String())
	req, err := http.NewRequest(method, u.String(), r)
	if err != nil {
		return nil, err
	}
	if c.pass != "" {
		req.SetBasicAuth("", c.pass)
	}
	return req, nil
}

func clientRequest(c *client, method, path string, q url.Values, body interface{}) (*http.Response, error) {
	var r io.Reader
	var ok bool
	if body != nil {
		if r, ok = body.(io.Reader); !ok {
			var b []byte
			b, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			r = bytes.NewBuffer(b)
		}
	}

	req, err := clientCreateRequest(c, method, path, q, r)
	if err != nil {
		return nil, err
	}

	var client *http.Client
	if c.c != nil {
		client = c.c
	} else {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if err := clientErrCheck(res); err != nil {
		return nil, err
	}

	return res, nil
}

func clientRequestAndClose(c *client, method, path string, q url.Values, body interface{}) error {
	res, err := clientRequest(c, method, path, q, body)
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}

func clientRequestParsed(c *client, method, path string, q url.Values, body interface{}, v interface{}) error {
	res, err := clientRequest(c, method, path, q, body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return json.NewDecoder(res.Body).Decode(&v)
}

func clientErrCheck(res *http.Response) error {
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var errs apiErrors
	if err := json.Unmarshal(b, &errs); err != nil || errs.Errors == nil || len(errs.Errors) == 0 {
		return fmt.Errorf("Unknown error %d (%v)", res.StatusCode, string(b))
	}
	apiErr := errs.Errors[0]
	return fmt.Errorf("%s (%s %s)", apiErr.Detail, apiErr.Status, apiErr.Title)
}
