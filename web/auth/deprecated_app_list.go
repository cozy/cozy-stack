package auth

import (
	"html/template"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/mssola/user_agent"
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

func (d *DeprecatedAppList) RenderArgs(client *oauth.Client, inst *instance.Instance, uaStr string) map[string]interface{} {
	ua := user_agent.New(uaStr)

	var app config.DeprecatedApp

	for _, a := range d.apps {
		if client.SoftwareID == a.SoftwareID {
			app = a
			break
		}
	}

	platform := strings.ToLower(ua.Platform())

	if strings.Contains(strings.ToLower(ua.OS()), "android") ||
		strings.Contains(strings.ToLower(uaStr), "android") {
		platform = "android"
	}

	if strings.Contains(strings.ToLower(ua.OS()), "iphone") ||
		strings.Contains(strings.ToLower(uaStr), "iphone") ||
		platform == "ipad" {
		platform = "iphone"
	}

	if platform != "iphone" && platform != "android" {
		platform = "other"
	}

	storeURL := DefaultStoreURL
	if url, ok := app.StoreURLs[platform]; ok {
		storeURL = url
	}

	d.logger.WithDomain(inst.Domain).
		WithField("platform", platform).
		WithField("app", app.Name).
		Info("Deprecated app detected, stop authentication")

	res := map[string]interface{}{
		"Domain":      inst.ContextualDomain(),
		"ContextName": inst.ContextName,
		"Locale":      inst.Locale,
		"Title":       inst.TemplateTitle(),
		"Favicon":     middlewares.Favicon(inst),
		"AppName":     app.Name,
		"Platform":    platform,
		// template.URL is used in order to avoid the discard of url starting
		// by `market://` (the url is replaced by "#ZgotmplZ" in case of discard).
		//
		// More details at https://github.com/golang/go/blob/bce7aec3cdca8580585095007e9b7cea11a8812f/src/html/template/url.go#L19
		"StoreURL": template.URL(storeURL),
	}

	return res
}
