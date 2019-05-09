// spec package is introduced to avoid circular dependencies since this
// particular test requires to depend on routing directly to expose the API and
// the APP server.
package auth_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

const domain = "cozy.example.net"

var JWTSecret = []byte("foobar")

var ts *httptest.Server
var testInstance *instance.Instance

var jar http.CookieJar
var client *http.Client
var clientID string
var clientSecret string
var registrationToken string
var sharingClientID string
var altClientID string
var altRegistrationToken string
var csrfToken string
var code string
var refreshToken string
var linkedClientID string
var linkedClientSecret string
var linkedCode string

func getSessionID(cookies []*http.Cookie) string {
	for _, c := range cookies {
		if c.Name == "cozysessid" {
			b, err := base64.RawURLEncoding.DecodeString(c.Value)
			if err != nil {
				return ""
			}
			return string(b[8 : 8+32])
		}
	}
	return ""
}

func TestInstanceBlocked(t *testing.T) {
	// Block the instance
	testInstance.Blocked = true
	_ = couchdb.UpdateDoc(couchdb.GlobalDB, testInstance)

	req, _ := http.NewRequest("GET", ts.URL+"/auth/login", nil)
	req.Host = testInstance.Domain

	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, res.StatusCode)

	// Trying with a Accept: text/html header to simulate a browser
	req.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	res2, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, res2.StatusCode)
	body, err := ioutil.ReadAll(res2.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(body), "<title>Cozy</title>")
	assert.Contains(t, string(body), "<h1>Your Cozy has been blocked</h1>")

	// Unblock the instance
	testInstance.Blocked = false
	_ = couchdb.UpdateDoc(couchdb.GlobalDB, testInstance)
}

func TestIsLoggedInWhenNotLoggedIn(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "who_are_you", content)
}

func TestHomeWhenNotLoggedIn(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://cozy.example.net/auth/login",
			res.Header.Get("Location"))
	}
}

func TestHomeWhenNotLoggedInWithJWT(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/?jwt=foobar", nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://cozy.example.net/auth/login?jwt=foobar",
			res.Header.Get("Location"))
	}
}

func TestShowLoginPage(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login", nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "Log in")
}

func TestShowLoginPageWithRedirectBadURL(t *testing.T) {
	req1, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape(" "), nil)
	req1.Host = domain
	res1, err := client.Do(req1)
	assert.NoError(t, err)
	defer res1.Body.Close()
	assert.Equal(t, "400 Bad Request", res1.Status)
	assert.Equal(t, "text/plain; charset=UTF-8", res1.Header.Get("Content-Type"))

	req2, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("foo.bar"), nil)
	req2.Host = domain
	res2, err := client.Do(req2)
	assert.NoError(t, err)
	defer res2.Body.Close()
	assert.Equal(t, "400 Bad Request", res2.Status)
	assert.Equal(t, "text/plain; charset=UTF-8", res2.Header.Get("Content-Type"))

	req3, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("ftp://sub."+domain+"/foo"), nil)
	req3.Host = domain
	res3, err := client.Do(req3)
	assert.NoError(t, err)
	defer res3.Body.Close()
	assert.Equal(t, "400 Bad Request", res3.Status)
	assert.Equal(t, "text/plain; charset=UTF-8", res3.Header.Get("Content-Type"))
}

func TestShowLoginPageWithRedirectXSS(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://sub."+domain+"/<script>alert('foo')</script>"), nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.NotContains(t, string(body), "<script>")
	assert.Contains(t, string(body), "%3Cscript%3Ealert%28%27foo%27%29%3C/script%3E")
}

func TestShowLoginPageWithRedirectFragment(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://"+domain+"/auth/authorize#myfragment"), nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.NotContains(t, string(body), "myfragment")
	assert.Contains(t, string(body), `<input id="redirect" type="hidden" name="redirect" value="https://cozy.example.net/auth/authorize#=" />`)
}

func TestShowLoginPageWithRedirectSuccess(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://sub."+domain+"/foo/bar?query=foo#myfragment"), nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), `<input id="redirect" type="hidden" name="redirect" value="https://sub.cozy.example.net/foo/bar?query=foo#myfragment" />`)
}

func getLoginCSRFToken(c *http.Client, t *testing.T) string {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login", nil)
	req.Host = domain
	res, err := c.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	return res.Cookies()[0].Value
}

func TestLoginWithoutCSRFToken(t *testing.T) {
	res, err := postForm("/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
}

func TestLoginWithBadPassphrase(t *testing.T) {
	res, err := postForm("/auth/login", &url.Values{
		"passphrase": {"Nope"},
		"csrf_token": {getLoginCSRFToken(client, t)},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "401 Unauthorized", res.Status)
}

func TestLoginWithGoodPassphrase(t *testing.T) {
	token := getLoginCSRFToken(client, t)
	res, err := postForm("/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
		"csrf_token": {token},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://home.cozy.example.net/",
			res.Header.Get("Location"))
		cookies := res.Cookies()
		assert.Len(t, cookies, 2)
		assert.Equal(t, cookies[0].Name, "_csrf")
		assert.Equal(t, cookies[0].Value, token)
		assert.Equal(t, cookies[1].Name, session.SessionCookieName)
		assert.NotEmpty(t, cookies[1].Value)

		var results []*session.LoginEntry
		err = couchdb.GetAllDocs(
			testInstance,
			consts.SessionsLogins,
			&couchdb.AllDocsRequest{Limit: 100},
			&results,
		)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, "Go-http-client/1.1", results[0].UA)
		assert.True(t, strings.HasPrefix(results[0].IP, "127.0.0.1:"))
		assert.False(t, results[0].CreatedAt.IsZero())
	}
}

func TestLoginWithRedirect(t *testing.T) {
	res1, err := postForm("/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
		"redirect":   {"foo.bar"},
		"csrf_token": {getLoginCSRFToken(client, t)},
	})
	assert.NoError(t, err)
	defer res1.Body.Close()
	assert.Equal(t, "400 Bad Request", res1.Status)

	res2, err := postForm("/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
		"redirect":   {"https://sub." + domain + "/#myfragment"},
		"csrf_token": {getLoginCSRFToken(client, t)},
	})
	assert.NoError(t, err)
	defer res2.Body.Close()
	if assert.Equal(t, "303 See Other", res2.Status) {
		assert.Equal(t, "https://sub.cozy.example.net/#myfragment",
			res2.Header.Get("Location"))
	}
}

