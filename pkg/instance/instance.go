package instance

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/hooks"
	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsafero"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsswift"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

/* #nosec */
const (
	RegisterTokenLen      = 16
	PasswordResetTokenLen = 16
	SessionSecretLen      = 64
	OauthSecretLen        = 128
)

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
	// ErrInvalidTwoFactor is returned when the two-factor authentication
	// verification is invalid.
	ErrInvalidTwoFactor = errors.New("Invalid two-factor parameters")
	// ErrContextNotFound is returned when the instance has no context
	ErrContextNotFound = errors.New("Context not found")
	// ErrResetAlreadyRequested is returned when a passphrase reset token is already set and valid
	ErrResetAlreadyRequested = errors.New("The passphrase reset has already been requested")
	// ErrUnknownAuthMode is returned when an unknown authentication mode is
	// used.
	ErrUnknownAuthMode = errors.New("Unknown authentication mode")
	// ErrBadTOSVersion is returned when a malformed TOS version is provided.
	ErrBadTOSVersion = errors.New("Bad format for TOS version")
)

// BlockingReason structs holds a reason why an instance had been blocked
type BlockingReason struct {
	Code    string
	Message string
}

var (
	// BlockedLoginFailed is used when a security issue has been detected on the instance
	BlockedLoginFailed = BlockingReason{Code: "LOGIN_FAILED", Message: "The instance was block because of too many login failed attempts"}
	// BlockedPaymentFailed is used when a payment is missing for the instance
	BlockedPaymentFailed = BlockingReason{Code: "PAYMENT_FAILED", Message: "The instance requires a payment"}
	// BlockedUnknown is used when an instance is blocked but the reason is unknown
	BlockedUnknown = BlockingReason{Code: "UNKNOWN", Message: "This instance is blocked for an unknown reason"}
)

// An Instance has the informations relatives to the logical cozy instance,
// like the domain, the locale or the access to the databases and files storage
// It is a couchdb.Doc to be persisted in couchdb.
type Instance struct {
	DocID          string   `json:"_id,omitempty"`  // couchdb _id
	DocRev         string   `json:"_rev,omitempty"` // couchdb _rev
	Domain         string   `json:"domain"`         // The main DNS domain, like example.cozycloud.cc
	DomainAliases  []string `json:"domain_aliases,omitempty"`
	Prefix         string   `json:"prefix,omitempty"`     // Possible database prefix
	Locale         string   `json:"locale"`               // The locale used on the server
	UUID           string   `json:"uuid,omitempty"`       // UUID associated with the instance
	ContextName    string   `json:"context,omitempty"`    // The context attached to the instance
	TOSSigned      string   `json:"tos,omitempty"`        // Terms of Service signed version
	TOSLatest      string   `json:"tos_latest,omitempty"` // Terms of Service latest version
	AuthMode       AuthMode `json:"auth_mode,omitempty"`
	Blocked        bool     `json:"blocked,omitempty"`         // Whether or not the instance is blocked
	BlockingReason string   `json:"blocking_reason,omitempty"` // Why the instance is blocked
	NoAutoUpdate   bool     `json:"no_auto_update,omitempty"`  // Whether or not the instance has auto updates for its applications

	OnboardingFinished bool  `json:"onboarding_finished,omitempty"` // Whether or not the onboarding is complete.
	BytesDiskQuota     int64 `json:"disk_quota,string,omitempty"`   // The total size in bytes allowed to the user
	IndexViewsVersion  int   `json:"indexes_version"`

	// Swift cluster number, indexed from 1. If not zero, it indicates we're using swift layout 2, see pkg/vfs/swift.
	SwiftCluster int `json:"swift_cluster,omitempty"`

	// PassphraseHash is a hash of the user's passphrase. For more informations,
	// see crypto.GenerateFromPassphrase.
	PassphraseHash       []byte     `json:"passphrase_hash,omitempty"`
	PassphraseResetToken []byte     `json:"passphrase_reset_token,omitempty"`
	PassphraseResetTime  *time.Time `json:"passphrase_reset_time,omitempty"`

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

	vfs              vfs.VFS
	contextualDomain string
}

