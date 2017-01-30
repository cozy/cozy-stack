package request

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cozy/cozy-stack/pkg/config"
)

const defaultUserAgent = "go-cozy-client"

type (
	// Authorizer is an interface to represent any element that can be used as a
	// token bearer.
	Authorizer interface {
		AuthHeader() string
	}

	// Headers is a map of strings used to represent HTTP headers
	Headers map[string]string

	// Options is a struct holding of the details of a request.
	//
	// The NoResponse field can be used in case the call's response if not used. In
	// such cases, the response body is automatically closed.
	Options struct {
		Domain     string
		Method     string
		Path       string
		Queries    url.Values
		Headers    Headers
		Body       io.Reader
		Authorizer Authorizer
		NoResponse bool

		DisableSecure bool
		Client        *http.Client
		UserAgent     string
		ParseError    func(res *http.Response, b []byte) error
		BasicPassword string
	}

	// Error is the typical JSON-API error returned by the API
	Error struct {
		Status string `json:"status"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
)

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Title
	}
	return fmt.Sprintf("%s: %s", e.Title, e.Detail)
}

// BasicAuthorizer implements the HTTP basic auth for authorization.
type BasicAuthorizer struct {
	Username string
	Password string
}

func (b *BasicAuthorizer) AuthHeader() string {
	auth := b.Username + ":" + b.Password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// Req performs a request with the specified request options.
func Req(opts *Options) (*http.Response, error) {
	var scheme string
	if opts.DisableSecure {
		if !config.IsDevRelease() {
			panic("Should not disable HTTPs")
		}
		scheme = "http"
	} else {
		scheme = "https"
	}
	u := url.URL{
		Scheme: scheme,
		Host:   opts.Domain,
		Path:   opts.Path,
	}
	if opts.Queries != nil {
		u.RawQuery = opts.Queries.Encode()
	}

	req, err := http.NewRequest(opts.Method, u.String(), opts.Body)
	if err != nil {
		return nil, err
	}

	if opts.Headers != nil {
		for k, v := range opts.Headers {
			if k == "Content-Length" {
				var contentLength int64
				contentLength, err = strconv.ParseInt(v, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("Invalid Content-Length value")
				}
				req.ContentLength = contentLength
			} else {
				req.Header.Add(k, v)
			}
		}
	}

	if opts.Authorizer != nil {
		req.Header.Add("Authorization", opts.Authorizer.AuthHeader())
	}

	ua := opts.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}

	req.Header.Add("User-Agent", ua)

	if opts.BasicPassword != "" {
		req.SetBasicAuth("", opts.BasicPassword)
	}

	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, parseError(opts, res)
	}

	if opts.NoResponse {
		err = res.Body.Close()
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

func parseError(opts *Options, res *http.Response) (err error) {
	defer checkClose(res.Body, &err)
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return &Error{
			Status: http.StatusText(res.StatusCode),
			Title:  http.StatusText(res.StatusCode),
			Detail: err.Error(),
		}
	}
	if opts.ParseError == nil {
		return &Error{
			Status: http.StatusText(res.StatusCode),
			Title:  http.StatusText(res.StatusCode),
			Detail: string(b),
		}
	}
	return opts.ParseError(res, b)
}

// ReadJSON reads the content of the specified ReadCloser and closes it.
func ReadJSON(r io.ReadCloser, data interface{}) (err error) {
	defer checkClose(r, &err)
	return json.NewDecoder(r).Decode(&data)
}

// WriteJSON returns an io.Reader from which a JSON encoded data can be read.
func WriteJSON(data interface{}) (io.Reader, error) {
	buf, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(buf), nil
}

func checkClose(c io.Closer, err *error) {
	cerr := c.Close()
	if *err == nil && cerr != nil {
		*err = cerr
	}
}