func TestDelegatedJWTLoginWithRedirect(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, session.ExternalClaims{
		StandardClaims: jwt.StandardClaims{
			Subject:   "sruti",
			IssuedAt:  time.Now().Unix(),
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		},
		Name:  domain,
		Email: "sruti@external.notmycozy.net",
		Code:  "student",
	})
	signed, err := token.SignedString(JWTSecret)
	assert.NoError(t, err)
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login?jwt="+signed, nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusSeeOther, res.StatusCode)
}

func TestLoginWithSessionCode(t *testing.T) {
	cfg := config.GetConfig()
	cfg.Subdomains = config.FlatSubdomains
	defer func() { cfg.Subdomains = config.NestedSubdomains }()

	// Logout
	req, _ := http.NewRequest("DELETE", ts.URL+"/auth/login", nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	res.Body.Close()

	// Login
	res, err = postForm("/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
		"redirect":   {"https://cozy-app.example.net/private"},
		"csrf_token": {getLoginCSRFToken(client, t)},
	})
	assert.NoError(t, err)
	res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		location, err2 := url.Parse(res.Header.Get("Location"))
		assert.NoError(t, err2)
		assert.Equal(t, "cozy-app.example.net", location.Host)
		assert.Equal(t, "/private", location.Path)
		code2 := location.Query().Get("code")
		assert.Len(t, code2, 22)
	}

	// Already logged-in (GET)
	req, err = http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://cozy-app.example.net/private"), nil)
	assert.NoError(t, err)
	req.Host = domain
	res, err = client.Do(req)
	assert.NoError(t, err)
	res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		location, err2 := url.Parse(res.Header.Get("Location"))
		assert.NoError(t, err2)
		assert.Equal(t, "cozy-app.example.net", location.Host)
		assert.Equal(t, "/private", location.Path)
		code2 := location.Query().Get("code")
		assert.Len(t, code2, 22)
	}

	// Already logged-in (POST)
	res, err = postForm("/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
		"redirect":   {"https://cozy-app.example.net/private"},
		"csrf_token": {getLoginCSRFToken(client, t)},
	})
	assert.NoError(t, err)
	res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		location, err2 := url.Parse(res.Header.Get("Location"))
		assert.NoError(t, err2)
		assert.Equal(t, "cozy-app.example.net", location.Host)
		assert.Equal(t, "/private", location.Path)
		code2 := location.Query().Get("code")
		assert.Len(t, code2, 22)
	}
}

func TestIsLoggedInAfterLogin(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "logged_in", content)
}

func TestHomeWhenLoggedIn(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://home.cozy.example.net/",
			res.Header.Get("Location"))
	}
}

func TestRegisterClientNotJSON(t *testing.T) {
	res, err := postForm("/auth/register", &url.Values{"foo": {"bar"}})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	res.Body.Close()
}

func TestRegisterClientNoRedirectURI(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"client_name": "cozy-test",
		"software_id": "github.com/cozy/cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "invalid_redirect_uri", body["error"])
	assert.Equal(t, "redirect_uris is mandatory", body["error_description"])
}

func TestRegisterClientInvalidRedirectURI(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"redirect_uris": []string{"http://example.org/foo#bar"},
		"client_name":   "cozy-test",
		"software_id":   "github.com/cozy/cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "invalid_redirect_uri", body["error"])
	assert.Equal(t, "http://example.org/foo#bar is invalid", body["error_description"])
}

func TestRegisterClientNoClientName(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"redirect_uris": []string{"https://example.org/oauth/callback"},
		"software_id":   "github.com/cozy/cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "invalid_client_metadata", body["error"])
	assert.Equal(t, "client_name is mandatory", body["error_description"])
}

func TestRegisterClientNoSoftwareID(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"redirect_uris": []string{"https://example.org/oauth/callback"},
		"client_name":   "cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "invalid_client_metadata", body["error"])
	assert.Equal(t, "software_id is mandatory", body["error_description"])
}

func TestRegisterClientSuccessWithJustMandatoryFields(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"redirect_uris": []string{"https://example.org/oauth/callback"},
		"client_name":   "cozy-test",
		"software_id":   "github.com/cozy/cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "201 Created", res.Status)
	var client oauth.Client
	err = json.NewDecoder(res.Body).Decode(&client)
	assert.NoError(t, err)
	assert.NotEqual(t, client.ClientID, "")
	assert.NotEqual(t, client.ClientID, "ignored")
	assert.NotEqual(t, client.ClientSecret, "")
	assert.NotEqual(t, client.ClientSecret, "ignored")
	assert.NotEqual(t, client.RegistrationToken, "")
	assert.NotEqual(t, client.RegistrationToken, "ignored")
	assert.Equal(t, client.SecretExpiresAt, 0)
	assert.Equal(t, client.RedirectURIs, []string{"https://example.org/oauth/callback"})
	assert.Equal(t, client.GrantTypes, []string{"authorization_code", "refresh_token"})
	assert.Equal(t, client.ResponseTypes, []string{"code"})
	assert.Equal(t, client.ClientName, "cozy-test")
	assert.Equal(t, client.SoftwareID, "github.com/cozy/cozy-test")
	clientID = client.ClientID
	clientSecret = client.ClientSecret
	registrationToken = client.RegistrationToken
}

