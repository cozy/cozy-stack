package auth

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

const domain = "cozy.example.net"

var ts *httptest.Server
var registerToken []byte

// Stupid http.CookieJar which always returns all cookies.
// TODO use net/cookiejar when we have a proper dev env with hosts
type testJar struct {
	cookies []*http.Cookie
}

func (j *testJar) Cookies(u *url.URL) (cookies []*http.Cookie) {
	return j.cookies
}

func (j *testJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.cookies = append(j.cookies, cookies...)
}

var jar testJar
var client = &http.Client{
	CheckRedirect: noRedirect,
	Jar:           &jar,
}

func TestIsLoggedInWhenNotLoggedIn(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "who_are_you", content)
}

func TestRegisterWrongToken(t *testing.T) {
	res, err := postForm("/register", &url.Values{
		"passphrase":    {"MyPassphrase"},
		"registerToken": {"123"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
}

func TestRegisterCorrectToken(t *testing.T) {
	res, err := postForm("/register", &url.Values{
		"passphrase":    {"MyPassphrase"},
		"registerToken": {string(registerToken)},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://onboarding.cozy.example.net",
			res.Header.Get("Location"))
		cookies := res.Cookies()
		assert.Len(t, cookies, 1)
		assert.Equal(t, cookies[0].Name, SessionCookieName)
		assert.NotEmpty(t, cookies[0].Value)
	}
}

func TestIsLoggedInAfterRegister(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "logged_in", content)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	instance.Destroy(domain)
	i, _ := instance.Create(domain, "en", nil)
	registerToken = i.RegisterToken
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middlewares.SetInstance())
	Routes(router)
	router.GET("/test", func(c *gin.Context) {
		var content string
		if IsLoggedIn(c) {
			content = "logged_in"
		} else {
			content = "who_are_you"
		}
		c.String(http.StatusOK, content)
	})
	ts = httptest.NewServer(router)
	res := m.Run()
	ts.Close()
	instance.Destroy(domain)
	os.Exit(res)
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func postForm(u string, v *url.Values) (*http.Response, error) {
	req, _ := http.NewRequest("POST", ts.URL+u, bytes.NewBufferString(v.Encode()))
	req.Host = domain
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return client.Do(req)
}

func getTestURL() (string, error) {
	req, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	if err != nil {
		return "", err
	}
	content, _ := ioutil.ReadAll(res.Body)
	return string(content), nil
}
