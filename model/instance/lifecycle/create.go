package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/hooks"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/utils"
)

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

// Create builds an instance and initializes it
func Create(opts *Options) (*instance.Instance, error) {
	domain, err := validateDomain(opts.Domain)
	if err != nil {
		return nil, err
	}
	var inst *instance.Instance
	err = hooks.Execute("add-instance", []string{domain}, func() error {
		var err2 error
		inst, err2 = CreateWithoutHooks(opts)
		return err2
	})
	return inst, err
}

// CreateWithoutHooks builds an instance and initializes it. The difference
// with Create is that script hooks are not executed for this function.
func CreateWithoutHooks(opts *Options) (*instance.Instance, error) {
	domain, err := validateDomain(opts.Domain)
	if err != nil {
		return nil, err
	}
	if _, err = instance.GetFromCouch(domain); err != instance.ErrNotFound {
		if err == nil {
			err = instance.ErrExists
		}
		return nil, err
	}

	locale := opts.Locale
	if locale == "" {
		locale = consts.DefaultLocale
	}

	settings := buildSettings(opts)
	prefix := sha256.Sum256([]byte(domain))
	i := &instance.Instance{}
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
	i.IndexViewsVersion = couchdb.IndexViewsVersion
	i.RegisterToken = crypto.GenerateRandomBytes(instance.RegisterTokenLen)
	i.SessionSecret = crypto.GenerateRandomBytes(instance.SessionSecretLen)
	i.OAuthSecret = crypto.GenerateRandomBytes(instance.OauthSecretLen)
	i.CLISecret = crypto.GenerateRandomBytes(instance.OauthSecretLen)

	// If not cluster number is given, we rely on cluster one.
	if opts.SwiftCluster == 0 {
		i.SwiftCluster = 1
	} else {
		i.SwiftCluster = opts.SwiftCluster
	}

	if opts.AuthMode != "" {
		var authMode instance.AuthMode
		if authMode, err = instance.StringToAuthMode(opts.AuthMode); err == nil {
			i.AuthMode = authMode
		}
	}

	// If the authentication is disabled, we force a random password. It won't
	// be known by the user and cannot be used to authenticate. It will only be
	// used if the configuration is changed later: the user will be able to
	// reset the passphrase.
	if !i.IsPasswordAuthenticationEnabled() {
		opts.Passphrase = utils.RandomString(instance.RegisterTokenLen)
	}

	if opts.Passphrase != "" {
		if err = registerPassphrase(i, []byte(opts.Passphrase), i.RegisterToken); err != nil {
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
	if err := i.MakeVFS(); err != nil {
		return nil, err
	}
	if err := i.VFS().InitFs(); err != nil {
		return nil, err
	}
	if err := couchdb.CreateNamedDoc(i, settings); err != nil {
		return nil, err
	}
	if err := defineViewsAndIndex(i); err != nil {
		return nil, err
	}
	if err := createDefaultFilesTree(i); err != nil {
		return nil, err
	}
	sched := job.System()
	for _, trigger := range Triggers(i) {
		t, err := job.NewTrigger(i, trigger, nil)
		if err != nil {
			return nil, err
		}
		if err = sched.AddTrigger(t); err != nil {
			return nil, err
		}
	}
	for _, app := range opts.Apps {
		if err := installApp(i, app); err != nil {
			i.Logger().Errorf("Failed to install %s: %s", app, err)
		}
	}
	return i, nil
}

func buildSettings(opts *Options) *couchdb.JSONDoc {
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

	// Handling global/instance settings
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

	// Handling instance settings document
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

	return settings
}

// Triggers returns the list of the triggers to add when an instance is created
func Triggers(db prefixer.Prefixer) []job.TriggerInfos {
	// Create/update/remove thumbnails when an image is created/updated/removed
	return []job.TriggerInfos{
		{
			Domain:     db.DomainName(),
			Prefix:     db.DBPrefix(),
			Type:       "@event",
			WorkerType: "thumbnail",
			Arguments:  "io.cozy.files:CREATED,UPDATED,DELETED:image:class",
		},
	}
}
