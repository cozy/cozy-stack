package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestSecure(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

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

}
