package instance

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/globals"
	"github.com/cozy/cozy-stack/pkg/hooks"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsafero"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsswift"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/leonelquinteros/gotext"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

/* #nosec */
const (
	RegisterTokenLen      = 16
	PasswordResetTokenLen = 16
	SessionSecretLen      = 64
	OauthSecretLen        = 128
)

// passwordResetValidityDuration is the validity duration of the passphrase
// reset token.
var passwordResetValidityDuration = 15 * time.Minute

var twoFactorTOTPOptions = totp.ValidateOpts{
	Period:    30, // 30s
	Skew:      4,  // 30s +- 4*30s = [-2min; 2,5min]
	Digits:    otp.DigitsSix,
	Algorithm: otp.AlgorithmSHA256,
}

// DefaultLocale is the default locale when creating an instance
const DefaultLocale = "en"

const illegalChars = " /,;&?#@|='\"\t\r\n\x00"
const illegalFirstChars = "0123456789."

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
	// ErrContextNotFound is returned when the instance has no context
	ErrContextNotFound = errors.New("Context not found")
	// ErrResetAlreadyRequested is returned when a passphrase reset token is already set and valid
	ErrResetAlreadyRequested = errors.New("The passphrase reset has already been requested")
)

// An Instance has the informations relatives to the logical cozy instance,
// like the domain, the locale or the access to the databases and files storage
// It is a couchdb.Doc to be persisted in couchdb.
type Instance struct {
	DocID        string   `json:"_id,omitempty"`  // couchdb _id
	DocRev       string   `json:"_rev,omitempty"` // couchdb _rev
	Domain       string   `json:"domain"`         // The main DNS domain, like example.cozycloud.cc
	Locale       string   `json:"locale"`         // The locale used on the server
	AuthMode     AuthMode `json:"auth_mode"`
	NoAutoUpdate bool     `json:"no_auto_update,omitempty"` // Whether or not the instance has auto updates for its applications
	Dev          bool     `json:"dev,omitempty"`            // Whether or not the instance is for development

	OnboardingFinished bool `json:"onboarding_finished"` // Whether or not the onboarding is complete

	BytesDiskQuota int64 `json:"disk_quota,string,omitempty"` // The total size in bytes allowed to the user

	IndexViewsVersion int `json:"indexes_version"`

	// PassphraseHash is a hash of the user's passphrase. For more informations,
	// see crypto.GenerateFromPassphrase.
	PassphraseHash       []byte    `json:"passphrase_hash,omitempty"`
	PassphraseResetToken []byte    `json:"passphrase_reset_token"`
	PassphraseResetTime  time.Time `json:"passphrase_reset_time"`

	// Secure assets

	// Register token is used on registration to prevent from stealing instances
	// waiting for registration. The registerToken secret is only shared (in
	// clear) with the instance's user.
	RegisterToken []byte `json:"register_token,omitempty"`
	// SessionSecret is used to authenticate session cookies
	SessionSecret []byte `json:"session_secret,omitempty"`
	// OAuthSecret is used to authenticate OAuth2 token
	OAuthSecret []byte `json:"oauth_secret,omitempty"`
	// CLISecret is used to authenticate request from the CLI
	CLISecret []byte `json:"cli_secret,omitempty"`

	vfs vfs.VFS
}

