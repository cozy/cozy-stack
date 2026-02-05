package middlewares

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecure(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	config.GetConfig().Assets = "../../assets"
	setup := testutils.NewSetup(t, t.Name())

	setup.SetupSwiftTest()
	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()), "Could not init dynamic FS")

	t.Run("SecureMiddlewareHSTS", func(t *testing.T) {
		e := echo.New()
		req, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		h := Secure(&SecureConfig{
			HSTSMaxAge: 3600 * time.Second,
		})(echo.NotFoundHandler)
		_ = h(c)
		assert.Equal(t, "max-age=3600; includeSubDomains", rec.Header().Get(echo.HeaderStrictTransportSecurity))
	})

	t.Run("SecureMiddlewareCSP", func(t *testing.T) {
		e1 := echo.New()
		req1, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
		rec1 := httptest.NewRecorder()
		c1 := e1.NewContext(req1, rec1)
		h1 := Secure(&SecureConfig{
			CSPConnectSrc: nil,
			CSPFrameSrc:   nil,
			CSPScriptSrc:  nil,
		})(echo.NotFoundHandler)
		_ = h1(c1)

		e2 := echo.New()
		req2, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
		rec2 := httptest.NewRecorder()
		c2 := e2.NewContext(req2, rec2)
		h2 := Secure(&SecureConfig{
			CSPConnectSrc: nil,
			CSPFrameSrc:   []CSPSource{CSPSrcAny},
			CSPScriptSrc:  []CSPSource{CSPSrcSelf},
		})(echo.NotFoundHandler)
		_ = h2(c2)

		e3 := echo.New()
		req3, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
		rec3 := httptest.NewRecorder()
		c3 := e3.NewContext(req3, rec3)
		h3 := Secure(&SecureConfig{
			CSPConnectSrc: []CSPSource{CSPSrcParent, CSPSrcSelf},
			CSPFrameSrc:   []CSPSource{CSPSrcAny},
			CSPScriptSrc:  []CSPSource{CSPSrcSiblings},
		})(echo.NotFoundHandler)
		_ = h3(c3)

		e4 := echo.New()
		req4, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
		rec4 := httptest.NewRecorder()
		c4 := e4.NewContext(req4, rec4)
		c4.Set("instance", &instance.Instance{ContextName: "test"})
		h4 := Secure(&SecureConfig{
			CSPConnectSrc: nil,
			CSPFrameSrc:   []CSPSource{CSPSrcAny},
			CSPScriptSrc:  []CSPSource{CSPSrcSelf},
			CSPPerContext: map[string]map[string]string{
				"test": {
					"frame":   "https://example.net",
					"connect": "https://example.com",
				},
			},
		})(echo.NotFoundHandler)
		_ = h4(c4)

		assert.Equal(t, "", rec1.Header().Get(echo.HeaderContentSecurityPolicy))
		assert.Equal(t, "script-src 'self';frame-src *;", rec2.Header().Get(echo.HeaderContentSecurityPolicy))
		assert.Equal(t, "script-src https://*.cozy.local;frame-src *;connect-src https://cozy.local 'self';", rec3.Header().Get(echo.HeaderContentSecurityPolicy))
		assert.Equal(t, "script-src 'self';frame-src * https://example.net;connect-src https://example.com;", rec4.Header().Get(echo.HeaderContentSecurityPolicy))
	})

	t.Run("AppendCSPRule", func(t *testing.T) {
		r := appendCSPRule("", "frame-ancestors", "new-rule")
		assert.Equal(t, "frame-ancestors new-rule;", r)

		r = appendCSPRule("frame-ancestors;", "frame-ancestors", "new-rule")
		assert.Equal(t, "frame-ancestors new-rule;", r)

		r = appendCSPRule("frame-ancestors 1 2 3 ;", "frame-ancestors", "new-rule")
		assert.Equal(t, "frame-ancestors 1 2 3 new-rule;", r)

		r = appendCSPRule("frame-ancestors 1 2 3 ;", "frame-ancestors", "new-rule", "new-rule-2")
		assert.Equal(t, "frame-ancestors 1 2 3 new-rule new-rule-2;", r)

		r = appendCSPRule("frame-ancestors 'none';", "frame-ancestors", "new-rule")
		assert.Equal(t, "frame-ancestors new-rule;", r)

		r = appendCSPRule("script '*'; frame-ancestors 'self';", "frame-ancestors", "new-rule")
		assert.Equal(t, "script '*'; frame-ancestors 'self' new-rule;", r)

		r = appendCSPRule("script '*'; frame-ancestors 'self'; plop plop;", "frame-ancestors", "new-rule")
		assert.Equal(t, "script '*'; frame-ancestors 'self' new-rule; plop plop;", r)

		r = appendCSPRule("script '*'; toto;", "frame-ancestors", "new-rule")
		assert.Equal(t, "script '*'; toto;frame-ancestors new-rule;", r)
	})

	t.Run("SecureMiddlewareCSPWithOrgDomain", func(t *testing.T) {
		e := echo.New()
		req, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		inst := &instance.Instance{
			Domain:    "cozy.local",
			OrgDomain: "example.com",
		}
		c.Set("instance", inst)
		h := Secure(&SecureConfig{
			CSPDefaultSrc:     []CSPSource{CSPSrcSelf},
			CSPScriptSrc:      []CSPSource{CSPSrcSelf},
			CSPFrameSrc:       []CSPSource{CSPSrcSelf},
			CSPConnectSrc:     []CSPSource{CSPSrcSelf},
			CSPFontSrc:        []CSPSource{CSPSrcSelf},
			CSPImgSrc:         []CSPSource{CSPSrcSelf},
			CSPManifestSrc:    []CSPSource{CSPSrcSelf},
			CSPMediaSrc:       []CSPSource{CSPSrcSelf},
			CSPObjectSrc:      []CSPSource{CSPSrcSelf},
			CSPStyleSrc:       []CSPSource{CSPSrcSelf},
			CSPWorkerSrc:      []CSPSource{CSPSrcSelf},
			CSPFrameAncestors: []CSPSource{CSPSrcSelf},
			CSPBaseURI:        []CSPSource{CSPSrcSelf},
			CSPFormAction:     []CSPSource{CSPSrcSelf},
		})(echo.NotFoundHandler)
		_ = h(c)

		csp := rec.Header().Get(echo.HeaderContentSecurityPolicy)

		// Verify that matrix.example.com appears only once (in frame-src)
		count := strings.Count(csp, "matrix.example.com")
		assert.Equal(t, 1, count,
			"matrix.example.com should appear exactly once (in frame-src), but found %d times. CSP: %s",
			count, csp)

		// Verify that frame-src contains matrix.example.com
		frameSrcIndex := strings.Index(csp, "frame-src ")
		assert.NotEqual(t, -1, frameSrcIndex,
			"frame-src should be present in CSP. Full CSP: %s", csp)

		frameSrcEnd := strings.Index(csp[frameSrcIndex:], ";")
		assert.NotEqual(t, -1, frameSrcEnd,
			"frame-src should end with semicolon")

		frameSrcContent := csp[frameSrcIndex : frameSrcIndex+frameSrcEnd]
		assert.Contains(t, frameSrcContent, "matrix.example.com",
			"frame-src should contain matrix.example.com. Found: %s", frameSrcContent)

		// Verify that other directives do NOT contain matrix.example.com
		otherDirectives := []string{
			"default-src",
			"script-src",
			"connect-src",
			"font-src",
			"img-src",
			"manifest-src",
			"media-src",
			"object-src",
			"style-src",
			"worker-src",
			"frame-ancestors",
			"base-uri",
			"form-action",
		}

		for _, directivePattern := range otherDirectives {
			directiveIndex := strings.Index(csp, directivePattern+" ")
			if directiveIndex != -1 {
				directiveEnd := strings.Index(csp[directiveIndex:], ";")
				if directiveEnd != -1 {
					directiveContent := csp[directiveIndex : directiveIndex+directiveEnd]
					assert.NotContains(t, directiveContent, "matrix.example.com",
						"Directive %s should NOT contain matrix.example.com. Found: %s", directivePattern, directiveContent)
				}
			}
		}
	})

	t.Run("SecureMiddlewareCSPWithOrgID", func(t *testing.T) {
		e := echo.New()
		req, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		inst := &instance.Instance{
			Domain: "alice.cozy.example.com",
			OrgID:  "myorg123",
		}
		c.Set("instance", inst)
		h := Secure(&SecureConfig{
			CSPDefaultSrc:     []CSPSource{CSPSrcSelf},
			CSPScriptSrc:      []CSPSource{CSPSrcSelf},
			CSPFrameSrc:       []CSPSource{CSPSrcSelf},
			CSPConnectSrc:     []CSPSource{CSPSrcSelf},
			CSPFontSrc:        []CSPSource{CSPSrcSelf},
			CSPImgSrc:         []CSPSource{CSPSrcSelf},
			CSPManifestSrc:    []CSPSource{CSPSrcSelf},
			CSPMediaSrc:       []CSPSource{CSPSrcSelf},
			CSPObjectSrc:      []CSPSource{CSPSrcSelf},
			CSPStyleSrc:       []CSPSource{CSPSrcSelf},
			CSPWorkerSrc:      []CSPSource{CSPSrcSelf},
			CSPFrameAncestors: []CSPSource{CSPSrcSelf},
			CSPBaseURI:        []CSPSource{CSPSrcSelf},
			CSPFormAction:     []CSPSource{CSPSrcSelf},
		})(echo.NotFoundHandler)
		_ = h(c)

		csp := rec.Header().Get(echo.HeaderContentSecurityPolicy)

		// Verify that api-login-myorg123.cozy.example.com appears only once (in connect-src)
		expectedDomain := "api-login-myorg123.cozy.example.com"
		count := strings.Count(csp, expectedDomain)
		assert.Equal(t, 1, count,
			"%s should appear exactly once (in connect-src), but found %d times. CSP: %s",
			expectedDomain, count, csp)

		// Verify that connect-src contains the api-login domain
		connectSrcIndex := strings.Index(csp, "connect-src ")
		assert.NotEqual(t, -1, connectSrcIndex,
			"connect-src should be present in CSP. Full CSP: %s", csp)

		connectSrcEnd := strings.Index(csp[connectSrcIndex:], ";")
		assert.NotEqual(t, -1, connectSrcEnd,
			"connect-src should end with semicolon")

		connectSrcContent := csp[connectSrcIndex : connectSrcIndex+connectSrcEnd]
		assert.Contains(t, connectSrcContent, expectedDomain,
			"connect-src should contain %s. Found: %s", expectedDomain, connectSrcContent)

		// Verify that other directives do NOT contain the api-login domain
		otherDirectives := []string{
			"default-src",
			"script-src",
			"frame-src",
			"font-src",
			"img-src",
			"manifest-src",
			"media-src",
			"object-src",
			"style-src",
			"worker-src",
			"frame-ancestors",
			"base-uri",
			"form-action",
		}

		for _, directivePattern := range otherDirectives {
			directiveIndex := strings.Index(csp, directivePattern+" ")
			if directiveIndex != -1 {
				directiveEnd := strings.Index(csp[directiveIndex:], ";")
				if directiveEnd != -1 {
					directiveContent := csp[directiveIndex : directiveIndex+directiveEnd]
					assert.NotContains(t, directiveContent, expectedDomain,
						"Directive %s should NOT contain %s. Found: %s", directivePattern, expectedDomain, directiveContent)
				}
			}
		}
	})
}
