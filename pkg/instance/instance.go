package instance

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/settings"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/spf13/afero"
)

/* #nosec */
const (
	registerTokenLen = 16
	sessionSecretLen = 64
	oauthSecretLen   = 128
)

// DefaultLocale is the default locale when creating an instance
const DefaultLocale = "en"

var (
	// ErrNotFound is used when the seeked instance was not found
	ErrNotFound = errors.New("Instance not found")
	// ErrExists is used the instance already exists
	ErrExists = errors.New("Instance already exists")
	// ErrIllegalDomain is used when the domain named contains illegal characters
	ErrIllegalDomain = errors.New("Domain name contains illegal characters")
	// ErrMissingToken is returned by RegisterPassphrase if token is empty
	ErrMissingToken = errors.New("Empty register token")
	// ErrInvalidToken is returned by RegisterPassphrase if token is invalid
	ErrInvalidToken = errors.New("Invalid register token")
	// ErrMissingPassphrase is returned when the new passphrase is missing
	ErrMissingPassphrase = errors.New("Missing new passphrase")
	// ErrInvalidPassphrase is returned when the passphrase is invalid
	ErrInvalidPassphrase = errors.New("Invalid passphrase")
)

// An Instance has the informations relatives to the logical cozy instance,
// like the domain, the locale or the access to the databases and files storage
// It is a couchdb.Doc to be persisted in couchdb.
type Instance struct {
	DocID      string `json:"_id,omitempty"`  // couchdb _id
	DocRev     string `json:"_rev,omitempty"` // couchdb _rev
	Domain     string `json:"domain"`         // The main DNS domain, like example.cozycloud.cc
	Locale     string `json:"locale"`         // The locale used on the server
	StorageURL string `json:"storage"`        // Where the binaries are persisted
	Dev        bool   `json:"dev"`            // Whether or not the instance is for development

	// PassphraseHash is a hash of the user's passphrase. For more informations,
	// see crypto.GenerateFromPassphrase.
	PassphraseHash []byte `json:"passphrase_hash,omitempty"`

	// Secure assets

	// Register token is used on registration to prevent from stealing instances
	// waiting for registration. The registerToken secret is only shared (in
	// clear) with the instance's user.
	RegisterToken []byte `json:"register_token,omitempty"`
	// SessionSecret is used to authenticate session cookies
	SessionSecret []byte `json:"session_secret,omitempty"`
	// OAuthSecret is used to authenticate OAuth2 token
	OAuthSecret []byte `json:"oauth_secret,omitempty"`

	storage afero.Fs
}

// Options holds the parameters to create a new instance.
type Options struct {
	Domain   string
	Locale   string
	Timezone string
	Email    string
	Apps     []string
	Dev      bool
}

// DocType implements couchdb.Doc
func (i *Instance) DocType() string { return consts.Instances }

// ID implements couchdb.Doc
func (i *Instance) ID() string { return i.DocID }

// SetID implements couchdb.Doc
func (i *Instance) SetID(v string) { i.DocID = v }

// Rev implements couchdb.Doc
func (i *Instance) Rev() string { return i.DocRev }

// SetRev implements couchdb.Doc
func (i *Instance) SetRev(v string) { i.DocRev = v }

// Links is used to generate a JSON-API link for the instance
func (i *Instance) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/instances/" + i.DocID}
}

// Relationships is used to generate the content relationship in JSON-API format
func (i *Instance) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the jsonapi.Object interface
func (i *Instance) Included() []jsonapi.Object {
	return nil
}

// settings is a struct used for the settings of an instance
type instanceSettings struct {
	Timezone string `json:"tz,omitempty"`
	Email    string `json:"email,omitempty"`
}

func (s *instanceSettings) ID() string      { return consts.InstanceSettingsID }
func (s *instanceSettings) Rev() string     { return "" }
func (s *instanceSettings) DocType() string { return consts.Settings }
func (s *instanceSettings) SetID(_ string)  {}
func (s *instanceSettings) SetRev(_ string) {}

