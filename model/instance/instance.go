package instance

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/model/vfs/vfsafero"
	"github.com/cozy/cozy-stack/model/vfs/vfsswift"
	"github.com/cozy/cozy-stack/pkg/appfs"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"

	"github.com/cozy/afero"
	"github.com/sirupsen/logrus"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

// DefaultTemplateTitle represents the default template title. It could be
// overrided by configuring it in the instance context parameters
const DefaultTemplateTitle = "Cozy"

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

	// Swift cluster number, indexed from 1. If not zero, it indicates we're
	// using swift layout 2, see model/vfs/swift.
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

// MakeVFS is used to initialize the VFS linked to this instance
func (i *Instance) MakeVFS() error {
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
func (i *Instance) AppsCopier(appsType consts.AppType) appfs.Copier {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile:
		var baseDirName string
		switch appsType {
		case consts.WebappType:
			baseDirName = vfs.WebappsDirName
		case consts.KonnectorType:
			baseDirName = vfs.KonnectorsDirName
		}
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), baseDirName))
		return appfs.NewAferoCopier(baseFS)
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS("apps")
		return appfs.NewAferoCopier(baseFS)
	case config.SchemeSwift, config.SchemeSwiftSecure:
		return appfs.NewSwiftCopier(config.GetSwiftConnection(), appsType)
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
}

// AppsFileServer returns the web-application file server associated to this
// instance.
func (i *Instance) AppsFileServer() appfs.FileServer {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.WebappsDirName))
		return appfs.NewAferoFileServer(baseFS, nil)
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS("apps")
		return appfs.NewAferoFileServer(baseFS, nil)
	case config.SchemeSwift, config.SchemeSwiftSecure:
		return appfs.NewSwiftFileServer(config.GetSwiftConnection(), consts.WebappType)
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
}

// KonnectorsFileServer returns the web-application file server associated to this
// instance.
func (i *Instance) KonnectorsFileServer() appfs.FileServer {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.KonnectorsDirName))
		return appfs.NewAferoFileServer(baseFS, nil)
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS("apps")
		return appfs.NewAferoFileServer(baseFS, nil)
	case config.SchemeSwift, config.SchemeSwiftSecure:
		return appfs.NewSwiftFileServer(config.GetSwiftConnection(), consts.KonnectorType)
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

// GetFromContexts returns the parameters specific to the instance context
func (i *Instance) GetFromContexts(contexts map[string]interface{}) (interface{}, bool) {
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
	context, ok := i.GetFromContexts(contexts)
	if !ok {
		return nil, ErrContextNotFound
	}
	settings := context.(map[string]interface{})
	return settings, nil
}

// TemplateTitle returns the specific-context instance template title (if there
// is one). Otherwise, returns the default one
func (i *Instance) TemplateTitle() string {
	ctxSettings, err := i.SettingsContext()
	if err != nil {
		return DefaultTemplateTitle
	}
	if title, ok := ctxSettings["templates_title"].(string); ok {
		return title
	}
	return DefaultTemplateTitle
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

// IsPasswordAuthenticationEnabled returns false only if the instance is in a
// context where the config says that the stack shouldn't allow to authenticate
// with the password.
func (i *Instance) IsPasswordAuthenticationEnabled() bool {
	if i.ContextName == "" {
		return true
	}
	auth, ok := config.GetConfig().Authentication[i.ContextName].(map[string]interface{})
	if !ok {
		return true
	}
	disabled, ok := auth["disable_password_authentication"].(bool)
	if !ok {
		return true
	}
	return !disabled
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
	if build.IsDevRelease() {
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

// GetFromCouch finds an instance in CouchDB from its domain
func GetFromCouch(domain string) (*Instance, error) {
	var res couchdb.ViewResponse
	err := couchdb.ExecView(couchdb.GlobalDB, couchdb.DomainAndAliasesView, &couchdb.ViewRequest{
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
	if err = inst.MakeVFS(); err != nil {
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

// PickKey choose which of the Instance keys to use depending on token audience
func (i *Instance) PickKey(audience string) ([]byte, error) {
	switch audience {
	case consts.AppAudience, consts.KonnectorAudience:
		return i.SessionSecret, nil
	case consts.RefreshTokenAudience, consts.AccessTokenAudience, consts.ShareAudience:
		return i.OAuthSecret, nil
	case consts.CLIAudience:
		return i.CLISecret, nil
	}
	return nil, permission.ErrInvalidAudience
}

// MakeJWT is a shortcut to create a JWT
func (i *Instance) MakeJWT(audience, subject, scope, sessionID string, issuedAt time.Time) (string, error) {
	secret, err := i.PickKey(audience)
	if err != nil {
		return "", err
	}
	return crypto.NewJWT(secret, permission.Claims{
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
func (i *Instance) BuildAppToken(slug, sessionID string) string {
	scope := "" // apps tokens don't have a scope
	now := time.Now()
	token, err := i.MakeJWT(consts.AppAudience, slug, scope, sessionID, now)
	if err != nil {
		return ""
	}
	return token
}

// BuildKonnectorToken is used to build a token to identify the konnector for
// requests made to the stack
func (i *Instance) BuildKonnectorToken(slug string) string {
	scope := "" // apps tokens don't have a scope
	token, err := i.MakeJWT(consts.KonnectorAudience, slug, scope, "", time.Now())
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
	return i.MakeJWT(consts.ShareAudience, subject, scope, sessionID, time.Now())
}

// ensure Instance implements couchdb.Doc
var (
	_ couchdb.Doc = &Instance{}
)
