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
	"github.com/cozy/cozy-stack/pkg/jobs/workers"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/settings"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsafero"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsswift"
	"github.com/leonelquinteros/gotext"
	"github.com/spf13/afero"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

/* #nosec */
const (
	registerTokenLen      = 16
	passwordResetTokenLen = 16
	sessionSecretLen      = 64
	oauthSecretLen        = 128
)

// passwordResetValidityDuration is the validity duration of the passphrase
// reset token.
var passwordResetValidityDuration = 15 * time.Minute

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
	DocID  string `json:"_id,omitempty"`  // couchdb _id
	DocRev string `json:"_rev,omitempty"` // couchdb _rev
	Domain string `json:"domain"`         // The main DNS domain, like example.cozycloud.cc
	Locale string `json:"locale"`         // The locale used on the server
	Dev    bool   `json:"dev"`            // Whether or not the instance is for development

	BytesDiskQuota int64 `json:"disk_quota,string,omitempty"` // The total size in bytes allowed to the user

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
	Domain     string
	Locale     string
	Timezone   string
	Email      string
	PublicName string
	DiskQuota  int64
	Apps       []string
	Dev        bool
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

// settings is a struct used for the settings of an instance
type instanceSettings struct {
	Timezone   string `json:"tz,omitempty"`
	Email      string `json:"email,omitempty"`
	PublicName string `json:"public_name,omitempty"`
}

func (s *instanceSettings) ID() string      { return consts.InstanceSettingsID }
func (s *instanceSettings) Rev() string     { return "" }
func (s *instanceSettings) DocType() string { return consts.Settings }
func (s *instanceSettings) SetID(_ string)  {}
func (s *instanceSettings) SetRev(_ string) {}

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
	mutex := vfs.NewMemLock(i.Domain)
	index := vfs.NewCouchdbIndexer(i)
	disk := vfs.DiskThresholder(i)
	var err error
	switch fsURL.Scheme {
	case "file", "mem":
		i.vfs, err = vfsafero.New(index, disk, mutex, fsURL, i.Domain)
	case "swift":
		i.vfs, err = vfsswift.New(index, disk, mutex, i.Domain)
	default:
		err = fmt.Errorf("instance: unknown storage provider %s", fsURL.Scheme)
	}
	return err
}

// AppsFS returns the hidden filesystem associated with the specified
// application type
func (i *Instance) AppsFS(appsType apps.AppType) afero.Fs {
	switch appsType {
	case apps.Webapp:
		return i.hiddenFS(vfs.WebappsDirName)
	case apps.Konnector:
		return i.hiddenFS(vfs.KonnectorsDirName)
	}
	panic(fmt.Errorf("Unknown application type %s", string(appsType)))
}

// DiskQuota returns the number of bytes allowed on the disk to the user.
func (i *Instance) DiskQuota() int64 {
	return i.BytesDiskQuota
}

func (i *Instance) hiddenFS(dirname string) afero.Fs {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case "file", "mem":
		return afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.Domain, dirname))
	case "swift":
		panic("Not implemented")
	}
	return nil
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
	inst, err := apps.NewInstaller(i, i.AppsFS(apps.Webapp), &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		SourceURL: source,
		Slug:      slug,
	})
	if err != nil {
		return err
	}
	go inst.Install()
	for {
		_, done, err := inst.Poll()
		if err != nil {
			return err
		}
		if done {
			break
		}
	}
	return nil
}