// FS returns the afero storage provider where the binaries for
// the current instance are persisted
func (i *Instance) FS() afero.Fs {
	if i.storage == nil {
		if err := i.makeStorageFs(); err != nil {
			panic(err)
		}
	}
	return i.storage
}

// StartJobSystem creates all the resources necessary for the instance's job
// system to work properly.
func (i *Instance) StartJobSystem() error {
	broker := jobs.NewMemBroker(i.Domain, jobs.GetWorkersList())
	scheduler := jobs.NewMemScheduler(i.Domain, jobs.NewTriggerCouchStorage(i))
	return scheduler.Start(broker)
}

// StopJobSystem stops all the resources used by the job system associated with
// the instance.
func (i *Instance) StopJobSystem() error {
	// TODO
	return nil
}

// JobsBroker returns the jobs broker associated with the instance
func (i *Instance) JobsBroker() jobs.Broker {
	return jobs.GetMemBroker(i.Domain)
}

// JobsScheduler returns the jobs scheduler associated with the instance
func (i *Instance) JobsScheduler() jobs.Scheduler {
	return jobs.GetMemScheduler(i.Domain)
}

// Prefix returns the prefix to use in database naming for the
// current instance
func (i *Instance) Prefix() string {
	return i.Domain + "/"
}

// Scheme returns the scheme used for URLs. It is https by default and http
// for development instances.
func (i *Instance) Scheme() string {
	if i.Dev {
		return "http"
	}
	return "https"
}

// SubDomain returns the full url for a subdomain of this instance
// useful with apps slugs
func (i *Instance) SubDomain(s string) string {
	var domain string
	if config.GetConfig().Subdomains == config.NestedSubdomains {
		domain = s + "." + i.Domain
	} else {
		parts := strings.SplitN(i.Domain, ".", 2)
		domain = parts[0] + "-" + s + "." + parts[1]
	}
	u := url.URL{
		Scheme: i.Scheme(),
		Host:   domain,
		Path:   "/",
	}
	return u.String()
}

// FromURL normalizes a given url with the scheme and domain of the instance.
func (i *Instance) FromURL(u *url.URL) string {
	u2 := url.URL{
		Scheme:   i.Scheme(),
		Host:     i.Domain,
		Path:     u.Path,
		RawQuery: u.RawQuery,
		Fragment: u.Fragment,
	}
	return u2.String()
}

// PageURL returns the full URL for a path on the cozy stack
func (i *Instance) PageURL(path string, queries url.Values) string {
	var query string
	if queries != nil {
		query = queries.Encode()
	}
	u := url.URL{
		Scheme:   i.Scheme(),
		Host:     i.Domain,
		Path:     path,
		RawQuery: query,
	}
	return u.String()
}

// createInCouchdb creates the instance doc in the global database
func (i *Instance) createInCouchdb() error {
	_, err := Get(i.Domain)
	if err == nil {
		return ErrExists
	}
	if err != ErrNotFound {
		return err
	}
	err = couchdb.CreateDoc(couchdb.GlobalDB, i)
	if err != nil {
		return err
	}
	byDomain := mango.IndexOnFields("domain")
	return couchdb.DefineIndex(couchdb.GlobalDB, consts.Instances, byDomain)
}

// createRootDir creates the root directory for this instance
func (i *Instance) createRootDir() error {
	rootFsURL := config.BuildAbsFsURL("/")
	domainURL := config.BuildRelFsURL(i.Domain)

	rootFs, err := createFs(rootFsURL)
	if err != nil {
		return err
	}

	if err = rootFs.MkdirAll(domainURL.Path, 0755); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			if rmerr := rootFs.RemoveAll(domainURL.Path); rmerr != nil {
				log.Warn("[instance] Could not remove the instance directory")
			}
		}
	}()

	if err = vfs.CreateRootDirDoc(i); err != nil {
		return err
	}

	if err = vfs.CreateTrashDir(i); err != nil {
		return err
	}

	return nil
}

