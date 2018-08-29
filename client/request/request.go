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
	"time"
)

const defaultUserAgent = "go-cozy-client"

// defaultClient is the client used by default to access the stack. We avoid
// the use of http.DefaultClient which does not have any timeout.
var defaultClient = &http.Client{
	Timeout: 15 * time.Second,
}

type (
	// Authorizer is an interface to represent any element that can be used as a
	// token bearer.
	Authorizer interface {
		AuthHeader() string
		RealtimeToken() string
	}

	// Headers is a map of strings used to represent HTTP headers
	Headers map[string]string

	// Options is a struct holding of the details of a request.
	//
	// The NoResponse field can be used in case the call's response if not used. In
	// such cases, the response body is automatically closed.
	Options struct {
		Addr          string
		Domain        string
		Scheme        string
		Method        string
		Path          string
		Queries       url.Values
		Headers       Headers
		Body          io.Reader
		Authorizer    Authorizer
		ContentLength int64
		NoResponse    bool

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

// RealtimeToken implemented the interface Authorizer.
func (b *BasicAuthorizer) RealtimeToken() string {
	return ""
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

// RealtimeToken implemented the interface Authorizer.
func (b *BearerAuthorizer) RealtimeToken() string {
	return b.Token
}

// Req performs a request with the specified request options.
func Req(opts *Options) (*http.Response, error) {
	scheme := opts.Scheme
	if scheme == "" {
		scheme = "http"
	}
	var host string
	if opts.Addr != "" {
		host = opts.Addr
	} else {
		host = opts.Domain
	}
	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   opts.Path,
	}
	if opts.Queries != nil {
		u.RawQuery = opts.Queries.Encode()
	}

	req, err := http.NewRequest(opts.Method, u.String(), opts.Body)
	if err != nil {
		return nil, err
	}

	req.Host = opts.Domain
	if opts.ContentLength > 0 {
		req.ContentLength = opts.ContentLength
	}
	for k, v := range opts.Headers {
		req.Header.Add(k, v)
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
		client = defaultClient
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return res, parseError(opts, res)
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

// ErrSSEParse is used when an error occurred while parsing the SSE stream.
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
