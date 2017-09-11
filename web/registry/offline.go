package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"
)

var offlineRegistry *url.URL

type appDescription struct {
	En string `json:"en"`
	Fr string `json:"fr"`
}

type appVersions struct {
	Stable []string `json:"stable"`
	Beta   []string `json:"beta"`
	Dev    []string `json:"dev"`
}

type app struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Editor      string         `json:"editor"`
	Description appDescription `json:"description"`
	Repository  string         `json:"repository"`
	Tags        []string       `json:"tags"`
	Versions    appVersions    `json:"versions"`

	vers []*version
}

type version struct {
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	URL       string          `json:"url"`
	Sha256    string          `json:"sha256"`
	CreatedAt time.Time       `json:"created_at"`
	Size      string          `json:"size"`
	Manifest  json.RawMessage `json:"manifest"`
	TarPrefix string          `json:"tar_prefix"`
}

var (
	onboarding = &app{
		Name:   "onboarding",
		Type:   "webapp",
		Editor: "Cozy",
		Description: appDescription{
			En: "Register application for Cozy v3",
			Fr: "Application pour l'embarquement de Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-onboarding-v3",
		Tags:       []string{"welcome"},
		Versions: appVersions{
			Stable: []string{"3.0.0"},
		},
	}

	drive = &app{
		Name:   "drive",
		Type:   "webapp",
		Editor: "Cozy",
		Description: appDescription{
			En: "File manager for Cozy v3",
			Fr: "Gestionnaire de fichiers pour Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-drive",
		Tags:       []string{"files"},
		Versions: appVersions{
			Stable: []string{"0.3.5", "0.3.4"},
		},
	}

	photos = &app{
		Name:   "photos",
		Type:   "webapp",
		Editor: "Cozy",
		Description: appDescription{
			En: "Photos manager for Cozy v3",
			Fr: "Gestionnaire de photos pour Cozy v3",
		},
		Repository: "https://github.com/cozy/drive",
		Tags:       []string{"albums"},
		Versions: appVersions{
			Stable: []string{"3.0.0"},
		},
	}

	settings = &app{
		Name:   "settings",
		Type:   "webapp",
		Editor: "Cozy",
		Description: appDescription{
			En: "Settings manager for Cozy v3",
			Fr: "Gestionnaire de paramètres pour Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-settings",
		Tags:       []string{"profile"},
		Versions: appVersions{
			Stable: []string{"3.0.3"},
		},
	}

	collect = &app{
		Name:   "collect",
		Type:   "webapp",
		Editor: "Cozy",
		Description: appDescription{
			En: "Configuration application for konnectors",
			Fr: "Application de configuration pour les konnectors",
		},
		Repository: "https://github.com/cozy/cozy-collect",
		Tags:       []string{"konnectors"},
		Versions: appVersions{
			Stable: []string{"3.0.3"},
		},
		vers: []*version{
			{
				Name:      "collect",
				Version:   "3.0.3",
				URL:       "https://github.com/cozy/cozy-collect/releases/download/v3.0.3/cozy-collect-v3.0.3.tgz",
				Sha256:    "1332d2301c2362f207cf35880725179157368a921253293b062946eb6d96e3ae",
				CreatedAt: time.Now(),
				Size:      "3821149",
				TarPrefix: "cozy-collect-v3.0.3",
				Manifest: json.RawMessage(`{
"name": "collect",
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

	webapps = []*app{onboarding, drive, photos, settings, collect}
)

func getApp(appName string) (*app, bool) {
	appName = strings.ToLower(appName)
	for _, app := range webapps {
		if strings.ToLower(app.Name) == appName {
			return app, true
		}
	}
	return nil, false
}

func getVersion(appName, number string) (*version, bool) {
	app, ok := getApp(appName)
	if !ok {
		return nil, false
	}
	for _, v := range app.vers {
		if v.Version == number {
			return v, true
		}
	}
	return nil, false
}

func getLatestVersion(appName string) (*version, bool) {
	app, ok := getApp(appName)
	if !ok {
		return nil, false
	}
	if len(app.vers) == 0 {
		return nil, false
	}
	return app.vers[len(app.vers)-1], true
}

func init() {
	var mux = http.NewServeMux()
	mux.HandleFunc("/registry", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		resp := map[string]interface{}{
			"data": webapps,
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/registry/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		path := req.URL.Path
		if path == "/registry/" {
			json.NewEncoder(w).Encode(webapps)
			return
		}
		path = path[len("/registry/"):]
		paths := strings.Split(path, "/")
		var appName, versionNumber, channel string
		switch len(paths) {
		case 1:
			appName = paths[0]
		case 2:
			appName, versionNumber = paths[0], paths[1]
		case 3:
			if paths[2] == "latest" {
				appName, channel = paths[0], paths[1]
			}
		}
		var v interface{}
		var ok bool
		if appName != "" {
			if channel != "" {
				v, ok = getLatestVersion(appName)
			} else if versionNumber != "" {
				v, ok = getVersion(appName, versionNumber)
			} else {
				v, ok = getApp(appName)
			}
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
		} else {
			json.NewEncoder(w).Encode(v)
		}
	})

	server := httptest.NewServer(mux)
	offlineRegistry, _ = url.Parse(server.URL)
}
