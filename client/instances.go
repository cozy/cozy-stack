package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/client/request"
)

// Instance is a struct holding the representation of an instance on the API.
type Instance struct {
	ID   string `json:"id"`
	Meta struct {
		Rev string `json:"rev"`
	} `json:"meta"`
	Attrs struct {
		Domain               string    `json:"domain"`
		DomainAliases        []string  `json:"domain_aliases,omitempty"`
		Prefix               string    `json:"prefix,omitempty"`
		Locale               string    `json:"locale"`
		UUID                 string    `json:"uuid,omitempty"`
		OIDCID               string    `json:"oidc_id,omitempty"`
		ContextName          string    `json:"context,omitempty"`
		TOSSigned            string    `json:"tos,omitempty"`
		TOSLatest            string    `json:"tos_latest,omitempty"`
		AuthMode             int       `json:"auth_mode,omitempty"`
		NoAutoUpdate         bool      `json:"no_auto_update,omitempty"`
		Blocked              bool      `json:"blocked,omitempty"`
		OnboardingFinished   bool      `json:"onboarding_finished"`
		BytesDiskQuota       int64     `json:"disk_quota,string,omitempty"`
		IndexViewsVersion    int       `json:"indexes_version"`
		SwiftLayout          int       `json:"swift_cluster,omitempty"`
		PassphraseResetToken []byte    `json:"passphrase_reset_token"`
		PassphraseResetTime  time.Time `json:"passphrase_reset_time"`
		RegisterToken        []byte    `json:"register_token,omitempty"`
	} `json:"attributes"`
}

// InstanceOptions contains the options passed on instance creation.
type InstanceOptions struct {
	Domain             string
	DomainAliases      []string
	Locale             string
	UUID               string
	OIDCID             string
	TOSSigned          string
	TOSLatest          string
	Timezone           string
	ContextName        string
	Email              string
	PublicName         string
	Settings           string
	SwiftLayout        int
	DiskQuota          int64
	Apps               []string
	Passphrase         string
	KdfIterations      int
	Debug              *bool
	Blocked            *bool
	Deleting           *bool
	OnboardingFinished *bool
	Trace              *bool
}

// TokenOptions is a struct holding all the options to generate a token.
type TokenOptions struct {
	Domain   string
	Subject  string
	Audience string
	Scope    []string
	Expire   *time.Duration
}

// OAuthClientOptions is a struct holding all the options to generate an OAuth
// client associated to an instance.
type OAuthClientOptions struct {
	Domain                string
	RedirectURI           string
	ClientName            string
	SoftwareID            string
	AllowLoginScope       bool
	OnboardingSecret      string
	OnboardingApp         string
	OnboardingPermissions string
	OnboardingState       string
}

// UpdatesOptions is a struct holding all the options to launch an update.
type UpdatesOptions struct {
	Domain             string
	DomainsWithContext string
	Slugs              []string
	ForceRegistry      bool
	OnlyRegistry       bool
	Logs               chan *JobLog
}

// ImportOptions is a struct with the options for importing a tarball.
type ImportOptions struct {
	Filename    string
	Destination string
}

// DBPrefix returns the database prefix for the instance
func (i *Instance) DBPrefix() string {
	if i.Attrs.Prefix != "" {
		return i.Attrs.Prefix
	}
	return i.Attrs.Domain
}

// GetInstance returns the instance associated with the specified domain.
func (c *Client) GetInstance(domain string) (*Instance, error) {
	res, err := c.Req(&request.Options{
		Method: "GET",
		Path:   "/instances/" + domain,
	})
	if err != nil {
		return nil, err
	}
	return readInstance(res)
}

// CreateInstance is used to create a new cozy instance of the specified domain
// and locale.
func (c *Client) CreateInstance(opts *InstanceOptions) (*Instance, error) {
	if !validDomain(opts.Domain) {
		return nil, fmt.Errorf("Invalid domain: %s", opts.Domain)
	}
	q := url.Values{
		"Domain":        {opts.Domain},
		"Locale":        {opts.Locale},
		"UUID":          {opts.UUID},
		"OIDCID":        {opts.OIDCID},
		"TOSSigned":     {opts.TOSSigned},
		"Timezone":      {opts.Timezone},
		"ContextName":   {opts.ContextName},
		"Email":         {opts.Email},
		"PublicName":    {opts.PublicName},
		"Settings":      {opts.Settings},
		"SwiftLayout":   {strconv.Itoa(opts.SwiftLayout)},
		"DiskQuota":     {strconv.FormatInt(opts.DiskQuota, 10)},
		"Apps":          {strings.Join(opts.Apps, ",")},
		"Passphrase":    {opts.Passphrase},
		"KdfIterations": {strconv.Itoa(opts.KdfIterations)},
	}
	if opts.DomainAliases != nil {
		q.Add("DomainAliases", strings.Join(opts.DomainAliases, ","))
	}
	if opts.Trace != nil && *opts.Trace {
		q.Add("Trace", "true")
	}
	res, err := c.Req(&request.Options{
		Method:  "POST",
		Path:    "/instances",
		Queries: q,
	})
	if err != nil {
		return nil, err
	}
	return readInstance(res)
}

