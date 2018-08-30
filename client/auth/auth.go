package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/config"
)

type (
	// Client describes the data of an OAuth client
	Client struct {
		ClientID          string   `json:"client_id,omitempty"`
		ClientSecret      string   `json:"client_secret"`
		SecretExpiresAt   int      `json:"client_secret_expires_at"`
		RegistrationToken string   `json:"registration_access_token"`
		RedirectURIs      []string `json:"redirect_uris"`
		ClientName        string   `json:"client_name"`
		ClientKind        string   `json:"client_kind,omitempty"`
		ClientURI         string   `json:"client_uri,omitempty"`
		LogoURI           string   `json:"logo_uri,omitempty"`
		PolicyURI         string   `json:"policy_uri,omitempty"`
		SoftwareID        string   `json:"software_id"`
		SoftwareVersion   string   `json:"software_version,omitempty"`
	}

	// AccessToken describes the content of an access token
	AccessToken struct {
		TokenType    string `json:"token_type"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}

	// UserAcceptFunc is a function that can be defined by the user of this
	// library to describe how to ask the user for authorizing the client to
	// access to its data.
	//
	// The method should return the url on which the user has been redirected
	// which should contain a registering code and state, or an error .
	UserAcceptFunc func(accessURL string) (*url.URL, error)

	// Request represents an OAuth request with client parameters (*Client) and
	// list of scopes that the application wants to access.
	Request struct {
		ClientParams *Client
		Scopes       []string
		Domain       string
		Scheme       string
		HTTPClient   *http.Client
		UserAgent    string
		UserAccept   UserAcceptFunc
		Storage      Storage

		token  *AccessToken
		client *Client
	}

	// Error represents a client registration error returned by the OAuth server
	Error struct {
		Value       string `json:"error"`
		Description string `json:"error_description,omitempty"`
	}
)

func (e *Error) Error() string {
	return fmt.Sprintf("Authentication error: %s (%s)", e.Description, e.Value)
}

// Clone returns a new Client with cloned values
func (c *Client) Clone() *Client {
	redirects := make([]string, len(c.RedirectURIs))
	copy(redirects, c.RedirectURIs)
	return &Client{
		ClientID:          c.ClientID,
		ClientSecret:      c.ClientSecret,
		SecretExpiresAt:   c.SecretExpiresAt,
		RegistrationToken: c.RegistrationToken,
		RedirectURIs:      redirects,
		ClientName:        c.ClientName,
		ClientKind:        c.ClientKind,
		ClientURI:         c.ClientURI,
		LogoURI:           c.LogoURI,
		PolicyURI:         c.PolicyURI,
		SoftwareID:        c.SoftwareID,
		SoftwareVersion:   c.SoftwareVersion,
	}
}

// Clone returns a new AccessToken with cloned values
func (t *AccessToken) Clone() *AccessToken {
	return &AccessToken{
		TokenType:    t.TokenType,
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		Scope:        t.Scope,
	}
}

// AuthHeader implements the Tokener interface for the client
func (c *Client) AuthHeader() string {
	return "Bearer " + c.RegistrationToken
}

// AuthHeader implements the Tokener interface for the access token
func (t *AccessToken) AuthHeader() string {
	return "Bearer " + t.AccessToken
}

// RealtimeToken implements the Tokener interface for the access token
func (t *AccessToken) RealtimeToken() string {
	return t.AccessToken
}

// AuthHeader implements the Tokener interface for the request
func (r *Request) AuthHeader() string {
	return r.token.AuthHeader()
}

// RealtimeToken implements the Tokener interface for the access token
func (r *Request) RealtimeToken() string {
	return r.token.RealtimeToken()
}

// defaultClient defaults some values of the given client
func defaultClient(c *Client) *Client {
	if c == nil {
		c = &Client{}
	}
	if c.SoftwareID == "" {
		c.SoftwareID = "github.com/cozy/cozy-stack"
	}
	if c.SoftwareVersion == "" {
		c.SoftwareVersion = config.Version
	}
	if c.ClientName == "" {
		c.ClientName = "Cozy Go client"
	}
	if c.ClientKind == "" {
		c.ClientKind = "unknown"
	}
	return c
}

// Authenticate will start the authentication flow.
//
// If the storage has a client and token stored, it is reused and no
// authentication flow is started. Otherwise, a new client is registered and
// the authentication process is started.
func (r *Request) Authenticate() error {
	client, token, err := r.Storage.Load(r.Domain)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if client != nil && token != nil {
		r.client, r.token = client, token
		return nil
	}
	if client == nil {
		client, err = r.RegisterClient(defaultClient(r.ClientParams))
		if err != nil {
			return err
		}
	}
	b := make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, b); err != nil {
		return err
	}
	state := base64.StdEncoding.EncodeToString(b)
	if err = r.Storage.Save(r.Domain, client, nil); err != nil {
		return err
	}
	codeURL, err := r.AuthCodeURL(client, state)
	if err != nil {
		return err
	}
	receivedURL, err := r.UserAccept(codeURL)
	if err != nil {
		return err
	}
	query := receivedURL.Query()
	if state != query.Get("state") {
		return errors.New("Non matching states")
	}
	token, err = r.GetAccessToken(client, query.Get("code"))
	if err != nil {
		return err
	}
	if err = r.Storage.Save(r.Domain, client, token); err != nil {
		return err
	}
	r.client, r.token = client, token
	return nil
}

// AuthCodeURL returns the url on which the user is asked to authorize the
// application.
func (r *Request) AuthCodeURL(c *Client, state string) (string, error) {
	query := url.Values{
		"client_id":     {c.ClientID},
		"redirect_uri":  {c.RedirectURIs[0]},
		"state":         {state},
		"response_type": {"code"},
		"scope":         {strings.Join(r.Scopes, " ")},
	}
	u := url.URL{
		Scheme:   "https",
		Host:     r.Domain,
		Path:     "/auth/authorize",
		RawQuery: query.Encode(),
	}
	return u.String(), nil
}

// req performs an authentication HTTP request
func (r *Request) req(opts *request.Options) (*http.Response, error) {
	opts.Domain = r.Domain
	opts.Scheme = r.Scheme
	opts.Client = r.HTTPClient
	opts.ParseError = parseError
	return request.Req(opts)
}

// RegisterClient performs the registration of the specified client.
func (r *Request) RegisterClient(c *Client) (*Client, error) {
	body, err := request.WriteJSON(c)
	if err != nil {
		return nil, err
	}
	res, err := r.req(&request.Options{
		Method: "POST",
		Path:   "/auth/register",
		Headers: request.Headers{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Body: body,
	})
	if err != nil {
		return nil, err
	}
	return readClient(res.Body)
}

// GetAccessToken fetch the access token using the specified authorization
// code.
func (r *Request) GetAccessToken(c *Client, code string) (*AccessToken, error) {
	q := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
	}
	return r.retrieveToken(c, nil, q)
}

// RefreshToken performs a token refresh using the specified client and current
// access token.
func (r *Request) RefreshToken(c *Client, t *AccessToken) (*AccessToken, error) {
	q := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {t.RefreshToken},
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
	}
	return r.retrieveToken(c, t, q)
}

func (r *Request) retrieveToken(c *Client, t *AccessToken, q url.Values) (*AccessToken, error) {
	opts := &request.Options{
		Method: "POST",
		Path:   "/auth/access_token",
		Body:   strings.NewReader(q.Encode()),
		Headers: request.Headers{
			"Content-Type": "application/x-www-form-urlencoded",
			"Accept":       "application/json",
		},
	}
	if t != nil {
		opts.Authorizer = t
	}

	res, err := r.req(opts)

	if err != nil {
		return nil, err
	}
	token := &AccessToken{}
	if err := request.ReadJSON(res.Body, token); err != nil {
		return nil, err
	}
	return token, nil
}

func parseError(res *http.Response, b []byte) error {
	var err Error
	if err := json.Unmarshal(b, &err); err != nil {
		return &request.Error{
			Status: http.StatusText(res.StatusCode),
			Title:  http.StatusText(res.StatusCode),
			Detail: string(b),
		}
	}
	// TODO: handle multi-error
	return &err
}

func readClient(r io.ReadCloser) (*Client, error) {
	client := &Client{}
	if err := request.ReadJSON(r, client); err != nil {
		return nil, err
	}
	return defaultClient(client), nil
}
