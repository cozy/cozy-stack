package settings

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	args, _ := json.Marshal(&echo.Map{
		"passphrase":     "MyFirstPassphrase",
		"register_token": "BADBEEF",
	})
	res1, err := http.Post(ts.URL+"/settings/passphrase", "application/json", bytes.NewReader(args))
	assert.NoError(t, err)
	defer res1.Body.Close()
	assert.Equal(t, "400 Bad Request", res1.Status)

	args, _ = json.Marshal(&echo.Map{
		"passphrase":     "MyFirstPassphrase",
		"register_token": "XYZ",
	})
	res2, err := http.Post(ts.URL+"/settings/passphrase", "application/json", bytes.NewReader(args))
	assert.NoError(t, err)
	defer res2.Body.Close()
	assert.Equal(t, "400 Bad Request", res2.Status)
}

func TestRegisterPassphraseCorrectToken(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"passphrase":     "MyFirstPassphrase",
		"register_token": hex.EncodeToString(testInstance.RegisterToken),
	})
	res, err := http.Post(ts.URL+"/settings/passphrase", "application/json", bytes.NewReader(args))
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "204 No Content", res.Status)
	cookies := res.Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, cookies[0].Name, sessions.SessionCookieName)
	assert.NotEmpty(t, cookies[0].Value)
}

func TestUpdatePassphraseWithWrongPassphrase(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"new_passphrase":     "MyPassphrase",
		"current_passphrase": "BADBEEF",
	})
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/passphrase", bytes.NewReader(args))
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
}

func TestUpdatePassphraseSuccess(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"new_passphrase":     "MyPassphrase",
		"current_passphrase": "MyFirstPassphrase",
	})
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/passphrase", bytes.NewReader(args))
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "204 No Content", res.Status)
	cookies := res.Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, cookies[0].Name, sessions.SessionCookieName)
	assert.NotEmpty(t, cookies[0].Value)
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
