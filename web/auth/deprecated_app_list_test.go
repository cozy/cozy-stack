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
		list := NewDeprecatedAppList(config.DeprecatedAppsCfg{
			Apps: []config.DeprecatedApp{
				{
					SoftwareID: "github.com/cozy/super-app",
					Name:       "Super App",
					StoreURLs: map[string]string{
						"android": "https://some-android/url",
						"iphone":  "https://some-IOS/url",
					},
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
		tests := []struct {
			Name             string
			UA               string
			ExpectedPlatform string
			ExpectedURL      string
		}{
			{
				Name:             "with a classic android",
				UA:               "Mozilla/5.0 (Linux; U; Android 1.5; de-; HTC Magic Build/PLAT-RC33) AppleWebKit/528.5+ (KHTML, like Gecko) Version/3.1.2 Mobile Safari/525.20.1",
				ExpectedPlatform: "android",
				ExpectedURL:      "https://some-android/url",
			},
			{
				Name:             "with a firefox on ipad",
				UA:               "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/24.1 Safari/605.1.15",
				ExpectedPlatform: "iphone",
				ExpectedURL:      "https://some-IOS/url",
			},
			{
				Name:             "with an iphone",
				UA:               "Mozilla/5.0 (iPhone; U; CPU like Mac OS X; en) AppleWebKit/420.1 (KHTML, like Gecko) Version/3.0 Mobile/4A102 Safari/419",
				ExpectedPlatform: "iphone",
				ExpectedURL:      "https://some-IOS/url",
			},
			{
				Name:             "with an iphone",
				UA:               "Mozilla/5.0 (iPhone; U; CPU like Mac OS X; en) AppleWebKit/420.1 (KHTML, like Gecko) Version/3.0 Mobile/4A102 Safari/419",
				ExpectedPlatform: "iphone",
				ExpectedURL:      "https://some-IOS/url",
			},
			{
				Name:             "with an non mobile browser (chrome - ubuntu)",
				UA:               "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/55.0.2883.75 Safari/537.36",
				ExpectedPlatform: "other",
				ExpectedURL:      DefaultStoreURL,
			},
		}

		for _, test := range tests {
			t.Run(test.Name, func(t *testing.T) {
				config.UseTestFile(t)
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
				}

				inst := &instance.Instance{
					ContextName: "some-context",
					Domain:      "foobar.cozy.local",
					Locale:      "en",
				}

				list := NewDeprecatedAppList(config.DeprecatedAppsCfg{
					Apps: []config.DeprecatedApp{
						{
							SoftwareID: "github.com/cozy/super-app",
							Name:       "Super App",
							StoreURLs: map[string]string{
								"android": "https://some-android/url",
								"iphone":  "https://some-IOS/url",
							},
						},
					},
				})

				args := list.RenderArgs(oauthClient, inst, test.UA)
				assert.Equal(t, map[string]interface{}{
					"Domain":      "foobar.cozy.local",
					"ContextName": "some-context",
					"Locale":      "en",
					"Title":       instance.DefaultTemplateTitle,
					"Favicon":     middlewares.Favicon(inst),
					"AppName":     "Super App",
					"Platform":    test.ExpectedPlatform,
					"StoreURL":    template.URL(test.ExpectedURL),
				}, args)
			})
		}
	})

	t.Run("NewDeprecatedAppList with an empty object", func(t *testing.T) {
		list := NewDeprecatedAppList(config.DeprecatedAppsCfg{})

		assert.False(t, list.IsDeprecated(&oauth.Client{}))
	})

	t.Run("NewDeprecatedAppList with an nil Apps", func(t *testing.T) {
		list := NewDeprecatedAppList(config.DeprecatedAppsCfg{
			Apps: nil,
		})

		assert.False(t, list.IsDeprecated(&oauth.Client{}))
	})
}