func TestRegisterClientSuccessWithAllFields(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"_id":                       "ignored",
		"_rev":                      "ignored",
		"client_id":                 "ignored",
		"client_secret":             "ignored",
		"client_secret_expires_at":  42,
		"registration_access_token": "ignored",
		"redirect_uris":             []string{"https://example.org/oauth/callback"},
		"grant_types":               []string{"ignored"},
		"response_types":            []string{"ignored"},
		"client_name":               "new-cozy-test",
		"client_kind":               "test",
		"client_uri":                "https://github.com/cozy/cozy-test",
		"logo_uri":                  "https://raw.github.com/cozy/cozy-setup/gh-pages/assets/images/happycloud.png",
		"policy_uri":                "https://github/com/cozy/cozy-test/master/policy.md",
		"software_id":               "github.com/cozy/cozy-test",
		"software_version":          "v0.1.2",
	})
	assert.NoError(t, err)
	assert.Equal(t, "201 Created", res.Status)
	var client oauth.Client
	err = json.NewDecoder(res.Body).Decode(&client)
	assert.NoError(t, err)
	assert.Equal(t, client.CouchID, "")
	assert.Equal(t, client.CouchRev, "")
	assert.NotEqual(t, client.ClientID, "")
	assert.NotEqual(t, client.ClientID, "ignored")
	assert.NotEqual(t, client.ClientID, clientID)
	assert.NotEqual(t, client.ClientSecret, "")
	assert.NotEqual(t, client.ClientSecret, "ignored")
	assert.NotEqual(t, client.RegistrationToken, "")
	assert.NotEqual(t, client.RegistrationToken, "ignored")
	assert.Equal(t, client.SecretExpiresAt, 0)
	assert.Equal(t, client.RedirectURIs, []string{"https://example.org/oauth/callback"})
	assert.Equal(t, client.GrantTypes, []string{"authorization_code", "refresh_token"})
	assert.Equal(t, client.ResponseTypes, []string{"code"})
	assert.Equal(t, client.ClientName, "new-cozy-test")
	assert.Equal(t, client.ClientKind, "test")
	assert.Equal(t, client.ClientURI, "https://github.com/cozy/cozy-test")
	assert.Equal(t, client.LogoURI, "https://raw.github.com/cozy/cozy-setup/gh-pages/assets/images/happycloud.png")
	assert.Equal(t, client.PolicyURI, "https://github/com/cozy/cozy-test/master/policy.md")
	assert.Equal(t, client.SoftwareID, "github.com/cozy/cozy-test")
	assert.Equal(t, client.SoftwareVersion, "v0.1.2")
	altClientID = client.ClientID
	altRegistrationToken = client.RegistrationToken
}

func TestRegisterSharingClientSuccess(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"redirect_uris": []string{"https://cozy.example.org/sharings/answer"},
		"client_name":   "John",
		"software_id":   "github.com/cozy/cozy-stack",
		"client_kind":   "sharing",
		"client_uri":    "https://cozy.example.org",
	})
	assert.NoError(t, err)
	assert.Equal(t, "201 Created", res.Status)
	var client oauth.Client
	err = json.NewDecoder(res.Body).Decode(&client)
	assert.NoError(t, err)
	assert.NotEqual(t, client.ClientID, "")
	assert.NotEqual(t, client.ClientID, "ignored")
	assert.NotEqual(t, client.ClientSecret, "")
	assert.NotEqual(t, client.ClientSecret, "ignored")
	assert.NotEqual(t, client.RegistrationToken, "")
	assert.NotEqual(t, client.RegistrationToken, "ignored")
	assert.Equal(t, client.SecretExpiresAt, 0)
	assert.Equal(t, client.RedirectURIs, []string{"https://cozy.example.org/sharings/answer"})
	assert.Equal(t, client.ClientName, "John")
	assert.Equal(t, client.SoftwareID, "github.com/cozy/cozy-stack")
	sharingClientID = client.ClientID
}

func TestDeleteClientInvalidClientID(t *testing.T) {
	req, _ := http.NewRequest("DELETE", ts.URL+"/auth/register/123456789", nil)
	req.Host = domain
	req.Header.Add("Authorization", "Bearer "+altRegistrationToken)
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status)
}

func TestDeleteClientNoToken(t *testing.T) {
	req, _ := http.NewRequest("DELETE", ts.URL+"/auth/register/"+altClientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
}

func TestDeleteClientSuccess(t *testing.T) {
	req, _ := http.NewRequest("DELETE", ts.URL+"/auth/register/"+altClientID, nil)
	req.Host = domain
	req.Header.Add("Authorization", "Bearer "+altRegistrationToken)
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, "204 No Content", res.Status)
}

func TestReadClientInvalidToken(t *testing.T) {
	res, err := getJSON("/auth/register/"+clientID, altRegistrationToken)
	assert.NoError(t, err)
	assert.Equal(t, "401 Unauthorized", res.Status)
}

func TestReadClientInvalidClientID(t *testing.T) {
	res, err := getJSON("/auth/register/"+altClientID, registrationToken)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status)
}

func TestReadClientSuccess(t *testing.T) {
	res, err := getJSON("/auth/register/"+clientID, registrationToken)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	var client oauth.Client
	err = json.NewDecoder(res.Body).Decode(&client)
	assert.NoError(t, err)
	assert.Equal(t, client.ClientID, clientID)
	assert.Equal(t, client.ClientSecret, clientSecret)
	assert.Equal(t, client.SecretExpiresAt, 0)
	assert.Equal(t, client.RegistrationToken, "")
	assert.Equal(t, client.RedirectURIs, []string{"https://example.org/oauth/callback"})
	assert.Equal(t, client.GrantTypes, []string{"authorization_code", "refresh_token"})
	assert.Equal(t, client.ResponseTypes, []string{"code"})
	assert.Equal(t, client.ClientName, "cozy-test")
	assert.Equal(t, client.SoftwareID, "github.com/cozy/cozy-test")
}

func TestUpdateClientDeletedClientID(t *testing.T) {
	res, err := putJSON("/auth/register/"+altClientID, registrationToken, echo.Map{
		"client_id": altClientID,
	})
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status)
}

func TestUpdateClientInvalidClientID(t *testing.T) {
	res, err := putJSON("/auth/register/"+clientID, registrationToken, echo.Map{
		"client_id": "123456789",
	})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "invalid_client_id", body["error"])
	assert.Equal(t, "client_id is mandatory", body["error_description"])
}