// Options holds the parameters to create a new instance.
type Options struct {
	Domain         string
	DomainAliases  []string
	Locale         string
	UUID           string
	TOSSigned      string
	TOSLatest      string
	Timezone       string
	ContextName    string
	Email          string
	PublicName     string
	Settings       string
	SettingsObj    *couchdb.JSONDoc
	AuthMode       string
	Passphrase     string
	SwiftCluster   int
	DiskQuota      int64
	Apps           []string
	AutoUpdate     *bool
	Debug          *bool
	Blocked        *bool
	BlockingReason string

	OnboardingFinished *bool
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

	cloned.DomainAliases = make([]string, len(i.DomainAliases))
	copy(cloned.DomainAliases, i.DomainAliases)

	cloned.PassphraseHash = make([]byte, len(i.PassphraseHash))
	copy(cloned.PassphraseHash, i.PassphraseHash)

	cloned.PassphraseResetToken = make([]byte, len(i.PassphraseResetToken))
	copy(cloned.PassphraseResetToken, i.PassphraseResetToken)

	if i.PassphraseResetTime != nil {
		tmp := *i.PassphraseResetTime
		cloned.PassphraseResetTime = &tmp
	}

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

// DBPrefix returns the prefix to use in database naming for the
// current instance
func (i *Instance) DBPrefix() string {
	if i.Prefix != "" {
		return i.Prefix
	}
	return i.Domain
}

// DomainName returns the main domain name of the instance.
func (i *Instance) DomainName() string {
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
	mutex := lock.ReadWrite(i, "vfs")
	index := vfs.NewCouchdbIndexer(i)
	disk := vfs.DiskThresholder(i)
	var err error
	switch fsURL.Scheme {
	case config.SchemeFile, config.SchemeMem:
		i.vfs, err = vfsafero.New(i, index, disk, mutex, fsURL, i.DirName())
	case config.SchemeSwift, config.SchemeSwiftSecure:
		if i.SwiftCluster > 0 {
			i.vfs, err = vfsswift.NewV2(i, index, disk, mutex)
		} else {
			i.vfs, err = vfsswift.New(i, index, disk, mutex)
		}
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
	case config.SchemeFile:
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
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS("apps")
		return apps.NewAferoCopier(baseFS)
	case config.SchemeSwift, config.SchemeSwiftSecure:
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
	case config.SchemeFile:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.WebappsDirName))
		return apps.NewAferoFileServer(baseFS, nil)
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS("apps")
		return apps.NewAferoFileServer(baseFS, nil)
	case config.SchemeSwift, config.SchemeSwiftSecure:
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
	case config.SchemeFile:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.KonnectorsDirName))
		return apps.NewAferoFileServer(baseFS, nil)
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS("apps")
		return apps.NewAferoFileServer(baseFS, nil)
	case config.SchemeSwift, config.SchemeSwiftSecure:
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
	case config.SchemeFile:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.ThumbsDirName))
		return vfsafero.NewThumbsFs(baseFS)
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS(i.DomainName() + "-thumbs")
		return vfsafero.NewThumbsFs(baseFS)
	case config.SchemeSwift, config.SchemeSwiftSecure:
		if i.SwiftCluster > 0 {
			return vfsswift.NewThumbsFsV2(config.GetSwiftConnection(), i)
		}
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

// SettingsEMail returns the email address defined in the settings of this
// instance.
func (i *Instance) SettingsEMail() (string, error) {
	settings, err := i.SettingsDocument()
	if err != nil {
		return "", err
	}
	email, _ := settings.M["email"].(string)
	return email, nil
}

// SettingsPublicName returns the public name defined in the settings of this
// instance.
func (i *Instance) SettingsPublicName() (string, error) {
	settings, err := i.SettingsDocument()
	if err != nil {
		return "", err
	}
	email, _ := settings.M["public_name"].(string)
	return email, nil
}

func (i *Instance) getFromContexts(contexts map[string]interface{}) (interface{}, bool) {
	if contexts == nil {
		return nil, false
	}

	if i.ContextName != "" {
		context, ok := contexts[i.ContextName]
		if ok {
			return context, true
		}
	}

	context, ok := contexts[config.DefaultInstanceContext]
	if ok && context != nil {
		return context, ok
	}

	return nil, false
}

