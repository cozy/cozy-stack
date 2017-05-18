package instance

import (
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/settings"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsafero"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsswift"
	"github.com/leonelquinteros/gotext"
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

// DefaultLocale is the default locale when creating an instance
const DefaultLocale = "en"

const illegalChars = " /?#@\t\r\n\x00"

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
	DocID  string `json:"_id,omitempty"`  // couchdb _id
	DocRev string `json:"_rev,omitempty"` // couchdb _rev
	Domain string `json:"domain"`         // The main DNS domain, like example.cozycloud.cc
	Locale string `json:"locale"`         // The locale used on the server
	Dev    bool   `json:"dev"`            // Whether or not the instance is for development

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
func (i *Instance) Clone() couchdb.Doc { cloned := *i; return &cloned }

// Prefix returns the prefix to use in database naming for the
// current instance
func (i *Instance) Prefix() string {
	return i.Domain + "/"
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
	fsURL := config.FsURL()
	mutex := lock.ReadWrite(i.Domain)
	index := vfs.NewCouchdbIndexer(i)
	disk := vfs.DiskThresholder(i)
	var err error
	switch fsURL.Scheme {
	case config.SchemeFile, config.SchemeMem:
		i.vfs, err = vfsafero.New(index, disk, mutex, fsURL, i.Domain)
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
			path.Join(fsURL.Path, i.Domain, baseDirName))
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
			path.Join(fsURL.Path, i.Domain, vfs.WebappsDirName))
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
			path.Join(fsURL.Path, i.Domain, vfs.KonnectorsDirName))
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
			path.Join(fsURL.Path, i.Domain, vfs.ThumbsDirName))
		return vfsafero.NewThumbsFs(baseFS)
	case config.SchemeSwift:
		return vfsswift.NewThumbsFs(config.GetSwiftConnection(), i.Domain)
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
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
	administrative, err := vfs.NewDirDocWithPath("Administrative", consts.RootDirID, "/", nil)
	if err != nil {
		return err
	}
	if err = i.VFS().CreateDir(administrative); err != nil {
		return err
	}
	photos, err := vfs.NewDirDocWithPath("Photos", consts.RootDirID, "/", nil)
	if err != nil {
		return err
	}
	if err = i.VFS().CreateDir(photos); err != nil {
		return err
	}
	uploaded, err := vfs.NewDirDoc(i.VFS(), "Uploaded from Cozy Photos", photos.ID(), nil)
	if err != nil {
		return err
	}
	if err = i.VFS().CreateDir(uploaded); err != nil {
		return err
	}
	backuped, err := vfs.NewDirDoc(i.VFS(), "Backuped from my mobile", photos.ID(), nil)
	if err != nil {
		return err
	}
	if err = i.VFS().CreateDir(backuped); err != nil {
		return err
	}
	return nil
}

// Create builds an instance and initializes it
func Create(opts *Options) (*Instance, error) {
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

	if _, err := Get(i.Domain); err != ErrNotFound {
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
	if err := settings.CreateDefaultTheme(i); err != nil {
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
	sched := stack.GetScheduler()
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
			log.Error("[instance] Failed to install "+app, err)
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
		log.Infof("[instance] Indexes outdated: wanted %d; got %d",
			consts.IndexViewsVersion, i.IndexViewsVersion)
		if err = i.defineViewsAndIndex(); err != nil {
			log.Errorf("[instance] Could not re-define indexes and views %s: %s",
				i.Domain, err.Error())
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

	return instances[0], nil
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
		return po.Get(key, vars...)
	}
	if po, ok := translations[DefaultLocale]; ok {
		return po.Get(key, vars...)
	}
	return fmt.Sprintf(key, vars...)
}

// List returns the list of declared instances.
//
// TODO: pagination
// TODO: don't return the design docs
func List() ([]*Instance, error) {
	var docs []*Instance
	req := &couchdb.AllDocsRequest{Limit: 1000}
	err := couchdb.GetAllDocs(couchdb.GlobalDB, consts.Instances, req, &docs)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// Update is used to save changes made to an instance, it will invalidate
// caching
func Update(i *Instance) error {
	getCache().Revoke(i.Domain)
	if err := couchdb.UpdateDoc(couchdb.GlobalDB, i); err != nil {
		log.Errorf("[instance] Could not update %s: %s", i.Domain, err.Error())
		return err
	}
	return nil
}

// Destroy is used to remove the instance. All the data linked to this
// instance will be permanently deleted.
func Destroy(domain string) error {
	var err error
	domain, err = validateDomain(domain)
	if err != nil {
		return err
	}
	sched := stack.GetScheduler()
	triggers, err := sched.GetAll(domain)
	if err == nil {
		for _, t := range triggers {
			if err = sched.Delete(domain, t.Infos().TID); err != nil {
				log.Error("[instance] Failed to remove trigger: ", err)
			}
		}
	}
	i, err := Get(domain)
	if err != nil {
		return err
	}
	defer getCache().Revoke(domain)
	db := couchdb.SimpleDatabasePrefix(domain)
	if err = couchdb.DeleteAllDBs(db); err != nil {
		return err
	}
	if err = i.VFS().Delete(); err != nil {
		log.Errorf("[instance] Could not delete VFS %s: %s", i.Domain, err.Error())
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
		return nil
	}
	// If a passphrase reset token is set and valid, we do not generate new one,
	// and bail.
	if i.PassphraseResetToken != nil &&
		time.Now().UTC().Before(i.PassphraseResetTime) {
		return nil
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
	msg, err := jobs.NewMessage(jobs.JSONEncoding, map[string]interface{}{
		"mode":          "noreply",
		"subject":       i.Translate("Mail Password reset"),
		"template_name": "passphrase_reset_" + i.Locale,
		"template_values": map[string]string{
			"BaseURL":             i.PageURL("/", nil),
			"PassphraseResetLink": resetURL,
		},
	})
	if err != nil {
		return err
	}
	_, err = stack.GetBroker().PushJob(&jobs.JobRequest{
		Domain:     i.Domain,
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}

// PassphraseRenew changes the passphrase to the specified one if the given
// token matches the `PassphraseResetToken` field.
func (i *Instance) PassphraseRenew(pass, tok []byte) error {
	if i.PassphraseResetToken == nil {
		return ErrMissingToken
	}
	if !time.Now().UTC().Before(i.PassphraseResetTime) {
		return ErrMissingToken
	}
	if subtle.ConstantTimeCompare(i.PassphraseResetToken, tok) != 1 {
		return ErrInvalidToken
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
		log.Error("[instance] Failed to update hash in db", err)
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
	return domain, nil
}

// ensure Instance implements couchdb.Doc
var (
	_ couchdb.Doc = &Instance{}
)