func TestUpdateClientNoRedirectURI(t *testing.T) {
	res, err := putJSON("/auth/register/"+clientID, registrationToken, echo.Map{
		"client_id":   clientID,
		"client_name": "cozy-test",
		"software_id": "github.com/cozy/cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "invalid_redirect_uri", body["error"])
	assert.Equal(t, "redirect_uris is mandatory", body["error_description"])
}

func TestUpdateClientSuccess(t *testing.T) {
	res, err := putJSON("/auth/register/"+clientID, registrationToken, echo.Map{
		"client_id":        clientID,
		"redirect_uris":    []string{"https://example.org/oauth/callback"},
		"client_name":      "cozy-test",
		"software_id":      "github.com/cozy/cozy-test",
		"software_version": "v0.1.3",
	})
	assert.NoError(t, err)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	var client oauth.Client
	err = json.NewDecoder(res.Body).Decode(&client)
	assert.NoError(t, err)
	assert.Equal(t, client.ClientID, clientID)
	assert.Equal(t, client.ClientSecret, clientSecret)
	assert.Equal(t, client.SecretExpiresAt, 0)
	assert.Equal(t, client.RegistrationToken, "")
	assert.Equal(t, client.RedirectURIs, []string{"https://example.org/oauth/callback"})
	assert.Equal(t, client.GrantTypes, []string{"authorization_code", "refresh_token"})
	assert.Equal(t, client.ResponseTypes, []string{"code"})
	assert.Equal(t, client.ClientName, "cozy-test")
	assert.Equal(t, client.SoftwareID, "github.com/cozy/cozy-test")
	assert.Equal(t, client.SoftwareVersion, "v0.1.3")
}

func TestUpdateClientSecret(t *testing.T) {
	res, err := putJSON("/auth/register/"+clientID, registrationToken, echo.Map{
		"client_id":        clientID,
		"client_secret":    clientSecret,
		"redirect_uris":    []string{"https://example.org/oauth/callback"},
		"client_name":      "cozy-test",
		"software_id":      "github.com/cozy/cozy-test",
		"software_version": "v0.1.4",
	})
	assert.NoError(t, err)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	var client oauth.Client
	err = json.NewDecoder(res.Body).Decode(&client)
	assert.NoError(t, err)
	assert.Equal(t, client.ClientID, clientID)
	assert.NotEqual(t, client.ClientSecret, "")
	assert.NotEqual(t, client.ClientSecret, clientSecret)
	assert.Equal(t, client.SecretExpiresAt, 0)
	assert.Equal(t, client.RegistrationToken, "")
	assert.Equal(t, client.RedirectURIs, []string{"https://example.org/oauth/callback"})
	assert.Equal(t, client.GrantTypes, []string{"authorization_code", "refresh_token"})
	assert.Equal(t, client.ResponseTypes, []string{"code"})
	assert.Equal(t, client.ClientName, "cozy-test")
	assert.Equal(t, client.SoftwareID, "github.com/cozy/cozy-test")
	assert.Equal(t, client.SoftwareVersion, "v0.1.4")
	clientSecret = client.ClientSecret
}

func TestAuthorizeFormRedirectsWhenNotLoggedIn(t *testing.T) {
	anonymousClient := &http.Client{CheckRedirect: noRedirect}
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := anonymousClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "303 See Other", res.Status)
}

func TestAuthorizeFormBadResponseType(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=token&state=123456&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "Invalid response type")
}

func TestAuthorizeFormNoState(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The state parameter is mandatory")
}

func TestAuthorizeFormNoClientId(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u, nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The client_id parameter is mandatory")
}

func TestAuthorizeFormNoRedirectURI(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The redirect_uri parameter is mandatory")
}

func TestAuthorizeFormNoScope(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The scope parameter is mandatory")
}

func TestAuthorizeFormInvalidClient(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u+"&client_id=f00", nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The client must be registered")
}

func TestAuthorizeFormInvalidRedirectURI(t *testing.T) {
	u := url.QueryEscape("https://evil.com/")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The redirect_uri parameter doesn&#39;t match the registered ones")
}

func TestAuthorizeFormSuccess(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "would like permission to access your Cozy")
	re := regexp.MustCompile(`<input type="hidden" name="csrf_token" value="(\w+)"`)
	matches := re.FindStringSubmatch(string(body))
	if assert.Len(t, matches, 2) {
		csrfToken = matches[1]
	}
}

func TestAuthorizeFormClientMobileApp(t *testing.T) {
	var oauthClient oauth.Client

	u := "https://example.org/oauth/callback"

	oauthClient.RedirectURIs = []string{u}
	oauthClient.ClientName = "cozy-test-2"
	oauthClient.SoftwareID = "registry://drive"
	oauthClient.Create(testInstance)

	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&redirect_uri="+u+"&client_id="+oauthClient.ClientID, nil)
	req.Host = testInstance.Domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	content, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(content), "io.cozy.files")
	defer res.Body.Close()
}

func TestAuthorizeWhenNotLoggedIn(t *testing.T) {
	anonymousClient := &http.Client{CheckRedirect: noRedirect}
	v := &url.Values{
		"state":        {"123456"},
		"client_id":    {clientID},
		"redirect_uri": {"https://example.org/oauth/callback"},
		"scope":        {"files:read"},
		"csrf_token":   {csrfToken},
	}
	req, _ := http.NewRequest("POST", ts.URL+"/auth/authorize", bytes.NewBufferString(v.Encode()))
	req.Host = domain
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	res, err := anonymousClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "403 Forbidden", res.Status)
}