// SettingsContext returns the map from the config that matches the context of
// this instance
func (i *Instance) SettingsContext() (map[string]interface{}, error) {
	contexts := config.GetConfig().Contexts
	context, ok := i.getFromContexts(contexts)
	if !ok {
		return nil, ErrContextNotFound
	}
	settings := context.(map[string]interface{})
	return settings, nil
}

// Registries returns the list of registries associated with the instance.
func (i *Instance) Registries() []*url.URL {
	contexts := config.GetConfig().Registries
	var context []*url.URL
	var ok bool
	if i.ContextName != "" {
		context, ok = contexts[i.ContextName]
	}
	if !ok {
		context, ok = contexts[config.DefaultInstanceContext]
		if !ok {
			context = make([]*url.URL, 0)
		}
	}
	return context
}

// DiskQuota returns the number of bytes allowed on the disk to the user.
func (i *Instance) DiskQuota() int64 {
	return i.BytesDiskQuota
}

// WithContextualDomain the current instance context with the given hostname.
func (i *Instance) WithContextualDomain(domain string) *Instance {
	if i.HasDomain(domain) {
		i.contextualDomain = domain
	}
	return i
}

// Scheme returns the scheme used for URLs. It is https by default and http
// for development instances.
func (i *Instance) Scheme() string {
	if config.IsDevRelease() {
		return "http"
	}
	return "https"
}

// ContextualDomain returns the domain with regard to the current domain
// request.
func (i *Instance) ContextualDomain() string {
	if i.contextualDomain != "" {
		return i.contextualDomain
	}
	return i.Domain
}

// HasDomain returns whether or not the given domain name is owned by this
// instance, as part of its main domain name or its aliases.
func (i *Instance) HasDomain(domain string) bool {
	if domain == i.Domain {
		return true
	}
	for _, alias := range i.DomainAliases {
		if domain == alias {
			return true
		}
	}
	return false
}

