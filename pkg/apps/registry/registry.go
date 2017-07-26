package registry

import (
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

// AppDescription is the embedded description of an application.
type AppDescription struct {
	En string `json:"en"`
	Fr string `json:"fr"`
}

// AppVersions is the embedded versions of an application.
type AppVersions struct {
	Stable []string `json:"stable"`
	Beta   []string `json:"beta"`
	Dev    []string `json:"dev"`
}

// An App is the manifest of on application on the registry.
type App struct {
	appType     string
	Name        string         `json:"name"`
	Editor      string         `json:"editor"`
	Description AppDescription `json:"description"`
	Repository  string         `json:"repository"`
	Tags        []string       `json:"tags"`
	Versions    AppVersions    `json:"versions"`
}

// ID is used to implement the couchdb.Doc aterface
func (a *App) ID() string { return a.Name }

// Rev is used to implement the couchdb.Doc aterface
func (a *App) Rev() string { return "" }

// DocType is used to implement the couchdb.Doc aterface
func (a *App) DocType() string { return "io.cozy.registry." + a.appType + "s" }

// Clone implements couchdb.Doc
func (a *App) Clone() couchdb.Doc {
	cloned := *a
	return &cloned
}

// SetID is used to implement the couchdb.Doc aterface
func (a *App) SetID(id string) {}

// SetRev is used to implement the couchdb.Doc aterface
func (a *App) SetRev(rev string) {}

// A Version describes a specific release of an application.
type Version struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	URL         string          `json:"url"`
	Sha256      string          `json:"sha256"`
	CreatedAt   time.Time       `json:"created_at"`
	Size        string          `json:"size"`
	Description string          `json:"description"`
	License     string          `json:"license"`
	Permissions permissions.Set `json:"permissions"`
	Locales     map[string]struct {
		Description string `json:"description"`
	} `json:"locales"`
}

// ID is used to implement the couchdb.Doc aterface
func (v *Version) ID() string { return v.Name + "/" + v.Version }

// Rev is used to implement the couchdb.Doc aterface
func (v *Version) Rev() string { return "" }

// DocType is used to implement the couchdb.Doc aterface
func (v *Version) DocType() string { return consts.Versions }

// Clone implements couchdb.Doc
func (v *Version) Clone() couchdb.Doc {
	cloned := *v
	return &cloned
}

// SetID is used to implement the couchdb.Doc aterface
func (v *Version) SetID(id string) {}

// SetRev is used to implement the couchdb.Doc aterface
func (v *Version) SetRev(rev string) {}