// createFSIndexes creates the indexes needed by VFS
func (i *Instance) createFSIndexes() error {
	for _, index := range vfs.Indexes {
		err := couchdb.DefineIndex(i, consts.Files, index)
		if err != nil {
			return err
		}
	}
	return couchdb.DefineViews(i, consts.Files, vfs.Views)
}

// createAppsDB creates the database needed for Apps
func (i *Instance) createAppsDB() error {
	return couchdb.CreateDB(i, consts.Manifests)
}

// createClientsDB creates the database needed for OAuth 2 clients
func (i *Instance) createClientsDB() error {
	return couchdb.CreateDB(i, consts.OAuthClients)
}

// createSettings creates the settings database and some documents like the
// default theme
func (i *Instance) createSettings() error {
	err := couchdb.CreateDB(i, consts.Settings)
	if err != nil {
		return err
	}
	return settings.CreateDefaultTheme(i)
}

func (i *Instance) createPermissionsDB() error {
	err := couchdb.CreateDB(i, consts.Permissions)

	if err != nil {
		return err
	}

	return couchdb.DefineIndex(i, consts.Permissions, permissions.Index)

}

// Create builds an instance and initializes it
func Create(opts *Options) (*Instance, error) {
	domain := opts.Domain
	if strings.ContainsAny(domain, vfs.ForbiddenFilenameChars) || domain == ".." || domain == "." {
		return nil, ErrIllegalDomain
	}

	if strings.ContainsAny(domain, " /?#@\t\r\n") {
		return nil, ErrIllegalDomain
	}

	if config.GetConfig().Subdomains == config.FlatSubdomains {
		parts := strings.SplitN(domain, ".", 2)
		if strings.Contains(parts[0], "-") {
			return nil, ErrIllegalDomain
		}
	}

	locale := opts.Locale
	if locale == "" {
		locale = DefaultLocale
	}

	i := new(Instance)

	i.Locale = locale
	i.Domain = domain
	i.StorageURL = config.BuildRelFsURL(domain).String()

	i.Dev = opts.Dev

	i.PassphraseHash = nil
	i.RegisterToken = crypto.GenerateRandomBytes(registerTokenLen)
	i.SessionSecret = crypto.GenerateRandomBytes(sessionSecretLen)
	i.OAuthSecret = crypto.GenerateRandomBytes(oauthSecretLen)

	var err error
	err = i.makeStorageFs()
	if err != nil {
		return nil, err
	}

	err = i.createInCouchdb()
	if err != nil {
		return nil, err
	}

	err = i.createRootDir()
	if err != nil {
		return nil, err
	}

	err = i.createAppsDB()
	if err != nil {
		return nil, err
	}

	err = i.createClientsDB()
	if err != nil {
		return nil, err
	}

	err = i.createSettings()
	if err != nil {
		return nil, err
	}

	err = i.createFSIndexes()
	if err != nil {
		return nil, err
	}

	err = i.StartJobSystem()
	if err != nil {
		return nil, err
	}

	err = i.createPermissionsDB()
	if err != nil {
		return nil, err
	}

	doc := &instanceSettings{
		Timezone: opts.Timezone,
		Email:    opts.Email,
	}
	if err = couchdb.CreateNamedDoc(i, doc); err != nil {
		return nil, err
	}

	// TODO atomicity with defer
	// TODO figure out what to do with locale
	// TODO install apps

	return i, nil
}

func (i *Instance) makeStorageFs() error {
	u, err := url.Parse(i.StorageURL)
	if err != nil {
		return err
	}
	i.storage, err = createFs(u)
	return err
}

// Get retrieves the instance for a request by its host.
func Get(domain string) (*Instance, error) {
	var instances []*Instance
	req := &couchdb.FindRequest{
		Selector: mango.Equal("domain", domain),
		Limit:    1,
	}
	err := couchdb.FindDocs(couchdb.GlobalDB, consts.Instances, req, &instances)
	if couchdb.IsNoDatabaseError(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, ErrNotFound
	}

	err = instances[0].makeStorageFs()
	if err != nil {
		return nil, err
	}

	return instances[0], nil
}