func TestAuthorizeWithInvalidCSRFToken(t *testing.T) {
	res, err := postForm("/auth/authorize", &url.Values{
		"state":        {"123456"},
		"client_id":    {clientID},
		"redirect_uri": {"https://example.org/oauth/callback"},
		"scope":        {"files:read"},
		"csrf_token":   {"azertyuiop"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "403 Forbidden", res.Status)
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "invalid csrf token")
}

func TestAuthorizeWithNoState(t *testing.T) {
	res, err := postForm("/auth/authorize", &url.Values{
		"client_id":    {clientID},
		"redirect_uri": {"https://example.org/oauth/callback"},
		"scope":        {"files:read"},
		"csrf_token":   {csrfToken},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The state parameter is mandatory")
}

func TestAuthorizeWithNoClientID(t *testing.T) {
	res, err := postForm("/auth/authorize", &url.Values{
		"state":        {"123456"},
		"redirect_uri": {"https://example.org/oauth/callback"},
		"scope":        {"files:read"},
		"csrf_token":   {csrfToken},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The client_id parameter is mandatory")
}

func TestAuthorizeWithInvalidClientID(t *testing.T) {
	res, err := postForm("/auth/authorize", &url.Values{
		"state":         {"123456"},
		"client_id":     {"987"},
		"redirect_uri":  {"https://example.org/oauth/callback"},
		"scope":         {"files:read"},
		"csrf_token":    {csrfToken},
		"response_type": {"code"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The client must be registered")
}

func TestAuthorizeWithNoRedirectURI(t *testing.T) {
	res, err := postForm("/auth/authorize", &url.Values{
		"state":         {"123456"},
		"client_id":     {clientID},
		"scope":         {"files:read"},
		"csrf_token":    {csrfToken},
		"response_type": {"code"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The redirect_uri parameter is mandatory")
}

func TestAuthorizeWithInvalidURI(t *testing.T) {
	res, err := postForm("/auth/authorize", &url.Values{
		"state":         {"123456"},
		"client_id":     {clientID},
		"redirect_uri":  {"/oauth/callback"},
		"scope":         {"files:read"},
		"csrf_token":    {csrfToken},
		"response_type": {"code"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The redirect_uri parameter doesn&#39;t match the registered ones")
}

func TestAuthorizeWithNoScope(t *testing.T) {
	res, err := postForm("/auth/authorize", &url.Values{
		"state":         {"123456"},
		"client_id":     {clientID},
		"redirect_uri":  {"https://example.org/oauth/callback"},
		"csrf_token":    {csrfToken},
		"response_type": {"code"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The scope parameter is mandatory")
}

func TestAuthorizeSuccess(t *testing.T) {
	res, err := postForm("/auth/authorize", &url.Values{
		"state":         {"123456"},
		"client_id":     {clientID},
		"redirect_uri":  {"https://example.org/oauth/callback"},
		"scope":         {"files:read"},
		"csrf_token":    {csrfToken},
		"response_type": {"code"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "302 Found", res.Status) {
		var results []oauth.AccessCode
		req := &couchdb.AllDocsRequest{}
		err = couchdb.GetAllDocs(testInstance, consts.OAuthAccessCodes, req, &results)
		assert.NoError(t, err)
		if assert.Len(t, results, 1) {
			code = results[0].Code
			expected := fmt.Sprintf("https://example.org/oauth/callback?access_code=%s&code=%s&state=123456#", code, code)
			assert.Equal(t, expected, res.Header.Get("Location"))
			assert.Equal(t, results[0].ClientID, clientID)
			assert.Equal(t, results[0].Scope, "files:read")
		}
	}
}

func TestAuthorizeSuccessOnboardingDeeplink(t *testing.T) {
	var oauthClient oauth.Client
	oauthClient.RedirectURIs = []string{"cozydrive://"}
	oauthClient.ClientName = "cozy-test-install-app"
	oauthClient.SoftwareID = "io.cozy.mobile.drive"
	oauthClient.OnboardingSecret = "toto"
	oauthClient.Create(testInstance)

	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "would like permission to access your Cozy")
	re := regexp.MustCompile(`<input type="hidden" name="csrf_token" value="(\w+)"`)
	matches := re.FindStringSubmatch(string(body))
	if assert.Len(t, matches, 2) {
		csrfToken = matches[1]
	}

	v := &url.Values{
		"state":         {"123456"},
		"client_id":     {oauthClient.ClientID},
		"redirect_uri":  {"cozydrive://"},
		"scope":         {"files:read"},
		"csrf_token":    {csrfToken},
		"response_type": {"code"},
	}
	req, err = http.NewRequest("POST", ts.URL+"/auth/authorize", bytes.NewBufferString(v.Encode()))
	assert.NoError(t, err)
	req.Host = domain
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	res, err = client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, 200, res.StatusCode) {
		content, err := ioutil.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Contains(t, string(content), "\"deeplink\":")
	}
}

func TestAuthorizeSuccessOnboarding(t *testing.T) {
	var oauthClient oauth.Client
	u := "https://example.org/oauth/callback"
	oauthClient.RedirectURIs = []string{u}
	oauthClient.ClientName = "cozy-test-install-app"
	oauthClient.SoftwareID = "io.cozy.mobile.drive"
	oauthClient.OnboardingSecret = "toto"
	oauthClient.Create(testInstance)

	res, err := postForm("/auth/authorize", &url.Values{
		"state":         {"123456"},
		"client_id":     {oauthClient.ClientID},
		"redirect_uri":  {"https://example.org/oauth/callback"},
		"scope":         {"files:read"},
		"csrf_token":    {csrfToken},
		"response_type": {"code"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, 302, res.StatusCode)
}

func TestInstallAppWithLinkedApp(t *testing.T) {
	var oauthClient oauth.Client
	u := "https://example.org/oauth/callback"
	oauthClient.RedirectURIs = []string{u}
	oauthClient.ClientName = "cozy-test-install-app"
	oauthClient.SoftwareID = "registry://drive"
	oauthClient.Create(testInstance)

	linkedClientID = oauthClient.ClientID         // Used for following tests
	linkedClientSecret = oauthClient.ClientSecret // Used for following tests

	res, err := postForm("/auth/authorize", &url.Values{
		"state":         {"123456"},
		"client_id":     {oauthClient.ClientID},
		"redirect_uri":  {u},
		"csrf_token":    {csrfToken},
		"response_type": {"code"},
	})

	assert.NoError(t, err)
	assert.Equal(t, 302, res.StatusCode)
	defer res.Body.Close()
	couchdbURL := config.GetConfig().CouchDB.URL
	db := testInstance.DBPrefix() + "%2F" + consts.Apps
	err = couchdb.EnsureDBExist(testInstance, consts.Apps)
	assert.NoError(t, err)
	reqGetChanges, err := http.NewRequest("GET", couchdbURL.String()+couchdb.EscapeCouchdbName(db)+"/_changes?feed=longpoll", nil)
	assert.NoError(t, err)
	resGetChanges, err := client.Do(reqGetChanges)
	assert.NoError(t, err)
	defer resGetChanges.Body.Close()
	assert.Equal(t, resGetChanges.StatusCode, 200)
	body, err := ioutil.ReadAll(resGetChanges.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(body), "io.cozy.apps/drive")

	var results []oauth.AccessCode
	reqDocs := &couchdb.AllDocsRequest{}
	err = couchdb.GetAllDocs(testInstance, consts.OAuthAccessCodes, reqDocs, &results)
	assert.NoError(t, err)
	for _, result := range results {
		if result.ClientID == linkedClientID {
			linkedCode = result.Code
			break
		}
	}
}

func TestCheckLinkedAppInstalled(t *testing.T) {
	// We use the webapp drive installed from the previous test
	err := auth.CheckLinkedAppInstalled(testInstance, "drive")
	assert.NoError(t, err)
}

func TestAccessTokenLinkedAppInstalled(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {linkedClientID},
		"client_secret": {linkedClientSecret},
		"code":          {linkedCode},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, 200, res.StatusCode)
	var response map[string]string
	err = json.NewDecoder(res.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, "bearer", response["token_type"])
	assert.Equal(t, "@io.cozy.apps/drive", response["scope"])
	assertValidToken(t, response["access_token"], "access", linkedClientID, "@io.cozy.apps/drive")
	assertValidToken(t, response["refresh_token"], "refresh", linkedClientID, "@io.cozy.apps/drive")
}

func TestAccessTokenNoGrantType(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "the grant_type parameter is mandatory")
}

func TestAccessTokenInvalidGrantType(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"token"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "invalid grant type")
}

func TestAccessTokenNoClientID(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"authorization_code"},
		"client_secret": {clientSecret},
		"code":          {code},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "the client_id parameter is mandatory")
}

func TestAccessTokenInvalidClientID(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {"foo"},
		"client_secret": {clientSecret},
		"code":          {code},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "the client must be registered")
}

func TestAccessTokenNoClientSecret(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type": {"authorization_code"},
		"client_id":  {clientID},
		"code":       {code},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "the client_secret parameter is mandatory")
}

func TestAccessTokenInvalidClientSecret(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"client_secret": {"foo"},
		"code":          {code},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "invalid client_secret")
}

func TestAccessTokenNoCode(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "the code parameter is mandatory")
}

func TestAccessTokenInvalidCode(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {"foo"},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "invalid code")
}

func TestAccessTokenSuccess(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "200 OK", res.Status)
	var response map[string]string
	err = json.NewDecoder(res.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, "bearer", response["token_type"])
	assert.Equal(t, "files:read", response["scope"])
	assertValidToken(t, response["access_token"], "access", clientID, "files:read")
	assertValidToken(t, response["refresh_token"], "refresh", clientID, "files:read")
	refreshToken = response["refresh_token"]
}

func TestRefreshTokenNoToken(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "invalid refresh token")
}

func TestRefreshTokenInvalidToken(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {"foo"},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "invalid refresh token")
}

func TestRefreshTokenInvalidSigningMethod(t *testing.T) {
	claims := permission.Claims{
		StandardClaims: jwt.StandardClaims{
			Audience: consts.RefreshTokenAudience,
			Issuer:   domain,
			IssuedAt: crypto.Timestamp(),
			Subject:  clientID,
		},
		Scope: "files:write",
	}
	token := jwt.NewWithClaims(jwt.GetSigningMethod("none"), claims)
	fakeToken, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	assert.NoError(t, err)
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {fakeToken},
	})
	assert.NoError(t, err)
	assertJSONError(t, res, "invalid refresh token")
}

func TestRefreshTokenSuccess(t *testing.T) {
	res, err := postForm("/auth/access_token", &url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "200 OK", res.Status)
	var response map[string]string
	err = json.NewDecoder(res.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, "bearer", response["token_type"])
	assert.Equal(t, "files:read", response["scope"])
	assert.Equal(t, "", response["refresh_token"])
	assertValidToken(t, response["access_token"], "access", clientID, "files:read")
}

func TestLogoutNoToken(t *testing.T) {
	req, _ := http.NewRequest("DELETE", ts.URL+"/auth/login", nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "401 Unauthorized", res.Status)
	cookies := jar.Cookies(nil)
	assert.Len(t, cookies, 2) // cozysessid and _csrf
}

func TestLogoutSuccess(t *testing.T) {
	a := app.WebappManifest{
		DocID:   consts.Apps + "/home",
		DocSlug: "home",
	}
	token := testInstance.BuildAppToken(a.Slug(), getSessionID(jar.Cookies(nil)))
	_, err := permission.CreateWebappSet(testInstance, a.Slug(), permission.Set{})
	assert.NoError(t, err)
	req, _ := http.NewRequest("DELETE", ts.URL+"/auth/login", nil)
	req.Host = domain
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	err = permission.DestroyWebapp(testInstance, "home")
	assert.NoError(t, err)

	assert.Equal(t, "204 No Content", res.Status)
	cookies := jar.Cookies(nil)
	assert.Len(t, cookies, 1) // _csrf
	assert.Equal(t, "_csrf", cookies[0].Name)
}

func TestLogoutOthers(t *testing.T) {
	var anonymousClient1, anonymousClient2 *http.Client
	{
		u1, _ := url.Parse(testInstance.PageURL("/", nil))
		u2, _ := url.Parse(testInstance.PageURL("/", nil))
		jar1, _ := cookiejar.New(nil)
		jar2, _ := cookiejar.New(nil)
		anonymousClient1 = &http.Client{
			CheckRedirect: noRedirect,
			Jar:           &testutils.CookieJar{Jar: jar1, URL: u1},
		}
		anonymousClient2 = &http.Client{
			CheckRedirect: noRedirect,
			Jar:           &testutils.CookieJar{Jar: jar2, URL: u2},
		}
	}

	res1, err := postFormWithClient(anonymousClient1, "/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
		"csrf_token": {getLoginCSRFToken(anonymousClient1, t)},
	})
	assert.NoError(t, err)
	defer res1.Body.Close()

	if !assert.Equal(t, "303 See Other", res1.Status) {
		return
	}
	cookies1 := res1.Cookies()
	assert.Len(t, cookies1, 2)

	res2, err := postFormWithClient(anonymousClient2, "/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
		"csrf_token": {getLoginCSRFToken(anonymousClient2, t)},
	})
	assert.NoError(t, err)
	defer res2.Body.Close()
	if !assert.Equal(t, "303 See Other", res2.Status) {
		return
	}
	cookies2 := res2.Cookies()
	assert.Len(t, cookies2, 2)

	a := app.WebappManifest{
		DocID:   consts.Apps + "/home",
		DocSlug: "home",
	}
	token := testInstance.BuildAppToken(a.Slug(), getSessionID(cookies1))
	_, err = permission.CreateWebappSet(testInstance, a.Slug(), permission.Set{})
	assert.NoError(t, err)

	reqLogout1, _ := http.NewRequest("DELETE", ts.URL+"/auth/login/others", nil)
	reqLogout1.Host = domain
	reqLogout1.Header.Add("Authorization", "Bearer "+token)
	reqLogout1.AddCookie(cookies1[1])
	resLogout1, err := client.Do(reqLogout1)
	assert.NoError(t, err)
	defer resLogout1.Body.Close()
	assert.Equal(t, 204, resLogout1.StatusCode)

	reqLogout2, _ := http.NewRequest("DELETE", ts.URL+"/auth/login/others", nil)
	reqLogout2.Host = domain
	reqLogout2.Header.Add("Authorization", "Bearer "+token)
	reqLogout2.AddCookie(cookies2[1])
	resLogout2, err := client.Do(reqLogout2)
	assert.NoError(t, err)
	defer resLogout2.Body.Close()
	assert.Equal(t, 401, resLogout2.StatusCode)

	reqLogout3, _ := http.NewRequest("DELETE", ts.URL+"/auth/login/others", nil)
	reqLogout3.Host = domain
	reqLogout3.Header.Add("Authorization", "Bearer "+token)
	reqLogout3.AddCookie(cookies1[1])
	resLogout3, err := client.Do(reqLogout3)
	assert.NoError(t, err)
	defer resLogout3.Body.Close()
	assert.Equal(t, 204, resLogout3.StatusCode)

	err = permission.DestroyWebapp(testInstance, "home")
	assert.NoError(t, err)
}

func TestPassphraseResetLoggedIn(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/passphrase_reset", nil)
	req.Host = domain
	res, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	defer res.Body.Close()
	assert.Equal(t, "200 OK", res.Status)
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), `you will be sent a link by email to choose a new pass`)
	assert.Contains(t, string(body), `<input type="hidden" name="csrf_token"`)
}