// Options holds the parameters to create a new instance.
type Options struct {
	Domain    string
	Locale    string
	DiskQuota int64
	Apps      []string
	Dev       bool
	Settings  couchdb.JSONDoc
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

// Clone implements couchdb.Doc
func (i *Instance) Clone() couchdb.Doc {
	cloned := *i

	cloned.PassphraseHash = make([]byte, len(i.PassphraseHash))
	copy(cloned.PassphraseHash, i.PassphraseHash)

	cloned.PassphraseResetToken = make([]byte, len(i.PassphraseResetToken))
	copy(cloned.PassphraseResetToken, i.PassphraseResetToken)

	cloned.RegisterToken = make([]byte, len(i.RegisterToken))
	copy(cloned.RegisterToken, i.RegisterToken)

	cloned.SessionSecret = make([]byte, len(i.SessionSecret))
	copy(cloned.SessionSecret, i.SessionSecret)

	cloned.OAuthSecret = make([]byte, len(i.OAuthSecret))
	copy(cloned.OAuthSecret, i.OAuthSecret)

	cloned.CLISecret = make([]byte, len(i.CLISecret))
	copy(cloned.CLISecret, i.CLISecret)
	return &cloned
}

// Prefix returns the prefix to use in database naming for the
// current instance
func (i *Instance) Prefix() string {
	return i.Domain
}

// Logger returns the logger associated with the instance
func (i *Instance) Logger() *logrus.Entry {
	return logger.WithDomain(i.Domain)
}

// VFS returns the storage provider where the binaries for the current instance
// are persisted
func (i *Instance) VFS() vfs.VFS {
	if i.vfs == nil {
		panic("instance: calling VFS() before makeVFS()")
	}
	return i.vfs
}

func (i *Instance) makeVFS() error {
	if i.vfs != nil {
		return nil
	}
	fsURL := config.FsURL()
	mutex := lock.ReadWrite(i.Domain)
	index := vfs.NewCouchdbIndexer(i)
	disk := vfs.DiskThresholder(i)
	var err error
	switch fsURL.Scheme {
	case config.SchemeFile, config.SchemeMem:
		i.vfs, err = vfsafero.New(index, disk, mutex, fsURL, i.DirName())
	case config.SchemeSwift:
		i.vfs, err = vfsswift.New(index, disk, mutex, i.Domain)
	default:
		err = fmt.Errorf("instance: unknown storage provider %s", fsURL.Scheme)
	}
	return err
}

// AppsCopier returns the application copier associated with the specified
// application type
func (i *Instance) AppsCopier(appsType apps.AppType) apps.Copier {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile, config.SchemeMem:
		var baseDirName string
		switch appsType {
		case apps.Webapp:
			baseDirName = vfs.WebappsDirName
		case apps.Konnector:
			baseDirName = vfs.KonnectorsDirName
		}
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), baseDirName))
		return apps.NewAferoCopier(baseFS)
	case config.SchemeSwift:
		return apps.NewSwiftCopier(config.GetSwiftConnection(), appsType)
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
}

// AppsFileServer returns the web-application file server associated to this
// instance.
func (i *Instance) AppsFileServer() apps.FileServer {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile, config.SchemeMem:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.WebappsDirName))
		return apps.NewAferoFileServer(baseFS, nil)
	case config.SchemeSwift:
		return apps.NewSwiftFileServer(config.GetSwiftConnection(), apps.Webapp)
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
}

// KonnectorsFileServer returns the web-application file server associated to this
// instance.
func (i *Instance) KonnectorsFileServer() apps.FileServer {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile, config.SchemeMem:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.KonnectorsDirName))
		return apps.NewAferoFileServer(baseFS, nil)
	case config.SchemeSwift:
		return apps.NewSwiftFileServer(config.GetSwiftConnection(), apps.Konnector)
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
}

// ThumbsFS returns the hidden filesystem for storing the thumbnails of the
// photos/image
func (i *Instance) ThumbsFS() vfs.Thumbser {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile, config.SchemeMem:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.ThumbsDirName))
		return vfsafero.NewThumbsFs(baseFS)
	case config.SchemeSwift:
		return vfsswift.NewThumbsFs(config.GetSwiftConnection(), i.Domain)
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
}

// SettingsDocument returns the document with the settings of this instance
func (i *Instance) SettingsDocument() (*couchdb.JSONDoc, error) {
	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(i, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return nil, err
	}
	doc.Type = consts.Settings
	return doc, nil
}

// Context returns the map from the config that matches the context of this instance
func (i *Instance) Context() (map[string]interface{}, error) {
	doc, err := i.SettingsDocument()
	if err != nil {
		return nil, err
	}
	ctx, ok := doc.M["context"].(string)
	if !ok {
		ctx = "default"
	}
	context, ok := config.GetConfig().Contexts[ctx].(map[string]interface{})
	if !ok {
		return nil, ErrContextNotFound
	}
	return context, nil
}

// Registries returns the list of registries associated with the instance.
func (i *Instance) Registries() ([]*url.URL, error) {
	doc, err := i.SettingsDocument()
	if err != nil {
		return nil, err
	}
	ctx, ok := doc.M["context"].(string)
	if !ok {
		ctx = "default"
	}
	registries := config.GetConfig().Registries
	regs, ok := registries[ctx]
	if !ok {
		regs, ok = registries["default"]
		if !ok {
			regs = make([]*url.URL, 0)
		}
	}
	return regs, nil
}