// SubDomain returns the full url for a subdomain of this instance
// useful with apps slugs
func (i *Instance) SubDomain(s string) *url.URL {
	domain := i.ContextualDomain()
	if config.GetConfig().Subdomains == config.NestedSubdomains {
		domain = s + "." + domain
	} else {
		parts := strings.SplitN(domain, ".", 2)
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
		Host:     i.ContextualDomain(),
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
		Host:     i.ContextualDomain(),
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
	context, err := i.SettingsContext()
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
	return i.redirection("default_redirection", consts.HomeSlug)
}

// OnboardedRedirection returns the URL where to redirect the user after
// onboarding
func (i *Instance) OnboardedRedirection() *url.URL {
	return i.redirection("onboarded_redirection", consts.HomeSlug)
}

func (i *Instance) installApp(slug string) error {
	source := "registry://" + slug + "/stable"
	inst, err := apps.NewInstaller(i, i.AppsCopier(apps.Webapp), &apps.InstallerOptions{
		Operation:  apps.Install,
		Type:       apps.Webapp,
		SourceURL:  source,
		Slug:       slug,
		Registries: i.Registries(),
	})
	if err != nil {
		return err
	}
	_, err = inst.RunSync()
	return err
}

func (i *Instance) update() error {
	if err := couchdb.UpdateDoc(couchdb.GlobalDB, i); err != nil {
		i.Logger().Errorf("Could not update: %s", err.Error())
		return err
	}
	return nil
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
	if _, err = getFromCouch(domain); err != ErrNotFound {
		if err == nil {
			err = ErrExists
		}
		return nil, err
	}

	locale := opts.Locale
	if locale == "" {
		locale = DefaultLocale
	}

	settings, _ := buildSettings(opts)
	prefix := sha256.Sum256([]byte(domain))
	i := new(Instance)
	i.Domain = domain
	i.DomainAliases, err = checkAliases(i, opts.DomainAliases)
	if err != nil {
		return nil, err
	}
	i.Prefix = "cozy" + hex.EncodeToString(prefix[:16])
	i.Locale = locale
	i.UUID = opts.UUID
	i.TOSSigned = opts.TOSSigned
	i.TOSLatest = opts.TOSLatest
	i.ContextName = opts.ContextName
	i.BytesDiskQuota = opts.DiskQuota
	i.IndexViewsVersion = consts.IndexViewsVersion
	i.RegisterToken = crypto.GenerateRandomBytes(RegisterTokenLen)
	i.SessionSecret = crypto.GenerateRandomBytes(SessionSecretLen)
	i.OAuthSecret = crypto.GenerateRandomBytes(OauthSecretLen)
	i.CLISecret = crypto.GenerateRandomBytes(OauthSecretLen)

	// If not cluster number is given, we rely on cluster one.
	if opts.SwiftCluster == 0 {
		i.SwiftCluster = 1
	} else {
		i.SwiftCluster = opts.SwiftCluster
	}

	if opts.AuthMode != "" {
		var authMode AuthMode
		if authMode, err = StringToAuthMode(opts.AuthMode); err == nil {
			i.AuthMode = authMode
		}
	}

	if opts.Passphrase != "" {
		if err = i.registerPassphrase([]byte(opts.Passphrase), i.RegisterToken); err != nil {
			return nil, err
		}
		// set the onboarding finished when specifying a passphrase. we totally
		// skip the onboarding in that case.
		i.OnboardingFinished = true
	}

	if onboardingFinished := opts.OnboardingFinished; onboardingFinished != nil {
		i.OnboardingFinished = *onboardingFinished
	}

	if autoUpdate := opts.AutoUpdate; autoUpdate != nil {
		i.NoAutoUpdate = !(*opts.AutoUpdate)
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
	if err := couchdb.CreateNamedDoc(i, settings); err != nil {
		return nil, err
	}
	if err := i.defineViewsAndIndex(); err != nil {
		return nil, err
	}
	if err := i.createDefaultFilesTree(); err != nil {
		return nil, err
	}
	sched := jobs.System()
	for _, trigger := range Triggers(i) {
		t, err := jobs.NewTrigger(i, trigger, nil)
		if err != nil {
			return nil, err
		}
		if err = sched.AddTrigger(t); err != nil {
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
	i, err := getFromCouch(domain)
	if err != nil {
		return nil, err
	}

	// This retry-loop handles the probability to hit an Update conflict from
	// this version update, since the instance document may be updated different
	// processes at the same time.
	for {
		if i == nil {
			i, err = getFromCouch(domain)
			if err != nil {
				return nil, err
			}
		}

		if i.IndexViewsVersion == consts.IndexViewsVersion {
			break
		}

		i.Logger().Debugf("Indexes outdated: wanted %d; got %d", consts.IndexViewsVersion, i.IndexViewsVersion)
		if err = i.defineViewsAndIndex(); err != nil {
			i.Logger().Errorf("Could not re-define indexes and views: %s", err.Error())
			return nil, err
		}

		// Copy over the instance object some data that we used to store on the
		// settings document.
		if i.TOSSigned == "" || i.UUID == "" || i.ContextName == "" {
			var settings *couchdb.JSONDoc
			settings, err = i.SettingsDocument()
			if err != nil {
				return nil, err
			}
			i.UUID, _ = settings.M["uuid"].(string)
			i.TOSSigned, _ = settings.M["tos"].(string)
			i.ContextName, _ = settings.M["context"].(string)
			// TOS version number were YYYYMMDD dates, before we used a semver-like
			// version scheme. We consider them to be the versions 1.0.0.
			if len(i.TOSSigned) == 8 {
				i.TOSSigned = "1.0.0-" + i.TOSSigned
			}
		}

		err = i.update()
		if err == nil {
			break
		}

		if !couchdb.IsConflictError(err) {
			return nil, err
		}

		i = nil
	}

	if err = i.makeVFS(); err != nil {
		return nil, err
	}
	return i, nil
}

func buildSettings(opts *Options) (*couchdb.JSONDoc, bool) {
	var settings *couchdb.JSONDoc
	if opts.SettingsObj != nil {
		settings = opts.SettingsObj
	} else {
		settings = &couchdb.JSONDoc{M: make(map[string]interface{})}
	}

	settings.Type = consts.Settings
	settings.SetID(consts.InstanceSettingsID)

	for _, s := range strings.Split(opts.Settings, ",") {
		if parts := strings.SplitN(s, ":", 2); len(parts) == 2 {
			settings.M[parts[0]] = parts[1]
		}
	}

	if contextName, ok := settings.M["context"].(string); ok {
		opts.ContextName = contextName
		delete(settings.M, "context")
	}
	if locale, ok := settings.M["locale"].(string); ok {
		opts.Locale = locale
		delete(settings.M, "locale")
	}
	if onboardingFinished, ok := settings.M["onboarding_finished"].(bool); ok {
		opts.OnboardingFinished = &onboardingFinished
		delete(settings.M, "onboarding_finished")
	}
	if uuid, ok := settings.M["uuid"].(string); ok {
		opts.UUID = uuid
		delete(settings.M, "uuid")
	}
	if tos, ok := settings.M["tos"].(string); ok {
		opts.TOSSigned = tos
		delete(settings.M, "tos")
	}
	if tos, ok := settings.M["tos_latest"].(string); ok {
		opts.TOSLatest = tos
		delete(settings.M, "tos_latest")
	}
	if autoUpdate, ok := settings.M["auto_update"].(string); ok {
		if b, err := strconv.ParseBool(autoUpdate); err == nil {
			opts.AutoUpdate = &b
		}
		delete(settings.M, "auto_update")
	}
	if authMode, ok := settings.M["auth_mode"].(string); ok {
		opts.AuthMode = authMode
		delete(settings.M, "auth_mode")
	}

	if tz := opts.Timezone; tz != "" {
		settings.M["tz"] = tz
	}
	if email := opts.Email; email != "" {
		settings.M["email"] = email
	}
	if name := opts.PublicName; name != "" {
		settings.M["public_name"] = name
	}

	if len(opts.TOSSigned) == 8 {
		opts.TOSSigned = "1.0.0-" + opts.TOSSigned
	}

	needUpdate := settings.Rev() != "" && len(settings.M) > 1
	return settings, needUpdate
}

// Patch updates the given instance with the specified options if necessary. It
// can also update the settings document if provided in the options.
func Patch(i *Instance, opts *Options) error {
	opts.Domain = i.Domain
	settings, settingsUpdate := buildSettings(opts)

	clouderyChanges := make(map[string]interface{})

	for {
		var err error
		if i == nil {
			i, err = Get(opts.Domain)
			if err != nil {
				return err
			}
		}

		needUpdate := false
		if opts.Locale != "" && opts.Locale != i.Locale {
			i.Locale = opts.Locale
			clouderyChanges["locale"] = i.Locale
			needUpdate = true
		}

		if opts.Blocked != nil && *opts.Blocked != i.Blocked {
			i.Blocked = *opts.Blocked
			needUpdate = true
		}

		if opts.BlockingReason != "" && opts.BlockingReason != i.BlockingReason {
			i.BlockingReason = opts.BlockingReason
			needUpdate = true
		}

		if aliases := opts.DomainAliases; aliases != nil {
			i.DomainAliases, err = checkAliases(i, aliases)
			if err != nil {
				return err
			}
			needUpdate = true
		}

		if opts.UUID != "" && opts.UUID != i.UUID {
			i.UUID = opts.UUID
			needUpdate = true
		}

		if opts.ContextName != "" && opts.ContextName != i.ContextName {
			i.ContextName = opts.ContextName
			needUpdate = true
		}

		if opts.AuthMode != "" {
			var authMode AuthMode
			authMode, err = StringToAuthMode(opts.AuthMode)
			if err != nil {
				return err
			}
			if i.AuthMode != authMode {
				i.AuthMode = authMode
				needUpdate = true
			}
		}

		if opts.SwiftCluster > 0 && opts.SwiftCluster != i.SwiftCluster {
			i.SwiftCluster = opts.SwiftCluster
			needUpdate = true
		}

		if opts.DiskQuota > 0 && opts.DiskQuota != i.BytesDiskQuota {
			i.BytesDiskQuota = opts.DiskQuota
			needUpdate = true
		} else if opts.DiskQuota == -1 {
			i.BytesDiskQuota = 0
			needUpdate = true
		}

		if opts.AutoUpdate != nil && !(*opts.AutoUpdate) != i.NoAutoUpdate {
			i.NoAutoUpdate = !(*opts.AutoUpdate)
			needUpdate = true
		}

		if opts.OnboardingFinished != nil && *opts.OnboardingFinished != i.OnboardingFinished {
			i.OnboardingFinished = *opts.OnboardingFinished
			needUpdate = true
		}

		if opts.TOSLatest != "" {
			if _, date, ok := parseTOSVersion(opts.TOSLatest); !ok || date.IsZero() {
				return ErrBadTOSVersion
			}
			if i.TOSLatest != opts.TOSLatest {
				if i.CheckTOSNotSigned(opts.TOSLatest) {
					i.TOSLatest = opts.TOSLatest
					needUpdate = true
				}
			}
		}

		if opts.TOSSigned != "" {
			if _, _, ok := parseTOSVersion(opts.TOSSigned); !ok {
				return ErrBadTOSVersion
			}
			if i.TOSSigned != opts.TOSSigned {
				i.TOSSigned = opts.TOSSigned
				if !i.CheckTOSNotSigned() {
					i.TOSLatest = ""
				}
				needUpdate = true
			}
		}

		if !needUpdate {
			break
		}

		err = i.update()
		if couchdb.IsConflictError(err) {
			i = nil
			continue
		}
		if err != nil {
			return err
		}
		break
	}

	if settingsUpdate {
		oldSettings, err := i.SettingsDocument()
		update := false
		if err == nil {
			old := oldSettings.M["email"]
			new := settings.M["email"]
			if old != new {
				clouderyChanges["email"] = new
				update = true
			}
			old = oldSettings.M["public_name"]
			new = settings.M["public_name"]
			if old != new {
				clouderyChanges["public_name"] = new
				update = true
			}
			old = oldSettings.M["tz"]
			new = settings.M["tz"]
			if old != new {
				update = true
			}
		}
		if update {
			if err := couchdb.UpdateDoc(i, settings); err != nil {
				return err
			}
		}
	}

	if debug := opts.Debug; debug != nil {
		var err error
		if *debug {
			err = logger.AddDebugDomain(i.Domain)
		} else {
			err = logger.RemoveDebugDomain(i.Domain)
		}
		if err != nil {
			return err
		}
	}

	i.managerUpdateSettings(clouderyChanges)

	return nil
}

func checkAliases(i *Instance, aliases []string) ([]string, error) {
	if aliases == nil {
		return nil, nil
	}
	aliases = utils.UniqueStrings(aliases)
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if alias == i.Domain {
			return nil, ErrExists
		}
		i2, err := getFromCouch(alias)
		if err != ErrNotFound {
			if err != nil {
				return nil, err
			}
			if i2.ID() != i.ID() {
				return nil, ErrExists
			}
		}
	}
	return aliases, nil
}

func getFromCouch(domain string) (*Instance, error) {
	var res couchdb.ViewResponse
	err := couchdb.ExecView(couchdb.GlobalDB, consts.DomainAndAliasesView, &couchdb.ViewRequest{
		Key:         domain,
		IncludeDocs: true,
		Limit:       1,
	}, &res)
	if couchdb.IsNoDatabaseError(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(res.Rows) == 0 {
		return nil, ErrNotFound
	}
	inst := &Instance{}
	err = json.Unmarshal(res.Rows[0].Doc, &inst)
	if err != nil {
		return nil, err
	}
	if err = inst.makeVFS(); err != nil {
		return nil, err
	}
	return inst, nil
}

// Translate is used to translate a string to the locale used on this instance
func (i *Instance) Translate(key string, vars ...interface{}) string {
	return i18n.Translate(key, i.Locale, vars...)
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
	return couchdb.ForeachDocs(couchdb.GlobalDB, consts.Instances, func(_ string, data json.RawMessage) error {
		var doc *Instance
		if err := json.Unmarshal(data, &doc); err != nil {
			return err
		}
		return fn(doc)
	})
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
	i, err := getFromCouch(domain)
	if err != nil {
		return err
	}

	// Deleting accounts manually to invoke the "account deletion hook" which may
	// launch a worker in order to clean the account.
	deleteAccounts(i)

	// Reload the instance, it can have been updated in CouchDB if the instance
	// had at least one account and was not up-to-date for its indexes/views.
	i, err = getFromCouch(domain)
	if err != nil {
		return err
	}

	sched := jobs.System()
	triggers, err := sched.GetAllTriggers(i)
	if err == nil {
		for _, t := range triggers {
			if err = sched.DeleteTrigger(i, t.Infos().TID); err != nil {
				logger.WithDomain(domain).Error(
					"Failed to remove trigger: ", err)
			}
		}
	}

	if err = couchdb.DeleteAllDBs(i); err != nil {
		i.Logger().Errorf("Could not delete all CouchDB databases: %s", err.Error())
		return err
	}

	if err = i.VFS().Delete(); err != nil {
		i.Logger().Errorf("Could not delete VFS: %s", err.Error())
		return err
	}

	return couchdb.DeleteDoc(couchdb.GlobalDB, i)
}

func deleteAccounts(i *Instance) {
	var accounts []*couchdb.JSONDoc
	if err := couchdb.GetAllDocs(i, consts.Accounts, nil, &accounts); err != nil || len(accounts) == 0 {
		return
	}

	ds := realtime.GetHub().Subscriber(i)
	defer ds.Close()

	accountsCount := 0
	for _, account := range accounts {
		account.Type = consts.Accounts
		if err := couchdb.DeleteDoc(i, account); err == nil {
			accountsCount++
		}
	}
	if accountsCount == 0 {
		return
	}

	if err := ds.Subscribe(consts.Jobs); err != nil {
		return
	}

	timeout := time.After(1 * time.Minute)
	for {
		select {
		case e := <-ds.Channel:
			j, ok := e.Doc.(*couchdb.JSONDoc)
			if ok {
				deleted, _ := j.M["account_deleted"].(bool)
				stateStr, _ := j.M["state"].(string)
				state := jobs.State(stateStr)
				if deleted && (state == jobs.Done || state == jobs.Errored) {
					accountsCount--
					if accountsCount == 0 {
						return
					}
				}
			}
		case <-timeout:
			return
		}
	}
}

func (i *Instance) registerPassphrase(pass, tok []byte) error {
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
	return nil
}

// RegisterPassphrase replace the instance registerToken by a passphrase
func (i *Instance) RegisterPassphrase(pass, tok []byte) error {
	if err := i.registerPassphrase(pass, tok); err != nil {
		return err
	}
	return i.update()
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
	if i.PassphraseResetToken != nil && i.PassphraseResetTime != nil &&
		time.Now().UTC().Before(*i.PassphraseResetTime) {
		i.Logger().Infof("Passphrase reset ignored: already sent at %s",
			i.PassphraseResetTime.String())
		return ErrResetAlreadyRequested
	}
	resetTime := time.Now().UTC().Add(config.PasswordResetInterval())
	i.PassphraseResetToken = crypto.GenerateRandomBytes(PasswordResetTokenLen)
	i.PassphraseResetTime = &resetTime
	if err := i.update(); err != nil {
		return err
	}
	// Send a mail containing the reset url for the user to actually reset its
	// passphrase.
	resetURL := i.PageURL("/auth/passphrase_renew", url.Values{
		"token": {hex.EncodeToString(i.PassphraseResetToken)},
	})
	return i.SendMail(&Mail{
		TemplateName: "passphrase_reset",
		TemplateValues: map[string]interface{}{
			"BaseURL":             i.PageURL("/", nil),
			"PassphraseResetLink": resetURL,
		},
	})
}

// Mail contains the informations to send a mail for the instance owner.
type Mail struct {
	TemplateName   string
	TemplateValues map[string]interface{}
}

// SendMail send a mail to the instance owner.
func (i *Instance) SendMail(m *Mail) error {
	msg, err := jobs.NewMessage(map[string]interface{}{
		"mode":            "noreply",
		"template_name":   m.TemplateName,
		"template_values": m.TemplateValues,
	})
	if err != nil {
		return err
	}
	_, err = jobs.System().PushJob(i, &jobs.JobRequest{
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
	if i.PassphraseResetTime != nil && !time.Now().UTC().Before(*i.PassphraseResetTime) {
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
	i.PassphraseResetTime = nil
	i.setPassphraseAndSecret(hash)
	return i.update()
}

// UpdatePassphrase replace the passphrase
func (i *Instance) UpdatePassphrase(pass, current []byte, twoFactorPasscode string, twoFactorToken []byte) error {
	if len(pass) == 0 {
		return ErrMissingPassphrase
	}
	// With two factor authentication, we do not check the validity of the
	// current passphrase, but the validity of the pair passcode/token which has
	// been exchanged against the current passphrase.
	if i.HasAuthMode(TwoFactorMail) {
		if !i.ValidateTwoFactorPasscode(twoFactorToken, twoFactorPasscode) {
			return ErrInvalidTwoFactor
		}
	} else {
		// the needUpdate flag is not checked against since the passphrase will be
		// regenerated with updated parameters just after, if the passphrase match.
		_, err := crypto.CompareHashAndPassphrase(i.PassphraseHash, current)
		if err != nil {
			return ErrInvalidPassphrase
		}
	}
	hash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		return err
	}
	i.setPassphraseAndSecret(hash)
	return i.update()
}

// ForceUpdatePassphrase replace the passphrase without checking the current one
func (i *Instance) ForceUpdatePassphrase(newPassword []byte) error {
	if len(newPassword) == 0 {
		return ErrMissingPassphrase
	}

	hash, err := crypto.GenerateFromPassphrase(newPassword)
	if err != nil {
		return err
	}
	i.setPassphraseAndSecret(hash)
	return i.update()
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
	err = i.update()
	if err != nil {
		i.Logger().Error("Failed to update hash in db", err)
	}

	return nil
}

// PickKey choose which of the Instance keys to use depending on token audience
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
func (i *Instance) MakeJWT(audience, subject, scope, sessionID string, issuedAt time.Time) (string, error) {
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
		Scope:     scope,
		SessionID: sessionID,
	})
}

// BuildAppToken is used to build a token to identify the app for requests made
// to the stack
func (i *Instance) BuildAppToken(m apps.Manifest, sessionID string) string {
	scope := "" // apps tokens don't have a scope
	subject := m.Slug()
	now := time.Now()
	token, err := i.MakeJWT(permissions.AppAudience, subject, scope, sessionID, now)
	if err != nil {
		return ""
	}
	return token
}

// BuildKonnectorToken is used to build a token to identify the konnector for
// requests made to the stack
func (i *Instance) BuildKonnectorToken(m apps.Manifest) string {
	scope := "" // apps tokens don't have a scope
	subject := m.Slug()
	token, err := i.MakeJWT(permissions.KonnectorAudience, subject, scope, "", time.Now())
	if err != nil {
		return ""
	}
	return token
}

// CreateShareCode returns a new sharecode to put the codes field of a
// permissions document
func (i *Instance) CreateShareCode(subject string) (string, error) {
	scope := ""
	sessionID := ""
	return i.MakeJWT(permissions.ShareAudience, subject, scope, sessionID, time.Now())
}

// Block function blocks an instance with an optional reason parameter
func (i *Instance) Block(reason ...string) error {
	var r string

	if len(reason) == 1 {
		r = reason[0]
	} else {
		r = BlockedUnknown.Code
	}
	blocked := true

	err := Patch(i, &Options{
		Blocked:        &blocked,
		BlockingReason: r,
	})
	if err != nil {
		return err
	}
	return nil
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
	domain = strings.ToLower(domain)
	if config.GetConfig().Subdomains == config.FlatSubdomains {
		parts := strings.SplitN(domain, ".", 2)
		if strings.Contains(parts[0], "-") {
			return "", ErrIllegalDomain
		}
	}
	return domain, nil
}

// ensure Instance implements couchdb.Doc
var (
	_ couchdb.Doc = &Instance{}
)
