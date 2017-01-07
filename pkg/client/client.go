package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cozy/cozy-stack/pkg/client/auth"
	"github.com/cozy/cozy-stack/pkg/client/request"
)

// ErrWrongPassphrase is used when the passphrase is wrong
var ErrWrongPassphrase = errors.New("Unauthorized: wrong passphrase")

// jsonAPIErrors is a group of errors. It is the error type returned by the
// API.
type jsonAPIErrors struct {
	Errors []*request.Error `json:"errors"`
}

// jsonAPIDocument is a simple JSONAPI document used to un-serialized
type jsonAPIDocument struct {
	Data     json.RawMessage `json:"data"`
	Included json.RawMessage `json:"included"`
}

// Client encapsulates the element representing a typical connection to the
// HTTP api of the cozy-stack.
//
// It holds the elements to authenticate a user, as well as the transport layer
// used for all the calls to the stack.
type Client struct {
	Domain string

	AdminPassword string
	AuthClient    *auth.Client
	AuthScopes    []string
	AuthAccept    auth.UserAcceptFunc
	AuthStorage   auth.Storage

	UserAgent     string
	Retries       int
	Timeout       time.Duration
	Transport     http.RoundTripper
	DisableSecure bool
	IsAdmin       bool

	authed bool
	inited int32
	authMu sync.Mutex
	auth   *auth.Request
	client *http.Client
}

func (c *Client) init() {
	if !atomic.CompareAndSwapInt32(&c.inited, 0, 1) {
		return
	}
	if c.Retries == 0 {
		c.Retries = 3
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.Transport == nil {
		c.Transport = &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConnsPerHost: 1024,
		}
	}
	if c.AuthStorage == nil {
		c.AuthStorage = auth.NewFileStorage()
	}
	if c.client == nil {
		c.client = &http.Client{
			Transport: c.Transport,
			Timeout:   c.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
}

// Authenticate is used to authenticate a user.
func (c *Client) Authenticate() (request.Authorizer, error) {
	c.init()
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.authed {
		return c.auth, nil
	}
	if c.auth == nil {
		c.auth = &auth.Request{
			ClientParams:  c.AuthClient,
			Scopes:        c.AuthScopes,
			Domain:        c.Domain,
			DisableSecure: c.DisableSecure,
			HTTPClient:    c.client,
			UserAgent:     c.UserAgent,
			UserAccept:    c.AuthAccept,
			Storage:       c.AuthStorage,
		}
	}
	if err := c.auth.Authenticate(); err != nil {
		return nil, err
	}
	c.authed = true
	return c.auth, nil
}

// Req is used to perform a request to the stack given the ReqOptions passed.
func (c *Client) Req(opts *request.Options) (*http.Response, error) {
	c.init()
	var err error
	if c.IsAdmin {
		opts.BasicPassword = c.AdminPassword
	} else {
		opts.Authorizer, err = c.Authenticate()
	}
	if err != nil {
		return nil, err
	}
	opts.Domain = c.Domain
	opts.Client = c.client
	opts.UserAgent = c.UserAgent
	opts.DisableSecure = c.DisableSecure
	opts.ParseError = parseJSONAPIError
	return request.Req(opts)
}

func parseJSONAPIError(res *http.Response, b []byte) error {
	var errs jsonAPIErrors
	if err := json.Unmarshal(b, &errs); err != nil || errs.Errors == nil || len(errs.Errors) == 0 {
		return &request.Error{
			Status: http.StatusText(res.StatusCode),
			Title:  http.StatusText(res.StatusCode),
			Detail: string(b),
		}
	}
	// TODO: handle multi-error
	return errs.Errors[0]
}

func readJSONAPI(r io.ReadCloser, data interface{}, included interface{}) (err error) {
	defer func() {
		if cerr := r.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()
	var doc jsonAPIDocument
	decoder := json.NewDecoder(r)
	if err = decoder.Decode(&doc); err != nil {
		return err
	}
	if data != nil {
		if err = json.Unmarshal(doc.Data, &data); err != nil {
			return err
		}
	}
	if included != nil && doc.Included != nil {
		if err = json.Unmarshal(doc.Included, &included); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONAPI(data interface{}) (io.Reader, error) {
	buf, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	doc := jsonAPIDocument{Data: buf}
	buf, err = json.Marshal(doc)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(buf), nil
}
