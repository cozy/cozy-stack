package auth

import (
	"fmt"
	"html/template"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/stretchr/testify/assert"
)

func Test_DeprecatedAppList(t *testing.T) {
	t.Run("IsDeprecated", func(t *testing.T) {
		list := NewDeprecatedAppList([]DeprecatedApp{
			{
				SoftwareID: "github.com/cozy/super-app",
				Name:       "Super App",
				StoreURLs: map[string]string{
					"Android": "https://some-android/url",
					"Iphone":  "https://some-IOS/url",
				},
			},
		})

		isDeprecated := list.IsDeprecated(&oauth.Client{
			ClientID:     "some-id",
			ClientSecret: "some-secret",
			RedirectURIs: []string{"http://localhost"},
			ClientName:   "Super App",
			ClientKind:   "mobile",
			SoftwareID:   "github.com/cozy/super-app",
		})
		assert.True(t, isDeprecated)
	})

	t.Run("RenderArgs", func(t *testing.T) {
		config.UseTestFile()
		middlewares.FuncsMap = template.FuncMap{
			"t":         fmt.Sprintf,
			"tHTML":     fmt.Sprintf,
			"split":     strings.Split,
			"replace":   strings.Replace,
			"hasSuffix": strings.HasSuffix,
			"asset":     statik.AssetPath,
		}
		middlewares.BuildTemplates()

		oauthClient := &oauth.Client{
			ClientID:     "some-id",
			ClientSecret: "some-secret",
			RedirectURIs: []string{"http://localhost"},
			ClientName:   "Super App",
			ClientKind:   "mobile",
			SoftwareID:   "github.com/cozy/super-app",
			// User on Android Firefox
			ClientOS: "Mozilla/5.0 (Android; Mobile; rv:17.0) Gecko/17.0 Firefox/17.0",
		}

		inst := &instance.Instance{
			ContextName: "some-context",
			Domain:      "foobar.cozy.local",
			Locale:      "en",
		}

		list := NewDeprecatedAppList([]DeprecatedApp{
			{
				SoftwareID: "github.com/cozy/super-app",
				Name:       "Super App",
				StoreURLs: map[string]string{
					"Android": "https://some-android/url",
					"Iphone":  "https://some-IOS/url",
				},
			},
		})

		args := list.RenderArgs(oauthClient, inst)
		assert.Equal(t, map[string]interface{}{
			"Domain":      "foobar.cozy.local",
			"ContextName": "some-context",
			"Locale":      "en",
			"Title":       instance.DefaultTemplateTitle,
			"Favicon":     middlewares.Favicon(inst),
			"AppName":     "Super App",
			"OS":          "Android",
			"StoreURL":    "https://some-android/url",
		}, args)
	})
}
