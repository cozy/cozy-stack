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
		ContextName          string    `json:"context,omitempty"`
		TOSSigned            string    `json:"tos,omitempty"`
		TOSLatest            string    `json:"tos_latest,omitempty"`
		AuthMode             int       `json:"auth_mode,omitempty"`
		NoAutoUpdate         bool      `json:"no_auto_update,omitempty"`
		Blocked              bool      `json:"blocked,omitempty"`
		Dev                  bool      `json:"dev"`
		OnboardingFinished   bool      `json:"onboarding_finished"`
		BytesDiskQuota       int64     `json:"disk_quota,string,omitempty"`
		IndexViewsVersion    int       `json:"indexes_version"`
		SwiftCluster         int       `json:"swift_cluster,omitempty"`
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
	TOSSigned          string
	TOSLatest          string
	Timezone           string
	ContextName        string
	Email              string
	PublicName         string
	Settings           string
	SwiftCluster       int
	DiskQuota          int64
	Apps               []string
	Passphrase         string
	Debug              *bool
	Blocked            *bool
	OnboardingFinished *bool
	Dev                bool
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
	Domain      string
	RedirectURI string
	ClientName  string
	SoftwareID  string
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
	Filename      string
	Destination   string
	IncreaseQuota bool
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
		"Domain":       {opts.Domain},
		"Locale":       {opts.Locale},
		"UUID":         {opts.UUID},
		"TOSSigned":    {opts.TOSSigned},
		"Timezone":     {opts.Timezone},
		"ContextName":  {opts.ContextName},
		"Email":        {opts.Email},
		"PublicName":   {opts.PublicName},
		"Settings":     {opts.Settings},
		"SwiftCluster": {strconv.Itoa(opts.SwiftCluster)},
		"DiskQuota":    {strconv.FormatInt(opts.DiskQuota, 10)},
		"Apps":         {strings.Join(opts.Apps, ",")},
		"Passphrase":   {opts.Passphrase},
		"Dev":          {strconv.FormatBool(opts.Dev)},
	}
	if opts.DomainAliases != nil {
		q.Add("DomainAliases", strings.Join(opts.DomainAliases, ","))
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
		"Locale":       {opts.Locale},
		"UUID":         {opts.UUID},
		"TOSSigned":    {opts.TOSSigned},
		"TOSLatest":    {opts.TOSLatest},
		"Timezone":     {opts.Timezone},
		"ContextName":  {opts.ContextName},
		"Email":        {opts.Email},
		"PublicName":   {opts.PublicName},
		"Settings":     {opts.Settings},
		"SwiftCluster": {strconv.Itoa(opts.SwiftCluster)},
		"DiskQuota":    {strconv.FormatInt(opts.DiskQuota, 10)},
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

// FsckInstance returns the list of the inconsistencies in the VFS.
func (c *Client) FsckInstance(domain string, prune, dryRun bool) ([]map[string]string, error) {
	if !validDomain(domain) {
		return nil, fmt.Errorf("Invalid domain: %s", domain)
	}
	res, err := c.Req(&request.Options{
		Method: "GET",
		Path:   "/instances/" + url.PathEscape(domain) + "/fsck",
		Queries: url.Values{
			"Prune":  {strconv.FormatBool(prune)},
			"DryRun": {strconv.FormatBool(dryRun)},
		},
	})
	if err != nil {
		return nil, err
	}
	var list []map[string]string
	if err = json.NewDecoder(res.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetToken is used to generate a toke with the specified options.
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
		"Domain":      {opts.Domain},
		"RedirectURI": {opts.RedirectURI},
		"ClientName":  {opts.ClientName},
		"SoftwareID":  {opts.SoftwareID},
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
		"filename":       {opts.Filename},
		"destination":    {opts.Destination},
		"increase_quota": {strconv.FormatBool(opts.IncreaseQuota)},
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