// List returns the list of declared instances.
//
// TODO: pagination
// TODO: don't return the design docs
func List() ([]*Instance, error) {
	var docs []*Instance
	req := &couchdb.AllDocsRequest{Limit: 100}
	err := couchdb.GetAllDocs(couchdb.GlobalDB, consts.Instances, req, &docs)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// Destroy is used to remove the instance. All the data linked to this
// instance will be permanently deleted.
func Destroy(domain string) (*Instance, error) {
	i, err := Get(domain)
	if err != nil {
		return nil, err
	}

	if err = couchdb.DeleteDoc(couchdb.GlobalDB, i); err != nil {
		return nil, err
	}

	if err = couchdb.DeleteAllDBs(i); err != nil {
		return nil, err
	}

	if err = i.StopJobSystem(); err != nil {
		return nil, err
	}

	rootFsURL := config.BuildAbsFsURL("/")
	domainURL := config.BuildRelFsURL(i.Domain)

	rootFs, err := createFs(rootFsURL)
	if err != nil {
		return nil, err
	}

	if err = rootFs.RemoveAll(domainURL.Path); err != nil {
		return nil, err
	}

	return i, nil
}

// RegisterPassphrase replace the instance registerToken by a passphrase
func (i *Instance) RegisterPassphrase(pass, tok []byte) error {
	if len(pass) == 0 {
		return ErrMissingPassphrase
	}
	if len(i.RegisterToken) == 0 {
		return ErrMissingToken
	}
	if subtle.ConstantTimeCompare(i.RegisterToken, tok) != 1 {
		return ErrInvalidToken
	}

	hash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		return err
	}

	i.RegisterToken = nil
	i.setPassphraseAndSecret(hash)

	return couchdb.UpdateDoc(couchdb.GlobalDB, i)
}

// UpdatePassphrase replace the passphrase
func (i *Instance) UpdatePassphrase(pass, current []byte) error {
	if len(pass) == 0 {
		return ErrMissingPassphrase
	}

	// the needUpdate flag is not checked against since the passphrase will be
	// regenerated with updated parameters just after, if the passphrase match.
	_, err := crypto.CompareHashAndPassphrase(i.PassphraseHash, current)
	if err != nil {
		return ErrInvalidPassphrase
	}

	hash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		return err
	}
	i.setPassphraseAndSecret(hash)

	return couchdb.UpdateDoc(couchdb.GlobalDB, i)
}

func (i *Instance) setPassphraseAndSecret(hash []byte) {
	i.PassphraseHash = hash
	i.SessionSecret = crypto.GenerateRandomBytes(sessionSecretLen)
}

// CheckPassphrase confirm an instance passport
func (i *Instance) CheckPassphrase(pass []byte) error {
	if len(pass) == 0 {
		return ErrMissingPassphrase
	}

	needUpdate, err := crypto.CompareHashAndPassphrase(i.PassphraseHash, pass)
	if err != nil {
		return err
	}

	if !needUpdate {
		return nil
	}

	newHash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		return err
	}

	i.PassphraseHash = newHash
	err = couchdb.UpdateDoc(couchdb.GlobalDB, i)
	if err != nil {
		log.Error("[instance] Failed to update hash in db", err)
	}

	return nil
}

func createFs(u *url.URL) (fs afero.Fs, err error) {
	switch u.Scheme {
	case "file":
		fs = afero.NewBasePathFs(afero.NewOsFs(), u.Path)
	case "mem":
		fs = afero.NewMemMapFs()
	default:
		err = fmt.Errorf("Unknown storage provider: %v", u.Scheme)
	}
	return
}

// ensure Instance implements couchdb.Doc & vfs.Context
var (
	_ couchdb.Doc = &Instance{}
	_ vfs.Context = &Instance{}
)
