package settings

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

const domain = "cozysettings.example.net"

var ts *httptest.Server
var testInstance *instance.Instance

func TestRegisterPassphraseWrongToken(t *testing.T) {
	res1, err := postForm("/settings/passphrase", &url.Values{
		"passphrase":    {"MyFirstPassphrase"},
		"registerToken": {"BADBEEF"},
	})
	assert.NoError(t, err)
	defer res1.Body.Close()
	assert.Equal(t, "400 Bad Request", res1.Status)

	res2, err := postForm("/settings/passphrase", &url.Values{
		"passphrase":    {"MyFirstPassphrase"},
		"registerToken": {"XYZ"},
	})
	assert.NoError(t, err)
	defer res2.Body.Close()
	assert.Equal(t, "400 Bad Request", res2.Status)
}

func TestRegisterPassphraseCorrectToken(t *testing.T) {
	res, err := postForm("/settings/passphrase", &url.Values{
		"passphrase":    {"MyFirstPassphrase"},
		"registerToken": {hex.EncodeToString(testInstance.RegisterToken)},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://onboarding.cozysettings.example.net/",
			res.Header.Get("Location"))
		cookies := res.Cookies()
		assert.Len(t, cookies, 1)
		assert.Equal(t, cookies[0].Name, sessions.SessionCookieName)
		assert.NotEmpty(t, cookies[0].Value)
	}
}

func TestUpdatePassphraseWithWrongPassphrase(t *testing.T) {
	res, err := putForm("/settings/passphrase", &url.Values{
		"new-passphrase":     {"MyPassphrase"},
		"current-passphrase": {"BADBEEF"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
}

func TestUpdatePassphraseSuccess(t *testing.T) {
	res, err := putForm("/settings/passphrase", &url.Values{
		"new-passphrase":     {"MyPassphrase"},
		"current-passphrase": {"MyFirstPassphrase"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://home.cozysettings.example.net/",
			res.Header.Get("Location"))
		cookies := res.Cookies()
		assert.Len(t, cookies, 1)
		assert.Equal(t, cookies[0].Name, sessions.SessionCookieName)
		assert.NotEmpty(t, cookies[0].Value)
	}
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	instance.Destroy(domain)
	testInstance, _ = instance.Create(&instance.Options{
		Domain: domain,
		Locale: "en",
	})

	r := echo.New()
	r.HTTPErrorHandler = errors.ErrorHandler
	Routes(r.Group("/settings", injectInstance(testInstance)))

	ts = httptest.NewServer(r)
	res := m.Run()
	ts.Close()
	instance.Destroy(domain)
	os.Exit(res)
}

func injectInstance(i *instance.Instance) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", i)
			return next(c)
		}
	}
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func postForm(u string, v *url.Values) (*http.Response, error) {
	req, _ := http.NewRequest("POST", ts.URL+u, bytes.NewBufferString(v.Encode()))
	req.Host = domain
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{CheckRedirect: noRedirect}
	return client.Do(req)
}

func putForm(u string, v *url.Values) (*http.Response, error) {
	req, _ := http.NewRequest("PUT", ts.URL+u, bytes.NewBufferString(v.Encode()))
	req.Host = domain
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{CheckRedirect: noRedirect}
	return client.Do(req)
}