func TestPassphraseReset(t *testing.T) {
	req1, _ := http.NewRequest("GET", ts.URL+"/auth/passphrase_reset", nil)
	req1.Host = domain
	res1, err := client.Do(req1)
	if !assert.NoError(t, err) {
		return
	}
	defer res1.Body.Close()
	assert.Equal(t, "200 OK", res1.Status)
	csrfCookie := res1.Cookies()[0]
	assert.Equal(t, "_csrf", csrfCookie.Name)
	res2, err := postForm("/auth/passphrase_reset", &url.Values{
		"csrf_token": {csrfCookie.Value},
	})
	if !assert.NoError(t, err) {
		return
	}
	defer res2.Body.Close()
	assert.Equal(t, "200 OK", res2.Status)
	assert.Equal(t, "text/html; charset=UTF-8", res2.Header.Get("Content-Type"))
}

func TestPassphraseRenewFormNoToken(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/passphrase_renew", nil)
	req.Host = domain
	res, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), `The link to reset the password is truncated or has expired`)
}

func TestPassphraseRenewFormBadToken(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/passphrase_renew?token=zzzz", nil)
	req.Host = domain
	res, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), `The link to reset the password is truncated or has expired`)
}

func TestPassphraseRenewFormWithToken(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/passphrase_renew?token=badbee", nil)
	req.Host = domain
	res, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
}