// CountInstances returns the number of instances.
func (c *Client) CountInstances() (int, error) {
	res, err := c.Req(&request.Options{
		Method: "GET",
		Path:   "/instances/count",
	})
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	var data map[string]int
	if err = json.NewDecoder(res.Body).Decode(&data); err != nil {
		return 0, err
	}
	return data["count"], nil
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
	if err = readJSONAPI(res.Body, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// ModifyInstance is used to update an instance.
func (c *Client) ModifyInstance(opts *InstanceOptions) (*Instance, error) {
	domain := opts.Domain
	if !validDomain(domain) {
		return nil, fmt.Errorf("Invalid domain: %s", domain)
	}
	q := url.Values{
		"Locale":      {opts.Locale},
		"UUID":        {opts.UUID},
		"OIDCID":      {opts.OIDCID},
		"TOSSigned":   {opts.TOSSigned},
		"TOSLatest":   {opts.TOSLatest},
		"Timezone":    {opts.Timezone},
		"ContextName": {opts.ContextName},
		"Email":       {opts.Email},
		"PublicName":  {opts.PublicName},
		"Settings":    {opts.Settings},
		"DiskQuota":   {strconv.FormatInt(opts.DiskQuota, 10)},
	}
	if opts.DomainAliases != nil {
		q.Add("DomainAliases", strings.Join(opts.DomainAliases, ","))
	}
	if opts.Debug != nil {
		q.Add("Debug", strconv.FormatBool(*opts.Debug))
	}
	if opts.Blocked != nil {
		q.Add("Blocked", strconv.FormatBool(*opts.Blocked))
	}
	if opts.Deleting != nil {
		q.Add("Deleting", strconv.FormatBool(*opts.Deleting))
	}
	if opts.OnboardingFinished != nil {
		q.Add("OnboardingFinished", strconv.FormatBool(*opts.OnboardingFinished))
	}
	res, err := c.Req(&request.Options{
		Method:  "PATCH",
		Path:    "/instances/" + domain,
		Queries: q,
	})
	if err != nil {
		return nil, err
	}
	return readInstance(res)
}

// DestroyInstance is used to delete an instance and all its data.
func (c *Client) DestroyInstance(domain string) error {
	if !validDomain(domain) {
		return fmt.Errorf("Invalid domain: %s", domain)
	}
	_, err := c.Req(&request.Options{
		Method:     "DELETE",
		Path:       "/instances/" + domain,
		NoResponse: true,
	})
	return err
}

// GetDebug is used to known if an instance has its logger in debug mode.
func (c *Client) GetDebug(domain string) (bool, error) {
	if !validDomain(domain) {
		return false, fmt.Errorf("Invalid domain: %s", domain)
	}
	_, err := c.Req(&request.Options{
		Method:     "GET",
		Path:       "/instances/" + domain + "/debug",
		NoResponse: true,
	})
	if err != nil {
		if e, ok := err.(*request.Error); ok {
			if e.Title == http.StatusText(http.StatusNotFound) {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

// EnableDebug sets the logger of an instance in debug mode.
func (c *Client) EnableDebug(domain string, ttl time.Duration) error {
	if !validDomain(domain) {
		return fmt.Errorf("Invalid domain: %s", domain)
	}
	_, err := c.Req(&request.Options{
		Method:     "POST",
		Path:       "/instances/" + domain + "/debug",
		NoResponse: true,
		Queries: url.Values{
			"TTL": {ttl.String()},
		},
	})
	return err
}

// DisableDebug disables the debug mode for the logger of an instance.
func (c *Client) DisableDebug(domain string) error {
	if !validDomain(domain) {
		return fmt.Errorf("Invalid domain: %s", domain)
	}
	_, err := c.Req(&request.Options{
		Method:     "DELETE",
		Path:       "/instances/" + domain + "/debug",
		NoResponse: true,
	})
	return err
}

// GetToken is used to generate a token with the specified options.
func (c *Client) GetToken(opts *TokenOptions) (string, error) {
	q := url.Values{
		"Domain":   {opts.Domain},
		"Subject":  {opts.Subject},
		"Audience": {opts.Audience},
		"Scope":    {strings.Join(opts.Scope, " ")},
	}
	if opts.Expire != nil {
		q.Add("Expire", opts.Expire.String())
	}
	res, err := c.Req(&request.Options{
		Method:  "POST",
		Path:    "/instances/token",
		Queries: q,
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

// RegisterOAuthClient register a new OAuth client associated to the specified
// instance.
func (c *Client) RegisterOAuthClient(opts *OAuthClientOptions) (map[string]interface{}, error) {
	q := url.Values{
		"Domain":                {opts.Domain},
		"RedirectURI":           {opts.RedirectURI},
		"ClientName":            {opts.ClientName},
		"SoftwareID":            {opts.SoftwareID},
		"AllowLoginScope":       {strconv.FormatBool(opts.AllowLoginScope)},
		"OnboardingSecret":      {opts.OnboardingSecret},
		"OnboardingApp":         {opts.OnboardingApp},
		"OnboardingPermissions": {opts.OnboardingPermissions},
		"OnboardingState":       {opts.OnboardingState},
	}
	res, err := c.Req(&request.Options{
		Method:  "POST",
		Path:    "/instances/oauth_client",
		Queries: q,
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var client map[string]interface{}
	if err = json.NewDecoder(res.Body).Decode(&client); err != nil {
		return nil, err
	}
	return client, nil
}

// Updates launch the updating process of the applications. When no Domain is
// specified, the updates are launched for all the existing instances.
func (c *Client) Updates(opts *UpdatesOptions) error {
	q := url.Values{
		"Domain":             {opts.Domain},
		"DomainsWithContext": {opts.DomainsWithContext},
		"Slugs":              {strings.Join(opts.Slugs, ",")},
		"ForceRegistry":      {strconv.FormatBool(opts.ForceRegistry)},
		"OnlyRegistry":       {strconv.FormatBool(opts.OnlyRegistry)},
	}
	channel, err := c.RealtimeClient(RealtimeOptions{
		DocTypes: []string{"io.cozy.jobs", "io.cozy.jobs.logs"},
	})
	if err != nil {
		return err
	}
	defer func() {
		if opts.Logs != nil {
			close(opts.Logs)
		}
		channel.Close()
	}()
	res, err := c.Req(&request.Options{
		Method:  "POST",
		Path:    "/instances/updates",
		Queries: q,
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var job struct {
		ID    string `json:"_id"`
		State string `json:"state"`
		Error string `json:"error"`
	}
	if err = json.NewDecoder(res.Body).Decode(&job); err != nil {
		return err
	}
	for evt := range channel.Channel() {
		if evt.Event == "error" {
			return fmt.Errorf("realtime: %s", evt.Payload.Title)
		}
		if evt.Payload.ID != job.ID {
			continue
		}
		switch evt.Payload.Type {
		case "io.cozy.jobs":
			if err = json.Unmarshal(evt.Payload.Doc, &job); err != nil {
				return err
			}
			if job.State == "errored" {
				return fmt.Errorf("error executing updates: %s", job.Error)
			}
			if job.State == "done" {
				return nil
			}
		case "io.cozy.jobs.logs":
			if opts.Logs != nil {
				var log JobLog
				if err = json.Unmarshal(evt.Payload.Doc, &log); err != nil {
					return err
				}
				opts.Logs <- &log
			}
		}
	}
	return err
}

// Export launch the creation of a tarball to export data from an instance.
func (c *Client) Export(domain string) error {
	if !validDomain(domain) {
		return fmt.Errorf("Invalid domain: %s", domain)
	}
	_, err := c.Req(&request.Options{
		Method:     "POST",
		Path:       "/instances/" + url.PathEscape(domain) + "/export",
		NoResponse: true,
	})
	return err
}

// Import launch the import of a tarball with data to put in an instance.
func (c *Client) Import(domain string, opts *ImportOptions) error {
	if !validDomain(domain) {
		return fmt.Errorf("Invalid domain: %s", domain)
	}
	q := url.Values{
		"filename":    {opts.Filename},
		"destination": {opts.Destination},
	}
	_, err := c.Req(&request.Options{
		Method:     "POST",
		Path:       "/instances/" + url.PathEscape(domain) + "/import",
		Queries:    q,
		NoResponse: true,
	})
	return err
}

// RebuildRedis puts the triggers in redis.
func (c *Client) RebuildRedis() error {
	_, err := c.Req(&request.Options{
		Method:     "POST",
		Path:       "/instances/redis",
		NoResponse: true,
	})
	return err
}

// DiskUsage returns the information about disk usage and quota
func (c *Client) DiskUsage(domain string, includeTrash bool) (map[string]interface{}, error) {
	var q map[string][]string
	if includeTrash {
		q = url.Values{
			"include": {"trash"},
		}
	}

	res, err := c.Req(&request.Options{
		Method:  "GET",
		Path:    "/instances/" + url.PathEscape(domain) + "/disk-usage",
		Queries: q,
	})
	if err != nil {
		return nil, err
	}
	var info map[string]interface{}
	if err = json.NewDecoder(res.Body).Decode(&info); err != nil {
		return nil, err
	}
	return info, nil
}

func readInstance(res *http.Response) (*Instance, error) {
	in := &Instance{}
	if err := readJSONAPI(res.Body, &in); err != nil {
		return nil, err
	}
	return in, nil
}

func validDomain(domain string) bool {
	return !strings.ContainsAny(domain, " /?#@\t\r\n")
}
