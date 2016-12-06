package auth

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

const domain = "cozy.example.net"

var ts *httptest.Server
var registerToken []byte
var instanceURL *url.URL

// Stupid http.CookieJar which always returns all cookies.
// NOTE golang stdlib uses cookies for the URL (ie the testserver),
// not for the host (ie the instance), so we do it manually
type testJar struct {
	Jar *cookiejar.Jar
}

func (j *testJar) Cookies(u *url.URL) (cookies []*http.Cookie) {
	return j.Jar.Cookies(instanceURL)
}

func (j *testJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.Jar.SetCookies(instanceURL, cookies)
}

var jar *testJar
var client *http.Client

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

func TestLogout(t *testing.T) {
	req, _ := http.NewRequest("DELETE", ts.URL+"/auth/login", nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://cozy.example.net/auth/login",
			res.Header.Get("Location"))
		cookies := jar.Cookies(instanceURL)
		assert.Len(t, cookies, 0)
	}
}

func TestIsLoggedOutAfterLogout(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "who_are_you", content)
}

func TestShowLoginPage(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login", nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "Please enter your passphrase")
}

func TestShowLoginPageWithRedirectBadURL(t *testing.T) {
	req1, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape(" "), nil)
	req1.Host = domain
	res1, err := client.Do(req1)
	defer res1.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res1.Status)
	assert.Equal(t, "text/plain; charset=utf-8", res1.Header.Get("Content-Type"))

	req2, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("foo.bar"), nil)
	req2.Host = domain
	res2, err := client.Do(req2)
	defer res2.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res2.Status)
	assert.Equal(t, "text/plain; charset=utf-8", res2.Header.Get("Content-Type"))

	req3, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("ftp://sub."+domain+"/foo/bar"), nil)
	req3.Host = domain
	res3, err := client.Do(req3)
	defer res3.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res3.Status)
	assert.Equal(t, "text/plain; charset=utf-8", res3.Header.Get("Content-Type"))

	req4, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://"+domain+"/foo/bar"), nil)
	req4.Host = domain
	res4, err := client.Do(req4)
	defer res4.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res4.Status)
	assert.Equal(t, "text/plain; charset=utf-8", res4.Header.Get("Content-Type"))

	req5, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://."+domain+"/foo/bar"), nil)
	req5.Host = domain
	res5, err := client.Do(req5)
	defer res5.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res5.Status)
	assert.Equal(t, "text/plain; charset=utf-8", res5.Header.Get("Content-Type"))
}

func TestShowLoginPageWithRedirectXSS(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://sub."+domain+"/<script>alert('foo')</script>"), nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.NotContains(t, string(body), "<script>")
	assert.Contains(t, string(body), "%3Cscript%3Ealert%28%27foo%27%29%3C/script%3E")
}

func TestShowLoginPageWithRedirectFragment(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://sub."+domain+"/#myfragment"), nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.NotContains(t, string(body), "myfragment")
	assert.Contains(t, string(body), `<input type="hidden" name="redirect" value="https://sub.cozy.example.net/" />`)
}

func TestShowLoginPageWithRedirectSuccess(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://sub."+domain+"/foo/bar?query=foo#myfragment"), nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.NotContains(t, string(body), "myfragment")
	assert.Contains(t, string(body), `<input type="hidden" name="redirect" value="https://sub.cozy.example.net/foo/bar?query=foo" />`)
}

func TestLoginWithBadPassphrase(t *testing.T) {
	res, err := postForm("/auth/login", &url.Values{
		"passphrase": {"Nope"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "401 Unauthorized", res.Status)
}

func TestLoginWithGoodPassphrase(t *testing.T) {
	res, err := postForm("/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://home.cozy.example.net",
			res.Header.Get("Location"))
		cookies := res.Cookies()
		assert.Len(t, cookies, 1)
		assert.Equal(t, cookies[0].Name, SessionCookieName)
		assert.NotEmpty(t, cookies[0].Value)
	}
}

func TestIsLoggedInAfterLogin(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "logged_in", content)
}

func TestMain(m *testing.M) {
	instanceURL, _ = url.Parse("https://" + domain + "/")
	j, _ := cookiejar.New(nil)
	jar = &testJar{
		Jar: j,
	}
	client = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}
	config.UseTestFile()
	instance.Destroy(domain)
	i, _ := instance.Create(domain, "en", nil)
	registerToken = i.RegisterToken

	r := echo.New()
	r.HTTPErrorHandler = web.ErrorHandler
	Routes(r.Group("", middlewares.NeedInstance))

	r.GET("/test", func(c echo.Context) error {
		var content string
		if IsLoggedIn(c) {
			content = "logged_in"
		} else {
			content = "who_are_you"
		}
		return c.String(http.StatusOK, content)
	}, middlewares.NeedInstance)

	handler, err := web.Create(&web.Config{
		Router: r,
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ts = httptest.NewServer(handler)
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