var (
	onboarding = &App{
		appType: "webapp",
		Name:    "Onboarding",
		Editor:  "Cozy",
		Description: AppDescription{
			En: "Register application for Cozy v3",
			Fr: "Application pour l'embarquement de Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-onboarding-v3",
		Versions: AppVersions{
			Stable: []string{"3.0.0"},
		},
	}

	drive = &App{
		appType: "webapp",
		Name:    "Drive",
		Editor:  "Cozy",
		Description: AppDescription{
			En: "File manager for Cozy v3",
			Fr: "Gestionnaire de fichiers pour Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-drive",
		Tags:       []string{"files"},
		Versions: AppVersions{
			Stable: []string{"0.3.5", "0.3.4"},
		},
	}

	photos = &App{
		appType: "webapp",
		Name:    "Photos",
		Editor:  "Cozy",
		Description: AppDescription{
			En: "Photos manager for Cozy v3",
			Fr: "Gestionnaire de photos pour Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-photos-v3",
		Tags:       []string{"albums"},
		Versions: AppVersions{
			Stable: []string{"3.0.0"},
		},
	}

	settings = &App{
		appType: "webapp",
		Name:    "Settings",
		Editor:  "Cozy",
		Description: AppDescription{
			En: "Settings manager for Cozy v3",
			Fr: "Gestionnaire de paramètres pour Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-settings",
		Tags:       []string{"profile"},
		Versions: AppVersions{
			Stable: []string{"3.0.3"},
		},
	}

	collect = &App{
		appType: "webapp",
		Name:    "Collect",
		Editor:  "Cozy",
		Description: AppDescription{
			En: "Configuration application for konnectors",
			Fr: "Application de configuration pour les konnectors",
		},
		Repository: "https://github.com/cozy/cozy-collect",
		Tags:       []string{"konnectors"},
		Versions: AppVersions{
			Stable: []string{"3.0.3"},
		},
	}

	collectThreeOhThree = &Version{
		Name:        "Collect",
		Version:     "3.0.3",
		URL:         "https://github.com/cozy/cozy-collect/releases/download/v3.0.3/cozy-collect-v3.0.3.tgz",
		Sha256:      "1332d2301c2362f207cf35880725179157368a921253293b062946eb6d96e3ae",
		CreatedAt:   time.Now(),
		Size:        "3821149",
		Description: "Configuration application for konnectors",
		License:     "AGPL-3.0",
		Permissions: permissions.Set{
			permissions.Rule{
				Title:       "apps",
				Type:        "io.cozy.apps",
				Description: "Required by the cozy-bar to display the icons of the apps",
				Verbs:       permissions.Verbs(permissions.GET, permissions.POST, permissions.PUT),
			},
			permissions.Rule{
				Title:       "settings",
				Type:        "io.cozy.settings",
				Description: "Required by the cozy-bar display Claudy and to know which applications are coming soon",
				Verbs:       permissions.Verbs(permissions.GET),
			},
			permissions.Rule{
				Title:       "konnectors",
				Type:        "io.cozy.konnectors",
				Description: "Required to get the list of konnectors",
				Verbs:       permissions.Verbs(permissions.GET, permissions.POST, permissions.PUT, permissions.DELETE),
			},
			permissions.Rule{
				Title:       "konnectors results",
				Description: "Required to get the list of konnectors results",
				Type:        "io.cozy.konnectors.result",
				Verbs:       permissions.Verbs(permissions.GET),
			},
			permissions.Rule{
				Title:       "accounts",
				Description: "Required to manage accounts associated to konnectors",
				Type:        "io.cozy.accounts",
				Verbs:       permissions.Verbs(permissions.GET, permissions.POST, permissions.PUT, permissions.DELETE),
			},
			permissions.Rule{
				Title:       "files",
				Description: "Required to access folders",
				Verbs:       permissions.ALL,
				Type:        "io.cozy.files",
			},
			permissions.Rule{
				Title:       "jobs",
				Description: "Required to run the konnectors",
				Verbs:       permissions.ALL,
				Type:        "io.cozy.jobs",
			},
			permissions.Rule{
				Title:       "triggers",
				Description: "Required to run the konnectors",
				Verbs:       permissions.ALL,
				Type:        "io.cozy.triggers",
			},
			permissions.Rule{
				Title:       "permissions",
				Description: "Required to run the konnectors",
				Verbs:       permissions.ALL,
				Type:        "io.cozy.permissions",
			},
		},
		Locales: map[string]struct {
			Description string `json:"description"`
		}{
			"fr": {"Application de configuration pour les konnectors"},
		},
	}

	webapps = []*App{onboarding, drive, photos, settings, collect}

	webappsVersions = []*Version{collectThreeOhThree}
)

// All returns all the (webapps|konnectors) applications.
func All(appType apps.AppType) []*App {
	return webapps
}

// FindBySlug returns the application with the given slug.
func FindBySlug(appType apps.AppType, slug string) (*App, error) {
	for _, app := range All(appType) {
		if strings.ToLower(app.Name) == slug {
			return app, nil
		}
	}
	return nil, ErrAppNotFound
}

// GetAppVersion returns a version object for an app + version number.
func GetAppVersion(appType apps.AppType, name, number string) (*Version, error) {
	for _, v := range webappsVersions {
		if v.Name == name && v.Version == number {
			return v, nil
		}
	}
	return nil, ErrVersionNotFound
}
