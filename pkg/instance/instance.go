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
	"github.com/cozy/cozy-stack/pkg/settings"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/spf13/afero"
)

const (
	registerTokenLen = 16
	sessionSecretLen = 64
	oauthSecretLen   = 128
)

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
	StorageURL string `json:"storage"`        // Where the binaries are persisted

	// PassphraseHash is a hash of the user's passphrase. For more informations,
	// see crypto.GenerateFromPassphrase.
	PassphraseHash []byte `json:"passphraseHash,omitempty"`

	// Secure assets

	// Register token is used on registration to prevent from stealing instances
	// waiting for registration. The registerToken secret is only shared (in
	// clear) with the instance's user.
	RegisterToken []byte `json:"registerToken,omitempty"`
	// SessionSecret is used to authenticate session cookies
	SessionSecret []byte `json:"sessionSecret,omitempty"`
	// OAuthSecret is used to authenticate OAuth2 token
	OAuthSecret []byte `json:"oauthSecret,omitempty"`

	storage afero.Fs
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

// SelfLink is used to generate a JSON-API link for the instance
func (i *Instance) SelfLink() string {
	return "/instances/" + i.DocID
}

// Relationships is used to generate the content relationship in JSON-API format
func (i *Instance) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the jsonapi.Object interface
func (i *Instance) Included() []jsonapi.Object {
	return nil
}

// Addr returns the full address of the domain of the instance
func (i *Instance) Addr() string {
	if config.IsDevRelease() && i.Domain == "dev" {
		return "localhost:8080"
	}
	return i.Domain
}

// SubDomain returns the full url for a subdomain of this instance
// useful with apps slugs
func (i *Instance) SubDomain(s string) string {
	if config.GetConfig().Subdomains == config.NestedSubdomains {
		return "https://" + s + "." + i.Addr() + "/"
	}
	parts := strings.SplitN(i.Addr(), ".", 2)
	return "https://" + parts[0] + "-" + s + "." + parts[1] + "/"
}

// PageURL returns the full URL for a page on the cozy stack
func (i *Instance) PageURL(page string) string {
	return "https://" + i.Domain + page
}

// ensure Instance implements couchdb.Doc & vfs.Context
var _ couchdb.Doc = (*Instance)(nil)
var _ vfs.Context = (*Instance)(nil)

// CreateInCouchdb create the instance doc in the global database
func (i *Instance) createInCouchdb() (err error) {
	if _, err = Get(i.Domain); err == nil {
		return ErrExists
	}
	if err != nil && err != ErrNotFound {
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
			rootFs.RemoveAll(domainURL.Path)
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

// createFSIndexes creates the index needed by VFS
func (i *Instance) createFSIndexes() error {
	for _, index := range vfs.Indexes {
		err := couchdb.DefineIndex(i, consts.Files, index)
		if err != nil {
			return err
		}
	}
	return nil
}

// createAppsDB creates the database needed for Apps
func (i *Instance) createAppsDB() error {
	return couchdb.CreateDB(i, consts.Manifests)
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

// Create build an instance and .Create it
func Create(domain string, locale string, apps []string) (*Instance, error) {
	if strings.ContainsAny(domain, vfs.ForbiddenFilenameChars) || domain == ".." || domain == "." {
		return nil, ErrIllegalDomain
	}

	if config.GetConfig().Subdomains == config.NestedSubdomains {
		parts := strings.SplitN(domain, ".", 2)
		if strings.Contains(parts[0], "-") {
			return nil, ErrIllegalDomain
		}
	}

	i := new(Instance)

	i.Domain = domain
	i.StorageURL = config.BuildRelFsURL(domain).String()

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

	err = i.createSettings()
	if err != nil {
		return nil, err
	}

	err = i.createFSIndexes()
	if err != nil {
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
	if config.IsDevRelease() {
		if domain == "" || strings.Contains(domain, "127.0.0.1") || strings.Contains(domain, "localhost") {
			domain = "dev"
		}
	}

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

// Prefix returns the prefix to use in database naming for the
// current instance
func (i *Instance) Prefix() string {
	return i.Domain + "/"
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
	if err := crypto.CompareHashAndPassphrase(i.PassphraseHash, current); err != nil {
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
	err := crypto.CompareHashAndPassphrase(i.PassphraseHash, pass)
	if err != nil {
		return err
	}

	newhash, err := crypto.UpdateHash(i.PassphraseHash, pass)
	if err == nil {
		i.PassphraseHash = newhash
		err := couchdb.UpdateDoc(couchdb.GlobalDB, i)
		if err != nil {
			log.Info("Failed to update hash in db", err)
		}
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