// DiskQuota returns the number of bytes allowed on the disk to the user.
func (i *Instance) DiskQuota() int64 {
	return i.BytesDiskQuota
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
func (i *Instance) SubDomain(s string) *url.URL {
	var domain string
	if config.GetConfig().Subdomains == config.NestedSubdomains {
		domain = s + "." + i.Domain
	} else {
		parts := strings.SplitN(i.Domain, ".", 2)
		domain = parts[0] + "-" + s + "." + parts[1]
	}
	return &url.URL{
		Scheme: i.Scheme(),
		Host:   domain,
		Path:   "/",
	}
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

// PublicName returns the settings' public name or a default one if missing
func (i *Instance) PublicName() (string, error) {
	doc, err := i.SettingsDocument()
	if err != nil {
		return "", err
	}
	publicName, _ := doc.M["public_name"].(string)
	// if the public name is not defined, use the instance's domain
	if publicName == "" {
		split := strings.Split(i.Domain, ".")
		publicName = split[0]
	}
	return publicName, nil
}

func (i *Instance) redirection(key, defaultSlug string) *url.URL {
	context, err := i.Context()
	if err != nil {
		return i.SubDomain(defaultSlug)
	}
	redirect, ok := context[key].(string)
	if !ok {
		return i.SubDomain(defaultSlug)
	}
	splits := strings.SplitN(redirect, "#", 2)
	parts := strings.SplitN(splits[0], "/", 2)
	u := i.SubDomain(parts[0])
	if len(parts) == 2 {
		u.Path = parts[1]
	}
	if len(splits) == 2 {
		u.Fragment = splits[1]
	}
	return u
}

// DefaultRedirection returns the URL where to redirect the user afer login
// (and in most other cases where we need a redirection URL)
func (i *Instance) DefaultRedirection() *url.URL {
	if !i.OnboardingFinished {
		return i.SubDomain(consts.OnboardingSlug)
	}
	return i.redirection("default_redirection", consts.DriveSlug)
}

// OnboardedRedirection returns the URL where to redirect the user after
// onboarding
func (i *Instance) OnboardedRedirection() *url.URL {
	return i.redirection("onboarded_redirection", consts.DriveSlug)
}

func (i *Instance) installApp(slug string) error {
	source, ok := consts.AppsRegistry[slug]
	if !ok {
		return errors.New("Unknown app")
	}
	inst, err := apps.NewInstaller(i, i.AppsCopier(apps.Webapp), &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		SourceURL: source,
		Slug:      slug,
	})
	if err != nil {
		return err
	}
	_, err = inst.RunSync()
	return err
}

func (i *Instance) defineViewsAndIndex() error {
	if err := couchdb.DefineIndexes(i, consts.Indexes); err != nil {
		return err
	}
	if err := couchdb.DefineViews(i, consts.Views); err != nil {
		return err
	}
	i.IndexViewsVersion = consts.IndexViewsVersion
	return nil
}

func (i *Instance) createDefaultFilesTree() error {
	var errf error

	createDir := func(dir *vfs.DirDoc, err error) (*vfs.DirDoc, error) {
		if err != nil {
			errf = multierror.Append(errf, err)
			return nil, err
		}
		err = i.VFS().CreateDir(dir)
		if err != nil && !os.IsExist(err) {
			errf = multierror.Append(errf, err)
			return nil, err
		}
		return dir, nil
	}

	name := i.Translate("Tree Administrative")
	createDir(vfs.NewDirDocWithPath(name, consts.RootDirID, "/", nil)) // #nosec

	name = i.Translate("Tree Photos")
	photos, err := createDir(vfs.NewDirDocWithPath(name, consts.RootDirID, "/", nil))
	if err == nil {
		name = i.Translate("Tree Uploaded from Cozy Photos")
		createDir(vfs.NewDirDoc(i.VFS(), name, photos.ID(), nil)) // #nosec
		name = i.Translate("Tree Backed up from my mobile")
		createDir(vfs.NewDirDoc(i.VFS(), name, photos.ID(), nil)) // #nosec
	}

	return errf
}

// Create builds an instance and initializes it
func Create(opts *Options) (*Instance, error) {
	domain, err := validateDomain(opts.Domain)
	if err != nil {
		return nil, err
	}
	var i *Instance
	err = hooks.Execute("add-instance", []string{domain}, func() error {
		var err2 error
		i, err2 = CreateWithoutHooks(opts)
		return err2
	})
	return i, err
}

// CreateWithoutHooks builds an instance and initializes it. The difference
// with Create is that script hooks are not executed for this function.
func CreateWithoutHooks(opts *Options) (*Instance, error) {
	domain, err := validateDomain(opts.Domain)
	if err != nil {
		return nil, err
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
	i.BytesDiskQuota = opts.DiskQuota
	i.Dev = opts.Dev
	i.IndexViewsVersion = consts.IndexViewsVersion

	i.PassphraseHash = nil
	i.PassphraseResetToken = nil
	i.PassphraseResetTime = time.Time{}
	i.RegisterToken = crypto.GenerateRandomBytes(RegisterTokenLen)
	i.SessionSecret = crypto.GenerateRandomBytes(SessionSecretLen)
	i.OAuthSecret = crypto.GenerateRandomBytes(OauthSecretLen)
	i.CLISecret = crypto.GenerateRandomBytes(OauthSecretLen)

	if err := couchdb.CreateDB(couchdb.GlobalDB, consts.Instances); !couchdb.IsFileExists(err) {
		if err != nil {
			return nil, err
		}
		if err := couchdb.DefineIndexes(couchdb.GlobalDB, consts.GlobalIndexes); err != nil {
			return nil, err
		}
	}

	if _, err := getFromCouch(i.Domain); err != ErrNotFound {
		if err == nil {
			err = ErrExists
		}
		return nil, err
	}
	if err := couchdb.CreateDoc(couchdb.GlobalDB, i); err != nil {
		return nil, err
	}
	if err := couchdb.CreateDB(i, consts.Files); err != nil {
		return nil, err
	}
	if err := couchdb.CreateDB(i, consts.Apps); err != nil {
		return nil, err
	}
	if err := couchdb.CreateDB(i, consts.Konnectors); err != nil {
		return nil, err
	}
	if err := couchdb.CreateDB(i, consts.OAuthClients); err != nil {
		return nil, err
	}
	if err := couchdb.CreateDB(i, consts.Settings); err != nil {
		return nil, err
	}
	if err := couchdb.CreateDB(i, consts.Permissions); err != nil {
		return nil, err
	}
	if err := couchdb.CreateDB(i, consts.Sharings); err != nil {
		return nil, err
	}
	if err := i.makeVFS(); err != nil {
		return nil, err
	}
	if err := i.VFS().InitFs(); err != nil {
		return nil, err
	}
	if opts.Settings.M == nil {
		opts.Settings.M = make(map[string]interface{})
	}
	opts.Settings.M["_id"] = consts.InstanceSettingsID
	opts.Settings.Type = consts.Settings
	if err := couchdb.CreateNamedDoc(i, opts.Settings); err != nil {
		return nil, err
	}
	if err := i.defineViewsAndIndex(); err != nil {
		return nil, err
	}
	if err := i.createDefaultFilesTree(); err != nil {
		return nil, err
	}
	sched := globals.GetScheduler()
	for _, trigger := range Triggers(i.Domain) {
		t, err := scheduler.NewTrigger(&trigger)
		if err != nil {
			return nil, err
		}
		if err = sched.Add(t); err != nil {
			return nil, err
		}
	}
	for _, app := range opts.Apps {
		if err := i.installApp(app); err != nil {
			i.Logger().Errorf("Failed to install %s: %s", app, err)
		}
	}
	return i, nil
}

// Get retrieves the instance for a request by its host.
func Get(domain string) (*Instance, error) {
	var err error
	domain, err = validateDomain(domain)
	if err != nil {
		return nil, err
	}
	cache := getCache()
	i := cache.Get(domain)
	if i == nil {
		i, err = getFromCouch(domain)
		if err != nil {
			return nil, err
		}
		cache.Set(domain, i)
	}
	if i.IndexViewsVersion != consts.IndexViewsVersion {
		i.Logger().Infof("Indexes outdated: wanted %d; got %d",
			consts.IndexViewsVersion, i.IndexViewsVersion)
		if err = i.defineViewsAndIndex(); err != nil {
			i.Logger().Errorf("Could not re-define indexes and views: %s",
				err.Error())
			return nil, err
		}
		if err = Update(i); err != nil {
			return nil, err
		}
	}
	if err = i.makeVFS(); err != nil {
		return nil, err
	}
	return i, nil
}

func getFromCouch(domain string) (*Instance, error) {
	var instances []*Instance
	req := &couchdb.FindRequest{
		UseIndex: "by-domain",
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
	i := instances[0]
	if err = i.makeVFS(); err != nil {
		return nil, err
	}
	return i, nil
}

var translations = make(map[string]*gotext.Po)

// LoadLocale creates the translation object for a locale from the content of a .po file
func LoadLocale(identifier, rawPO string) {
	po := &gotext.Po{Language: identifier}
	po.Parse(rawPO)
	translations[identifier] = po
}

// Translate is used to translate a string to the locale used on this instance
func (i *Instance) Translate(key string, vars ...interface{}) string {
	if po, ok := translations[i.Locale]; ok {
		translated := po.Get(key, vars...)
		if translated != key && translated != "" {
			return translated
		}
	}
	if po, ok := translations[DefaultLocale]; ok {
		translated := po.Get(key, vars...)
		if translated != key && translated != "" {
			return translated
		}
	}
	i.Logger().Infof("Translation not found for '%s'", key)
	if strings.HasPrefix(key, "Permissions ") {
		key = strings.Replace(key, "Permissions ", "", 1)
	}
	return fmt.Sprintf(key, vars...)
}

// List returns the list of declared instances.
func List() ([]*Instance, error) {
	var all []*Instance
	err := ForeachInstances(func(doc *Instance) error {
		all = append(all, doc)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}

// ForeachInstances execute the given callback for each instances.
func ForeachInstances(fn func(*Instance) error) error {
	return couchdb.ForeachDocs(couchdb.GlobalDB, consts.Instances, func(data []byte) error {
		var doc *Instance
		if err := json.Unmarshal(data, &doc); err != nil {
			return err
		}
		return fn(doc)
	})
}

// Update is used to save changes made to an instance, it will invalidate
// caching
func Update(i *Instance) error {
	getCache().Revoke(i.Domain)
	if err := couchdb.UpdateDoc(couchdb.GlobalDB, i); err != nil {
		i.Logger().Errorf("Could not update: %s", err.Error())
		return err
	}
	return nil
}

// Destroy is used to remove the instance. All the data linked to this
// instance will be permanently deleted.
func Destroy(domain string) error {
	domain, err := validateDomain(domain)
	if err != nil {
		return err
	}
	return hooks.Execute("remove-instance", []string{domain}, func() error {
		return DestroyWithoutHooks(domain)
	})
}

// DestroyWithoutHooks is used to remove the instance. The difference with
// Destroy is that scripts hooks are not executed for this function.
func DestroyWithoutHooks(domain string) error {
	var err error
	domain, err = validateDomain(domain)
	if err != nil {
		return err
	}
	sched := globals.GetScheduler()
	triggers, err := sched.GetAll(domain)
	if err == nil {
		for _, t := range triggers {
			if err = sched.Delete(domain, t.Infos().TID); err != nil {
				logger.WithDomain(domain).Error(
					"Failed to remove trigger: ", err)
			}
		}
	}
	i, err := getFromCouch(domain)
	if err != nil {
		return err
	}
	defer getCache().Revoke(domain)
	db := couchdb.SimpleDatabasePrefix(domain)
	if err = couchdb.DeleteAllDBs(db); err != nil {
		return err
	}
	if err = i.VFS().Delete(); err != nil {
		i.Logger().Errorf("Could not delete VFS: %s", err.Error())
	}
	return couchdb.DeleteDoc(couchdb.GlobalDB, i)
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
	return Update(i)
}

// RequestPassphraseReset generates a new registration token for the user to
// renew its password.
func (i *Instance) RequestPassphraseReset() error {
	// If a registration token is set, we do not generate another token than the
	// registration one, and bail.
	if i.RegisterToken != nil {
		i.Logger().Info("Passphrase reset ignored: not registered")
		return nil
	}
	// If a passphrase reset token is set and valid, we do not generate new one,
	// and bail.
	if i.PassphraseResetToken != nil &&
		time.Now().UTC().Before(i.PassphraseResetTime) {
		i.Logger().Infof("Passphrase reset ignored: already sent at %s",
			i.PassphraseResetTime.String())
		return ErrResetAlreadyRequested
	}
	i.PassphraseResetToken = crypto.GenerateRandomBytes(PasswordResetTokenLen)
	i.PassphraseResetTime = time.Now().UTC().Add(passwordResetValidityDuration)
	if err := Update(i); err != nil {
		return err
	}
	// Send a mail containing the reset url for the user to actually reset its
	// passphrase.
	resetURL := i.PageURL("/auth/passphrase_renew", url.Values{
		"token": {hex.EncodeToString(i.PassphraseResetToken)},
	})
	return i.SendMail(&Mail{
		SubjectKey:   "Mail Password reset",
		TemplateName: "passphrase_reset",
		TemplateValues: map[string]interface{}{
			"BaseURL":             i.PageURL("/", nil),
			"PassphraseResetLink": resetURL,
		},
	})
}

// Mail contains the informations to send a mail for the instance owner.
type Mail struct {
	SubjectKey     string
	TemplateName   string
	TemplateValues map[string]interface{}
}

// SendMail send a mail to the instance owner.
func (i *Instance) SendMail(m *Mail) error {
	msg, err := jobs.NewMessage(map[string]interface{}{
		"mode":            "noreply",
		"subject":         i.Translate(m.SubjectKey),
		"template_name":   m.TemplateName + "_" + i.Locale,
		"template_values": m.TemplateValues,
	})
	if err != nil {
		return err
	}
	_, err = globals.GetBroker().PushJob(&jobs.JobRequest{
		Domain:     i.Domain,
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}

// CheckPassphraseRenewToken checks whether the given token is good to use for
// resetting the passphrase.
func (i *Instance) CheckPassphraseRenewToken(tok []byte) error {
	if i.PassphraseResetToken == nil {
		return ErrMissingToken
	}
	if !time.Now().UTC().Before(i.PassphraseResetTime) {
		return ErrMissingToken
	}
	if subtle.ConstantTimeCompare(i.PassphraseResetToken, tok) != 1 {
		return ErrInvalidToken
	}
	return nil
}

// PassphraseRenew changes the passphrase to the specified one if the given
// token matches the `PassphraseResetToken` field.
func (i *Instance) PassphraseRenew(pass, tok []byte) error {
	err := i.CheckPassphraseRenewToken(tok)
	if err != nil {
		return err
	}
	hash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		return err
	}
	i.PassphraseResetToken = nil
	i.PassphraseResetTime = time.Time{}
	i.setPassphraseAndSecret(hash)
	return Update(i)
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
	return Update(i)
}

func (i *Instance) setPassphraseAndSecret(hash []byte) {
	i.PassphraseHash = hash
	i.SessionSecret = crypto.GenerateRandomBytes(SessionSecretLen)
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
	err = Update(i)
	if err != nil {
		i.Logger().Error("Failed to update hash in db", err)
	}

	return nil
}

// PickKey choose wich of the Instance keys to use depending on token audience
func (i *Instance) PickKey(audience string) ([]byte, error) {
	switch audience {
	case permissions.AppAudience,
		permissions.KonnectorAudience:
		return i.SessionSecret, nil
	case permissions.RefreshTokenAudience,
		permissions.AccessTokenAudience,
		permissions.ShareAudience:
		return i.OAuthSecret, nil
	case permissions.CLIAudience:
		return i.CLISecret, nil
	}
	return nil, permissions.ErrInvalidAudience
}

// MakeJWT is a shortcut to create a JWT
func (i *Instance) MakeJWT(audience, subject, scope string, issuedAt time.Time) (string, error) {
	secret, err := i.PickKey(audience)
	if err != nil {
		return "", err
	}
	return crypto.NewJWT(secret, permissions.Claims{
		StandardClaims: jwt.StandardClaims{
			Audience: audience,
			Issuer:   i.Domain,
			IssuedAt: issuedAt.Unix(),
			Subject:  subject,
		},
		Scope: scope,
	})
}

// BuildAppToken is used to build a token to identify the app for requests made
// to the stack
func (i *Instance) BuildAppToken(m apps.Manifest) string {
	scope := "" // apps tokens don't have a scope
	token, err := i.MakeJWT(permissions.AppAudience, m.Slug(), scope, time.Now())
	if err != nil {
		return ""
	}
	return token
}

// BuildKonnectorToken is used to build a token to identify the konnector for
// requests made to the stack
func (i *Instance) BuildKonnectorToken(m apps.Manifest) string {
	scope := "" // apps tokens don't have a scope
	token, err := i.MakeJWT(permissions.KonnectorAudience, m.Slug(), scope, time.Now())
	if err != nil {
		return ""
	}
	return token
}

func validateDomain(domain string) (string, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" || domain == ".." || domain == "." {
		return "", ErrIllegalDomain
	}
	if strings.ContainsAny(domain, illegalChars) {
		return "", ErrIllegalDomain
	}
	if strings.ContainsAny(domain[:1], illegalFirstChars) {
		return "", ErrIllegalDomain
	}
	return domain, nil
}

// ensure Instance implements couchdb.Doc
var (
	_ couchdb.Doc = &Instance{}
)
