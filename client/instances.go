package client

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/client/request"
)

// Instance is a struct holding the representation of an instance on the API.
type Instance struct {
	ID    string `json:"id"`
	Rev   string `json:"rev"`
	Attrs struct {
		Domain         string `json:"domain"`
		Locale         string `json:"locale"`
		StorageURL     string `json:"storage"`
		Dev            bool   `json:"dev"`
		PassphraseHash []byte `json:"passphraseHash,omitempty"`
		RegisterToken  []byte `json:"registerToken,omitempty"`
	} `json:"attributes"`
}

// InstanceOptions contains the options passed on instance creation.
type InstanceOptions struct {
	Domain   string
	Locale   string
	Timezone string
	Email    string
	Apps     []string
	Dev      bool
}

// TokenOptions is a struct holding all the options to generate a token.
type TokenOptions struct {
	Domain   string
	Subject  string
	Audience string
	Scope    []string
}

// CreateInstance is used to create a new cozy instance of the specified domain
// and locale.
func (c *Client) CreateInstance(opts *InstanceOptions) (*Instance, error) {
	var dev string
	if opts.Dev {
		dev = "true"
	} else {
		dev = "false"
	}
	if !validDomain(opts.Domain) {
		return nil, fmt.Errorf("Invalid domain: %s", opts.Domain)
	}
	res, err := c.Req(&request.Options{
		Method: "POST",
		Path:   "/instances",
		Queries: url.Values{
			"Domain":   {opts.Domain},
			"Locale":   {opts.Locale},
			"Timezone": {opts.Timezone},
			"Email":    {opts.Email},
			"Apps":     {strings.Join(opts.Apps, ",")},
			"Dev":      {dev},
		},
	})
	if err != nil {
		return nil, err
	}
	return readInstance(res)
}

// ListInstances returns the list of instances recorded on the stack.
func (c *Client) ListInstances() ([]*Instance, error) {
	res, err := c.Req(&request.Options{
		Method: "GET",
		Path:   "/instances",
	})
	if err != nil {
		return nil, err
	}
	var list []*Instance
	if err = readJSONAPI(res.Body, &list, nil); err != nil {
		return nil, err
	}
	return list, nil
}

// DestroyInstance is used to delete an instance and all its data.
func (c *Client) DestroyInstance(domain string) (*Instance, error) {
	if !validDomain(domain) {
		return nil, fmt.Errorf("Invalid domain: %s", domain)
	}
	res, err := c.Req(&request.Options{
		Method:     "DELETE",
		Path:       "/instances/" + domain,
		NoResponse: true,
	})
	if err != nil {
		return nil, err
	}
	return readInstance(res)
}

// GetToken is used to generate a toke with the specified options.
func (c *Client) GetToken(opts *TokenOptions) (string, error) {
	q := url.Values{
		"Domain":   {opts.Domain},
		"Subject":  {opts.Subject},
		"Audience": {opts.Audience},
		"Scope":    {strings.Join(opts.Scope, " ")},
	}
	res, err := c.Req(&request.Options{
		Method:     "GET",
		Path:       "/instances/token",
		Queries:    q,
		NoResponse: false,
	})
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func readInstance(res *http.Response) (*Instance, error) {
	in := &Instance{}
	if err := readJSONAPI(res.Body, &in, nil); err != nil {
		return nil, err
	}
	return in, nil
}

func validDomain(domain string) bool {
	return !strings.ContainsAny(domain, " /?#@\t\r\n")
}
