package registry

import (
	"strings"
	"time"
)

// An App is the manifest of on application on the registry.
type App struct {
	Name        string `json:"name"`
	Editor      string `json:"editor"`
	Description struct {
		En string `json:"en"`
		Fr string `json:"fr"`
	} `json:"description"`
	Repository string   `json:"repository"`
	Tags       []string `json:"tags"`
	Versions   struct {
		Stable []string `json:"stable"`
		Beta   []string `json:"beta"`
		Dev    []string `json:"dev"`
	} `json:"versions"`
}

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

const (
	onboarding = App{
		Name:   "Onboarding",
		Editor: "Cozy",
		Description: {
			En: "Register application for Cozy v3",
			Fr: "Application pour l'embarquement de Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-onboarding-v3",
		Versions: {
			Stable: []string{"3.0.0"},
		},
	}

	drive = App{
		Name:   "Drive",
		Editor: "Cozy",
		Description: {
			En: "File manager for Cozy v3",
			Fr: "Gestionnaire de fichiers pour Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-drive",
		Tags:       []string{"files"},
		Versions: {
			Stable: []string{"0.3.5", "0.3.4"},
		},
	}

	photos = App{
		Name:   "Photos",
		Editor: "Cozy",
		Description: {
			En: "Photos manager for Cozy v3",
			Fr: "Gestionnaire de photos pour Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-photos-v3",
		Tags:       []string{"albums"},
		Versions: {
			Stable: []string{"3.0.0"},
		},
	}

	settings = App{
		Name:   "Settings",
		Editor: "Cozy",
		Description: {
			En: "Settings manager for Cozy v3",
			Fr: "Gestionnaire de paramètres pour Cozy v3",
		},
		Repository: "https://github.com/cozy/cozy-settings",
		Tags:       []string{"profile"},
		Versions: {
			Stable: []string{"3.0.3"},
		},
	}

	collect = App{
		Name:   "Collect",
		Editor: "Cozy",
		Description: {
			En: "Configuration application for konnectors",
			Fr: "Application de configuration pour les konnectors",
		},
		Repository: "https://github.com/cozy/cozy-collect",
		Tags:       []string{"konnectors"},
		Versions: {
			Stable: []string{"3.0.3"},
		},
	}

	webapps = []App{onboarding, drive, photos, settings, collect}
)

// FindBySlug returns the application with the given slug, or nil if not found.
func FindBySlug(slug string) *App {
	for _, app := range webapps {
		if strings.ToLower(app.Name) == slug {
			return &app
		}
	}
	return nil
}