func TestPassphraseRenew(t *testing.T) {
	d := "test.cozycloud.cc.web_reset_form"
	_ = lifecycle.Destroy(d)
	in1, err := lifecycle.Create(&lifecycle.Options{
		Domain: d,
		Locale: "en",
		Email:  "alice@example.com",
	})
	if !assert.NoError(t, err) {
		return
	}
	defer func() {
		_ = lifecycle.Destroy(d)
	}()
	err = lifecycle.RegisterPassphrase(in1, []byte("MyPass"), in1.RegisterToken)
	if !assert.NoError(t, err) {
		return
	}
	req1, _ := http.NewRequest("GET", ts.URL+"/auth/passphrase_reset", nil)
	req1.Host = domain
	res1, err := client.Do(req1)
	if !assert.NoError(t, err) {
		return
	}
	defer res1.Body.Close()
	csrfCookie := res1.Cookies()[0]
	assert.Equal(t, "_csrf", csrfCookie.Name)
	res2, err := postFormDomain(d, "/auth/passphrase_reset", &url.Values{
		"csrf_token": {csrfCookie.Value},
	})
	if !assert.NoError(t, err) {
		return
	}
	defer res2.Body.Close()
	assert.Equal(t, "200 OK", res2.Status)
	in2, err := instance.GetFromCouch(d)
	if !assert.NoError(t, err) {
		return
	}
	res3, err := postFormDomain(d, "/auth/passphrase_renew", &url.Values{
		"passphrase_reset_token": {hex.EncodeToString(in2.PassphraseResetToken)},
		"passphrase":             {"NewPassphrase"},
		"csrf_token":             {csrfCookie.Value},
	})
	if !assert.NoError(t, err) {
		return
	}
	defer res3.Body.Close()
	if assert.Equal(t, "303 See Other", res3.Status) {
		assert.Equal(t, "https://test.cozycloud.cc.web_reset_form/auth/login",
			res3.Header.Get("Location"))
	}
}

func TestIsLoggedOutAfterLogout(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "who_are_you", content)
}

func TestSecretExchangeGoodSecret(t *testing.T) {
	body, _ := json.Marshal(echo.Map{"secret": "foobarsecret"})

	var oauthClient oauth.Client

	oauthClient.RedirectURIs = []string{"abc"}
	oauthClient.ClientName = "cozy-test"
	oauthClient.SoftwareID = "github.com/cozy/cozy-test"
	oauthClient.OnboardingSecret = "foobarsecret"
	oauthClient.Create(testInstance)

	req, _ := http.NewRequest("POST", ts.URL+"/auth/secret_exchange", bytes.NewBuffer(body))
	req.Host = domain
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Add("Accept", "application/json")

	res, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	resBody, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(resBody), "client_secret")
	defer res.Body.Close()
}

func TestSecretExchangeBadSecret(t *testing.T) {
	body, _ := json.Marshal(echo.Map{"secret": "badbarsecret"})

	var oauthClient oauth.Client

	oauthClient.RedirectURIs = []string{"abc"}
	oauthClient.ClientName = "cozy-test"
	oauthClient.SoftwareID = "github.com/cozy/cozy-test"
	oauthClient.OnboardingSecret = "foobarsecret"
	oauthClient.Create(testInstance)

	req, _ := http.NewRequest("POST", ts.URL+"/auth/secret_exchange", bytes.NewBuffer(body))
	req.Host = domain
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Add("Accept", "application/json")

	res, err := client.Do(req)

	if !assert.NoError(t, err) {
		return
	}

	resBody, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(resBody), "errors")

	defer res.Body.Close()
}

func TestSecretExchangeBadPayload(t *testing.T) {
	body, _ := json.Marshal(echo.Map{"foo": "bar"})

	req, _ := http.NewRequest("POST", ts.URL+"/auth/secret_exchange", bytes.NewBuffer(body))
	req.Host = domain
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Add("Accept", "application/json")

	res, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	resBody, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(resBody), "Missing secret")
	defer res.Body.Close()
}