// Create builds an instance and initializes it
func Create(opts *Options) (*Instance, error) {
	domain := strings.TrimSpace(opts.Domain)
	if domain == "" || domain == ".." || domain == "." {
		return nil, ErrIllegalDomain
	}
	if strings.ContainsAny(domain, " /?#@\t\r\n\x00") {
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

	i.BytesDiskQuota = opts.DiskQuota

	i.Dev = opts.Dev

	i.PassphraseHash = nil
	i.PassphraseResetToken = nil
	i.PassphraseResetTime = time.Time{}
	i.RegisterToken = crypto.GenerateRandomBytes(registerTokenLen)
	i.SessionSecret = crypto.GenerateRandomBytes(sessionSecretLen)
	i.OAuthSecret = crypto.GenerateRandomBytes(oauthSecretLen)
	i.CLISecret = crypto.GenerateRandomBytes(oauthSecretLen)

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
	settingsDoc := &instanceSettings{
		Timezone:   opts.Timezone,
		Email:      opts.Email,
		PublicName: opts.PublicName,
	}
	if err := couchdb.CreateNamedDoc(i, settingsDoc); err != nil {
		return nil, err
	}
	if err := couchdb.DefineIndexes(i, consts.Indexes); err != nil {
		return nil, err
	}
	if err := couchdb.DefineViews(i, consts.Views); err != nil {
		return nil, err
	}
	if err := i.StartJobSystem(); err != nil {
		return nil, err
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
	cache := getCache()
	var err error
	i := cache.Get(domain)
	if i == nil {
		i, err = getFromCouch(domain)
		if err != nil {
			return nil, err
		}
		cache.Set(domain, i)
	}

	if err = i.makeVFS(); err != nil {
		return nil, err
	}
	return i, nil
}

func getFromCouch(domain string) (*Instance, error) {
	// FIXME temporary workaround to delete instances with no named indexes
	errindex := couchdb.DefineIndexes(couchdb.GlobalDB, consts.GlobalIndexes)
	if errindex != nil && !couchdb.IsNotFoundError(errindex) {
		log.Error("[instance] could not define global indexes:", errindex)
	}
	// ---
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
	req := &couchdb.AllDocsRequest{Limit: 100}
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
	return couchdb.UpdateDoc(couchdb.GlobalDB, i)
}

// Destroy is used to remove the instance. All the data linked to this
// instance will be permanently deleted.
func Destroy(domain string) (*Instance, error) {
	defer getCache().Revoke(domain)
	i, err := Get(domain)
	if err != nil {
		// FIXME temporary workaround to delete instances with no named indexes
		var instances []*Instance
		req := &couchdb.FindRequest{
			Selector: mango.Equal("domain", domain),
			Limit:    1,
		}
		if err = couchdb.FindDocs(couchdb.GlobalDB, consts.Instances, req, &instances); err != nil {
			if couchdb.IsNoDatabaseError(err) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if len(instances) == 0 {
			return nil, ErrNotFound
		}
		i = instances[0]
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

	if err = i.VFS().Delete(); err != nil {
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
	i.PassphraseResetToken = crypto.GenerateRandomBytes(passwordResetTokenLen)
	i.PassphraseResetTime = time.Now().UTC().Add(passwordResetValidityDuration)
	if err := Update(i); err != nil {
		return err
	}
	// Send a mail containing the reset url for the user to actually reset its
	// passphrase.
	resetURL := i.PageURL("/auth/passphrase_renew", url.Values{
		"token": {hex.EncodeToString(i.PassphraseResetToken)},
	})
	msg, err := jobs.NewMessage(jobs.JSONEncoding, &workers.MailOptions{
		Mode:         workers.MailModeNoReply,
		Subject:      i.Translate("Mail Password reset"),
		TemplateName: "passphrase_reset_" + i.Locale,
		TemplateValues: struct {
			BaseURL             string
			PassphraseResetLink string
		}{
			BaseURL:             i.PageURL("/", nil),
			PassphraseResetLink: resetURL,
		},
	})
	if err != nil {
		return err
	}
	_, _, err = i.JobsBroker().PushJob(&jobs.JobRequest{
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
	err = Update(i)
	if err != nil {
		log.Error("[instance] Failed to update hash in db", err)
	}

	return nil
}

// PickKey choose wich of the Instance keys to use depending on token audience
func (i *Instance) PickKey(audience string) ([]byte, error) {
	switch audience {
	case permissions.AppAudience:
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
func (i *Instance) BuildAppToken(m *apps.WebappManifest) string {
	scope := "" // apps tokens don't have a scope
	token, err := i.MakeJWT(permissions.AppAudience, m.Slug(), scope, time.Now())
	if err != nil {
		return ""
	}
	return token
}

// ensure Instance implements couchdb.Doc
var (
	_ couchdb.Doc = &Instance{}
)
