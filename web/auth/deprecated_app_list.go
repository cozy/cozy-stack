package auth

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/mssola/user_agent"
)

const DefaultStoreURL = "https://cozy.io/fr/download/"
const DefaultStoreName = "Your store"

type DeprecatedApp struct {
	// SoftwareID found inside the oauth client.
	SoftwareID string
	// Name as printed to the user.
	Name string

	StoreURLs map[string]string
}

// DeprecatedAppList lists and detects the deprecated apps.
type DeprecatedAppList struct {
	apps []DeprecatedApp
}

// NewDeprecatedAppList instantiates a new [DeprecatedAppList].
func NewDeprecatedAppList(apps []DeprecatedApp) *DeprecatedAppList {
	return &DeprecatedAppList{apps}
}

// IsDeprecated returns true if the givent client is marked as deprectated.
func (d *DeprecatedAppList) IsDeprecated(client *oauth.Client) bool {
	for _, app := range d.apps {
		if client.SoftwareID == app.SoftwareID {
			return true
		}
	}

	return false
}

func (d *DeprecatedAppList) RenderArgs(client *oauth.Client, inst *instance.Instance) map[string]interface{} {
	var app DeprecatedApp

	for _, a := range d.apps {
		if client.SoftwareID == a.SoftwareID {
			app = a
			break
		}
	}

	os := user_agent.New(client.ClientOS).OS()

	storeURL := DefaultStoreURL
	if url, ok := app.StoreURLs[os]; ok {
		storeURL = url
	}

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
