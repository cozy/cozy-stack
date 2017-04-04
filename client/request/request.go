package request

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"

	"github.com/Sirupsen/logrus"
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
		Scheme     string
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
	}

	// Error is the typical JSON-API error returned by the API
	Error struct {
		Status string `json:"status"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
)

// httpFallback can be used to fallback on a non-secure http request if the
// https one has failed to open. This should happend only on dev binaries.
var httpFallback int32

func (e *Error) Error() string {
	if e.Detail == "" || e.Title == e.Detail {
		return e.Title
	}
	return fmt.Sprintf("%s: %s", e.Title, e.Detail)
}

// BasicAuthorizer implements the HTTP basic auth for authorization.
type BasicAuthorizer struct {
	Username string
	Password string
}

// AuthHeader implemented the interface Authorizer.
func (b *BasicAuthorizer) AuthHeader() string {
	auth := b.Username + ":" + b.Password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

// BearerAuthorizer implements a placeholder authorizer if the token is already
// known.
type BearerAuthorizer struct {
	Token string
}

// AuthHeader implemented the interface Authorizer.
func (b *BearerAuthorizer) AuthHeader() string {
	return "Bearer " + b.Token
}

// Req performs a request with the specified request options.
func Req(opts *Options) (*http.Response, error) {
	scheme := opts.Scheme
	if scheme == "" {
		if atomic.LoadInt32(&httpFallback) == 1 {
			scheme = "http"
		} else {
			scheme = "https"
		}
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

	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}

	res, err := client.Do(req)
	if err != nil {
		// for a development release, we fallback on http mode if the https request
		// is not successful.
		// TODO: better identify the error (cross-platform)
		if config.IsDevRelease() && scheme == "https" {
			logrus.Debug("[request] fallback on http transport since https request has failed", err)
			atomic.StoreInt32(&httpFallback, 1)
			return Req(opts)
		}
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

// ErrSSEParse is used when an error occured while parsing the SSE stream.
var ErrSSEParse = errors.New("could not parse event stream")

// SSEEvent holds the data of a single SSE event.
type SSEEvent struct {
	Name  string
	Data  []byte
	Error error
}

// ReadSSE reads and parse a SSE source from a bufio.Reader into a channel of
// SSEEvent.
func ReadSSE(r io.ReadCloser, ch chan *SSEEvent) {
	var err error
	defer func() {
		if err != nil {
			ch <- &SSEEvent{Error: err}
		}
		if errc := r.Close(); errc != nil && err == nil {
			ch <- &SSEEvent{Error: errc}
		}
		close(ch)
	}()
	rb := bufio.NewReader(r)
	var ev *SSEEvent
	for {
		var bs []byte
		bs, err = rb.ReadBytes('\n')
		if err == io.EOF {
			err = nil
			return
		}
		if err != nil {
			return
		}
		if bytes.Equal(bs, []byte("\r\n")) {
			ev = nil
			continue
		}
		spl := bytes.SplitN(bs, []byte(": "), 2)
		if len(spl) != 2 {
			err = ErrSSEParse
			return
		}
		k, v := string(spl[0]), bytes.TrimSpace(spl[1])
		switch k {
		case "event":
			ev = &SSEEvent{Name: string(v)}
		case "data":
			if ev == nil {
				err = ErrSSEParse
				return
			}
			ev.Data = v
			ch <- ev
		default:
			err = ErrSSEParse
			return
		}
	}
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