func TestSecretExchangeNoPayload(t *testing.T) {
	req, _ := http.NewRequest("POST", ts.URL+"/auth/secret_exchange", nil)
	req.Host = domain
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Add("Accept", "application/json")

	res, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, res.StatusCode, 400)
	defer res.Body.Close()
}

func TestPassphraseOnboarding(t *testing.T) {
	// Here we create a new instance without passphrase
	d := "test.cozycloud.cc.web_passphrase"
	_ = lifecycle.Destroy(d)
	inst, err := lifecycle.Create(&lifecycle.Options{
		Domain: d,
		Locale: "en",
		Email:  "alice@example.com",
	})
	assert.NoError(t, err)
	assert.False(t, inst.OnboardingFinished)

	// Should redirect to /auth/passphrase
	req, _ := http.NewRequest("GET", ts.URL+"/?registerToken="+hex.EncodeToString(inst.RegisterToken), nil)
	req.Host = inst.Domain
	res, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 303, res.StatusCode)
	assert.Contains(t, res.Header.Get("Location"), "/auth/passphrase?registerToken=")

	// Adding a passphrase and check if we are redirected to home
	pass := []byte("passphrase")
	err = lifecycle.RegisterPassphrase(inst, pass, inst.RegisterToken)
	assert.NoError(t, err)

	inst.OnboardingFinished = true

	req2, _ := http.NewRequest("GET", ts.URL+"/?registerToken="+hex.EncodeToString(inst.RegisterToken), nil)
	req2.Host = inst.Domain
	res2, err2 := client.Do(req2)
	assert.NoError(t, err2)
	assert.Contains(t, res2.Header.Get("Location"), "/auth/login")
}

func TestPassphraseOnboardingFinished(t *testing.T) {
	// Using the testInstance which had already been onboarded
	// Should redirect to home
	req, _ := http.NewRequest("GET", ts.URL+"/auth/passphrase", nil)
	req.Host = domain

	res, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, res.StatusCode, 303)
	assert.Equal(t, res.Header.Get("Location"), "https://home.cozy.example.net/")
}

func TestPassphraseOnboardingBadRegisterToken(t *testing.T) {
	// Should render need_onboarding
	d := "test.cozycloud.cc.web_passphrase_bad_token"
	_ = lifecycle.Destroy(d)
	inst, err := lifecycle.Create(&lifecycle.Options{
		Domain: d,
		Locale: "en",
		Email:  "alice@example.com",
	})
	assert.NoError(t, err)
	assert.False(t, inst.OnboardingFinished)

	// Should redirect to /auth/passphrase
	req, _ := http.NewRequest("GET", ts.URL+"/auth/passphrase?registerToken=coincoin", nil)
	req.Host = inst.Domain
	res, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	content, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, 200, res.StatusCode)
	assert.Contains(t, string(content), "Your Cozy has not been yet activated.")

}

func TestMain(m *testing.M) {
	config.UseTestFile()
	conf := config.GetConfig()
	conf.Assets = "../../assets"

	conf.Authentication = make(map[string]interface{})
	confAuth := make(map[string]interface{})
	confAuth["jwt_secret"] = base64.StdEncoding.EncodeToString(JWTSecret)
	conf.Authentication[config.DefaultInstanceContext] = confAuth

	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "auth_test")

	testInstance = setup.GetTestInstance(&lifecycle.Options{
		Domain:     domain,
		Passphrase: "MyPassphrase",
	})

	jar = setup.GetCookieJar()
	client = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}

	ts = setup.GetTestServer("/test", fakeAPI, func(r *echo.Echo) *echo.Echo {
		handler, err := web.CreateSubdomainProxy(r, apps.Serve)
		if err != nil {
			setup.CleanupAndDie("Cant start subdomain proxy", err)
		}
		return handler
	})
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	os.Exit(setup.Run())
}

func fakeAPI(g *echo.Group) {
	g.Use(middlewares.NeedInstance, middlewares.LoadSession)
	g.GET("", func(c echo.Context) error {
		var content string
		if middlewares.IsLoggedIn(c) {
			content = "logged_in"
		} else {
			content = "who_are_you"
		}
		return c.String(http.StatusOK, content)
	})
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func getJSON(u, token string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", ts.URL+u, nil)
	req.Host = domain
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)
	return client.Do(req)
}

func postJSON(u string, v echo.Map) (*http.Response, error) {
	body, _ := json.Marshal(v)
	req, _ := http.NewRequest("POST", ts.URL+u, bytes.NewBuffer(body))
	req.Host = domain
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Add("Accept", "application/json")
	return client.Do(req)
}

func putJSON(u, token string, v echo.Map) (*http.Response, error) {
	body, _ := json.Marshal(v)
	req, _ := http.NewRequest("PUT", ts.URL+u, bytes.NewBuffer(body))
	req.Host = domain
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)
	return client.Do(req)
}

func postForm(u string, v *url.Values) (*http.Response, error) {
	req, _ := http.NewRequest("POST", ts.URL+u, bytes.NewBufferString(v.Encode()))
	req.Host = domain
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return client.Do(req)
}

func postFormDomain(domain, u string, v *url.Values) (*http.Response, error) {
	req, _ := http.NewRequest("POST", ts.URL+u, bytes.NewBufferString(v.Encode()))
	req.Host = domain
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return client.Do(req)
}

func postFormWithClient(c *http.Client, u string, v *url.Values) (*http.Response, error) {
	req, _ := http.NewRequest("POST", ts.URL+u, bytes.NewBufferString(v.Encode()))
	req.Host = domain
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return c.Do(req)
}

func getTestURL() (string, error) {
	req, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	req.Host = domain
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	content, _ := ioutil.ReadAll(res.Body)
	return string(content), nil
}

func assertValidToken(t *testing.T, token, audience, subject, scope string) {
	claims := permission.Claims{}
	err := crypto.ParseJWT(token, func(token *jwt.Token) (interface{}, error) {
		return testInstance.OAuthSecret, nil
	}, &claims)
	assert.NoError(t, err)
	assert.Equal(t, audience, claims.Audience)
	assert.Equal(t, domain, claims.Issuer)
	assert.Equal(t, subject, claims.Subject)
	assert.Equal(t, scope, claims.Scope)
}

func assertJSONError(t *testing.T, res *http.Response, message string) {
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
	var response map[string]string
	err := json.NewDecoder(res.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, message, response["error"])
}
