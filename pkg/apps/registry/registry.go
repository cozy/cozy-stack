package registry

import (
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/couchdb"
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
	Version     string    `json:"version"`
	URL         string    `json:"url"`
	Sha256      string    `json:"sha256"`
	CreatedAt   time.Time `json:"created_at"`
	Size        string    `json:"size"`
	Description string    `json:"description"`
	License     string    `json:"license"`
	Permissions struct {
		Apps struct {
			Description string   `json:"description"`
			Type        string   `json:"type"`
			Verbs       []string `json:"verbs"`
		} `json:"apps"`
		Settings struct {
			Description string   `json:"description"`
			Type        string   `json:"type"`
			Verbs       []string `json:"verbs"`
		} `json:"settings"`
	} `json:"permissions"`
	Locales struct {
		Fr struct {
			Description string `json:"description"`
		} `json:"fr"`
	} `json:"locales"`
}

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

	webapps = []*App{onboarding, drive, photos, settings, collect}
)

// All returns all the (webapps|konnectors) applications.
func All(appType apps.AppType) []*App {
	return webapps
}

// FindBySlug returns the application with the given slug, or nil if not found.
func FindBySlug(appType apps.AppType, slug string) *App {
	for _, app := range All(appType) {
		if strings.ToLower(app.Name) == slug {
			return app
		}
	}
	return nil
}
