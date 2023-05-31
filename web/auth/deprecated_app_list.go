package auth

import (
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/web/middlewares"
)

const DefaultStoreURL = "https://cozy.io/fr/download/"

// DeprecatedAppList lists and detects the deprecated apps.
type DeprecatedAppList struct {
	apps   []config.DeprecatedApp
	logger *logger.Entry
}

// NewDeprecatedAppList instantiates a new [DeprecatedAppList].
func NewDeprecatedAppList(cfg config.DeprecatedAppsCfg) *DeprecatedAppList {
	return &DeprecatedAppList{
		apps:   cfg.Apps,
		logger: logger.WithNamespace("deprecated"),
	}
}

// IsDeprecated returns true if the given client is marked as deprectated.
func (d *DeprecatedAppList) IsDeprecated(client *oauth.Client) bool {
	for _, app := range d.apps {
		if client.SoftwareID == app.SoftwareID {
			return true
		}
	}

	return false
}

func (d *DeprecatedAppList) RenderArgs(client *oauth.Client, inst *instance.Instance) map[string]interface{} {
	var app config.DeprecatedApp

	for _, a := range d.apps {
		if client.SoftwareID == a.SoftwareID {
			app = a
			break
		}
	}

	os := strings.ToLower(client.ClientOS)

	storeURL := DefaultStoreURL
	if url, ok := app.StoreURLs[os]; ok {
		storeURL = url
	}

	d.logger.WithDomain(inst.Domain).
		WithField("os", os).
		WithField("app", app.Name).
		Info("Deprecated app detected, stop authentication")

	res := map[string]interface{}{
		"Domain":      inst.ContextualDomain(),
		"ContextName": inst.ContextName,
		"Locale":      inst.Locale,
		"Title":       inst.TemplateTitle(),
		"Favicon":     middlewares.Favicon(inst),
		"AppName":     app.Name,
		"OS":          os,
		"StoreURL":    storeURL,
	}

	return res
}
