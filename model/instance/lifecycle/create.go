package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/hooks"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/utils"
)

// Options holds the parameters to create a new instance.
type Options struct {
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
	SettingsObj        *couchdb.JSONDoc
	AuthMode           string
	Passphrase         string
	Key                string
	KdfIterations      int
	SwiftLayout        int
	DiskQuota          int64
	Apps               []string
	AutoUpdate         *bool
	Debug              *bool
	Deleting           *bool
	Traced             *bool
	OnboardingFinished *bool
	Blocked            *bool
	BlockingReason     string
}

func (opts *Options) trace(name string, do func()) {
	if opts.Traced != nil && *opts.Traced {
		t := time.Now()
		defer func() {
			elapsed := time.Since(t)
			logger.
				WithDomain("admin").
				WithField("nspace", "trace").
				Printf("%s: %v", name, elapsed)
		}()
	}
	do()
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
	domain := opts.Domain
	var err error
	opts.trace("validate domain", func() {
		domain, err = validateDomain(domain)
	})
	if err != nil {
		return nil, err
	}
	opts.trace("check if instance already exist", func() {
		_, err = instance.GetFromCouch(domain)
	})
	if err != instance.ErrNotFound {
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
	i.OIDCID = opts.OIDCID
	i.TOSSigned = opts.TOSSigned
	i.TOSLatest = opts.TOSLatest
	i.ContextName = opts.ContextName
	i.BytesDiskQuota = opts.DiskQuota
	i.IndexViewsVersion = couchdb.IndexViewsVersion
	opts.trace("generate secrets", func() {
		i.RegisterToken = crypto.GenerateRandomBytes(instance.RegisterTokenLen)
		i.SessSecret = crypto.GenerateRandomBytes(instance.SessionSecretLen)
		i.OAuthSecret = crypto.GenerateRandomBytes(instance.OauthSecretLen)
		i.CLISecret = crypto.GenerateRandomBytes(instance.OauthSecretLen)
	})

	switch config.FsURL().Scheme {
	case config.SchemeSwift, config.SchemeSwiftSecure:
		switch opts.SwiftLayout {
		case 0:
			return nil, errors.New("Swift layout v1 disabled for instance creation")
		case 1, 2:
			i.SwiftLayout = opts.SwiftLayout
		default:
			i.SwiftLayout = config.GetConfig().Fs.DefaultLayout
		}
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
		opts.KdfIterations = crypto.DefaultPBKDF2Iterations
	}

	if opts.Passphrase != "" {
		opts.trace("register passphrase", func() {
			err = registerPassphrase(i, i.RegisterToken, PassParameters{
				Pass:       []byte(opts.Passphrase),
				Iterations: opts.KdfIterations,
				Key:        opts.Key,
			})
		})
		if err != nil {
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

	opts.trace("init couchdb", func() {
		if err = couchdb.CreateDoc(couchdb.GlobalDB, i); err != nil {
			return
		}
		if err = couchdb.CreateDB(i, consts.Files); err != nil {
			return
		}
		if err = couchdb.CreateDB(i, consts.Apps); err != nil {
			return
		}
		if err = couchdb.CreateDB(i, consts.Konnectors); err != nil {
			return
		}
		if err = couchdb.CreateDB(i, consts.OAuthClients); err != nil {
			return
		}
		if err = couchdb.CreateDB(i, consts.Permissions); err != nil {
			return
		}
		if err = couchdb.CreateDB(i, consts.Sharings); err != nil {
			return
		}
		if err = couchdb.CreateNamedDocWithDB(i, settings); err != nil {
			return
		}
		_, err = contact.CreateMyself(i, settings)
	})
	if err != nil {
		return nil, err
	}

	opts.trace("init VFS", func() {
		if err = i.MakeVFS(); err != nil {
			return
		}
		if err = i.VFS().InitFs(); err != nil {
			return
		}
		err = createDefaultFilesTree(i)
	})
	if err != nil {
		return nil, err
	}

	opts.trace("define views and indexes", func() {
		err = DefineViewsAndIndex(i)
	})
	if err != nil {
		return nil, err
	}

	opts.trace("add triggers", func() {
		sched := job.System()
		for _, trigger := range Triggers(i) {
			var t job.Trigger
			t, err = job.NewTrigger(i, trigger, nil)
			if err != nil {
				return
			}
			if err = sched.AddTrigger(t); err != nil {
				return
			}
		}
	})
	if err != nil {
		return nil, err
	}

	opts.trace("install apps", func() {
		for _, app := range opts.Apps {
			if err := installApp(i, app); err != nil {
				i.Logger().Errorf("Failed to install %s: %s", app, err)
			}
		}
	})

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
	if oidcID, ok := settings.M["oidc_id"].(string); ok {
		opts.OIDCID = oidcID
		delete(settings.M, "oidc_id")
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
