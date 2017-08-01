package registry

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
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

	vers []*Version
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
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	URL       string          `json:"url"`
	Sha256    string          `json:"sha256"`
	CreatedAt time.Time       `json:"created_at"`
	Size      string          `json:"size"`
	Manifest  json.RawMessage `json:"manifest"`
	TarPrefix string          `json:"tar_prefix"`
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
		Tags:       []string{"welcome"},
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
		vers: []*Version{
			{
				Name:      "Collect",
				Version:   "3.0.3",
				URL:       "https://github.com/cozy/cozy-collect/releases/download/v3.0.3/cozy-collect-v3.0.3.tgz",
				Sha256:    "1332d2301c2362f207cf35880725179157368a921253293b062946eb6d96e3ae",
				CreatedAt: time.Now(),
				Size:      "3821149",
				TarPrefix: "cozy-collect-v3.0.3",
				Manifest: json.RawMessage(`{
"name": "Collect",
"slug": "collect",
"icon": "cozy_collect.svg",
"description": "Configuration application for konnectors",
"category": "cozy",
"source": "https://github.com/cozy/cozy-collect.git@build",
"editor": "Cozy",
"developer": {
  "name": "Cozy",
  "url": "https://cozy.io"
},
"default_locale": "en",
"locales": {
  "fr": {
    "description": "Application de configuration pour les konnectors"
  }
},
"version": "3.0.3",
"licence": "AGPL-3.0",
"permissions": {
  "apps": {
    "description": "Required by the cozy-bar to display the icons of the apps",
    "type": "io.cozy.apps",
    "verbs": ["GET", "POST", "PUT"]
  },
  "settings": {
    "description": "Required by the cozy-bar display Claudy and to know which applications are coming soon",
    "type": "io.cozy.settings",
    "verbs": ["GET"]
  },
  "konnectors": {
    "description": "Required to get the list of konnectors",
    "type": "io.cozy.konnectors",
    "verbs": ["GET", "POST", "PUT", "DELETE"]
  },
  "konnectors results": {
    "description": "Required to get the list of konnectors results",
    "type": "io.cozy.konnectors.result",
    "verbs": ["GET"]
  },
  "accounts": {
    "description": "Required to manage accounts associated to konnectors",
    "type": "io.cozy.accounts",
    "verbs": ["GET", "POST", "PUT", "DELETE"]
  },
  "files": {
    "description": "Required to access folders",
    "type": "io.cozy.files"
  },
  "jobs": {
    "description": "Required to run the konnectors",
    "type": "io.cozy.jobs"
  },
  "triggers": {
    "description": "Required to run the konnectors",
    "type": "io.cozy.triggers"
  },
  "permissions": {
    "description": "Required to run the konnectors",
    "verbs": ["ALL"],
    "type": "io.cozy.permissions"
  }
},
"routes": {
  "/": {
    "folder": "/",
    "index": "index.html",
    "public": false
  },
  "/services": {
    "folder": "/services",
    "index": "index.html",
    "public": false
  }
},
"intents": [{
  "action": "CREATE",
  "type": ["io.cozy.accounts"],
  "href": "/services"
}]}`),
			},
		},
	}

	webapps = []*App{onboarding, drive, photos, settings, collect}
)

// All returns all the (webapps|konnectors) applications.
func All(appType string) []*App {
	return webapps
}

// FindBySlug returns the application with the given slug.
func FindBySlug(slug string) (*App, error) {
	for _, app := range webapps {
		if strings.ToLower(app.Name) == slug {
			return app, nil
		}
	}
	return nil, ErrAppNotFound
}

// GetAppVersion returns a version object for an app + version number.
func GetAppVersion(slug, number string) (*Version, error) {
	app, err := FindBySlug(slug)
	if err != nil {
		return nil, err
	}
	for _, v := range app.vers {
		if v.Version == number {
			return v, nil
		}
	}
	return nil, ErrVersionNotFound
}

// GetAppLatestVersion returns the latest version for an app.
func GetAppLatestVersion(slug string) (*Version, error) {
	app, err := FindBySlug(slug)
	if err != nil {
		return nil, err
	}
	if len(app.vers) == 0 {
		return nil, ErrVersionNotFound
	}
	return app.vers[len(app.vers)-1], nil
}
