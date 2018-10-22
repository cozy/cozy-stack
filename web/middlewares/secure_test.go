package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

func TestSecureMiddlewareHSTS(t *testing.T) {
	e := echo.New()
	req, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := Secure(&SecureConfig{
		HSTSMaxAge: 3600 * time.Second,
	})(echo.NotFoundHandler)
	h(c)
	assert.Equal(t, "max-age=3600; includeSubDomains", rec.Header().Get(echo.HeaderStrictTransportSecurity))
}

func TestSecureMiddlewareCSP(t *testing.T) {
	e1 := echo.New()
	req1, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e1.NewContext(req1, rec1)
	h1 := Secure(&SecureConfig{
		CSPConnectSrc: nil,
		CSPFrameSrc:   nil,
		CSPScriptSrc:  nil,
	})(echo.NotFoundHandler)
	h1(c1)

	e2 := echo.New()
	req2, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
	rec2 := httptest.NewRecorder()
	c2 := e2.NewContext(req2, rec2)
	h2 := Secure(&SecureConfig{
		CSPConnectSrc: nil,
		CSPFrameSrc:   []CSPSource{CSPSrcAny},
		CSPScriptSrc:  []CSPSource{CSPSrcSelf},
	})(echo.NotFoundHandler)
	h2(c2)

	e3 := echo.New()
	req3, _ := http.NewRequest(echo.GET, "http://app.cozy.local/", nil)
	rec3 := httptest.NewRecorder()
	c3 := e3.NewContext(req3, rec3)
	h3 := Secure(&SecureConfig{
		CSPConnectSrc: []CSPSource{CSPSrcParent, CSPSrcSelf},
		CSPFrameSrc:   []CSPSource{CSPSrcAny},
		CSPScriptSrc:  []CSPSource{CSPSrcSiblings},
	})(echo.NotFoundHandler)
	h3(c3)

	assert.Equal(t, "", rec1.Header().Get(echo.HeaderContentSecurityPolicy))
	assert.Equal(t, "script-src 'self';frame-src *;", rec2.Header().Get(echo.HeaderContentSecurityPolicy))
	assert.Equal(t, "script-src https://*.cozy.local;frame-src *;connect-src https://cozy.local 'self';", rec3.Header().Get(echo.HeaderContentSecurityPolicy))
}

func TestAppendCSPRule(t *testing.T) {
	r := appendCSPRule("", "frame-ancestors", "new-rule")
	assert.Equal(t, "frame-ancestors new-rule;", r)

	r = appendCSPRule("frame-ancestors;", "frame-ancestors", "new-rule")
	assert.Equal(t, "frame-ancestors new-rule;", r)

	r = appendCSPRule("frame-ancestors 1 2 3 ;", "frame-ancestors", "new-rule")
	assert.Equal(t, "frame-ancestors 1 2 3 new-rule;", r)

	r = appendCSPRule("frame-ancestors 1 2 3 ;", "frame-ancestors", "new-rule", "new-rule-2")
	assert.Equal(t, "frame-ancestors 1 2 3 new-rule new-rule-2;", r)

	r = appendCSPRule("script '*'; frame-ancestors 'self';", "frame-ancestors", "new-rule")
	assert.Equal(t, "script '*'; frame-ancestors 'self' new-rule;", r)

	r = appendCSPRule("script '*'; frame-ancestors 'self'; plop plop;", "frame-ancestors", "new-rule")
	assert.Equal(t, "script '*'; frame-ancestors 'self' new-rule; plop plop;", r)

	r = appendCSPRule("script '*'; toto;", "frame-ancestors", "new-rule")
	assert.Equal(t, "script '*'; toto;frame-ancestors new-rule;", r)
}
