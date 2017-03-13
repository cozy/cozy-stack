package sharings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var testInstance *instance.Instance
var jar *testJar
var client *http.Client
var clientOAuth oauth.Client
var clientID string
var instanceURL *url.URL
var domain string

type testJar struct {
	Jar *cookiejar.Jar
}

func (j *testJar) Cookies(u *url.URL) (cookies []*http.Cookie) {
	return j.Jar.Cookies(instanceURL)
}

func (j *testJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.Jar.SetCookies(instanceURL, cookies)
}

func TestSharingAnswerBadState(t *testing.T) {
	state := ""
	res, err := postJSON("/sharings/answer", echo.Map{
		"state": state,
	})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestSharingAnswerBadClientID(t *testing.T) {
	state := "stateoftheart"
	clientID2 := "myclient"
	res, err := postJSON("/sharings/answer", echo.Map{
		"state":     state,
		"client_id": clientID2,
	})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestSharingRequestNoScope(t *testing.T) {
	urlVal := url.Values{}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestNoState(t *testing.T) {
	urlVal := url.Values{
		"scope": {"dummyscope"},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestNoSharingType(t *testing.T) {
	urlVal := url.Values{
		"scope": {"dummyscope"},
		"state": {"dummystate"},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 422, res.StatusCode)
}

func TestSharingRequestBadScope(t *testing.T) {
	urlVal := url.Values{
		"scope":        []string{":"},
		"state":        {"dummystate"},
		"sharing_type": {consts.OneShotSharing},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func TestSharingRequestSuccess(t *testing.T) {

	rule := permissions.Rule{
		Type:        "io.cozy.events",
		Title:       "event",
		Description: "My event",
		Verbs:       permissions.VerbSet{permissions.POST: {}},
		Values:      []string{"1234"},
	}
	set := permissions.Set{rule}
	scope, err := set.MarshalScopeString()
	assert.NoError(t, err)

	state := "sharing_id"
	desc := "share cher"

	urlVal := url.Values{
		"desc":          {desc},
		"state":         {state},
		"scope":         {scope},
		"sharing_type":  {consts.OneShotSharing},
		"client_id":     {clientID},
		"redirect_uri":  {clientOAuth.RedirectURIs[0]},
		"response_type": {"code"},
	}

	req, _ := http.NewRequest("GET", ts.URL+"/sharings/request?"+urlVal.Encode(), nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusSeeOther, res.StatusCode)
}

func TestCreateSharingWithBadType(t *testing.T) {
	res, err := postJSON("/sharings/", echo.Map{
		"sharing_type": "shary pie",
	})
	assert.NoError(t, err)
	assert.Equal(t, 422, res.StatusCode)
}

func TestSendMailsWithWrongSharingID(t *testing.T) {
	req, _ := http.NewRequest("PUT", ts.URL+"/sharings/wrongid/sendMails",
		nil)

	res, err := http.DefaultClient.Do(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestCreateSharingWithNonExistingRecipient(t *testing.T) {
	type recipient map[string]map[string]string

	rec := recipient{
		"recipient": {
			"id": "hodor",
		},
	}
	recipients := []recipient{rec}

	res, err := postJSON("/sharings/", echo.Map{
		"sharing_type": consts.OneShotSharing,
		"recipients":   recipients,
	})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestCreateSharingSuccess(t *testing.T) {
	res, err := postJSON("/sharings/", echo.Map{
		"sharing_type": consts.OneShotSharing,
	})
	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test needs couchdb to run.")
		os.Exit(1)
	}

	domain = "test-sharings"

	instance.Destroy(domain)
	testInstance, err = instance.Create(&instance.Options{
		Domain: domain,
		Locale: "en",
	})
	if err != nil {
		fmt.Println("Could not create test instance.", err)
		os.Exit(1)
	}

	instanceURL, _ = url.Parse("https://" + domain + "/")
	j, _ := cookiejar.New(nil)
	jar = &testJar{
		Jar: j,
	}
	client = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}

	testInstance.RegisterPassphrase([]byte("MyPassphrase"), testInstance.RegisterToken)

	r := echo.New()
	r.GET("/test", func(c echo.Context) error {
		var content string
		if middlewares.IsLoggedIn(c) {
			content = "logged_in"
		} else {
			content = "who_are_you"
		}
		return c.String(http.StatusOK, content)
	}, middlewares.NeedInstance, middlewares.LoadSession)

	clientOAuth = oauth.Client{
		RedirectURIs: []string{"http://localhost/oauth/callback"},
		ClientName:   "test-permissions",
		SoftwareID:   "github.com/cozy/cozy-stack/web/data",
	}
	clientOAuth.Create(testInstance)
	clientID = clientOAuth.ClientID

	handler := echo.New()
	handler.HTTPErrorHandler = errors.ErrorHandler
	handler.Use(injectInstance(testInstance))
	auth.Routes(handler.Group("/auth"))
	Routes(handler.Group("/sharings"))

	ts = httptest.NewServer(handler)

	res := m.Run()
	ts.Close()
	instance.Destroy(domain)

	os.Exit(res)
}

func postJSON(u string, v echo.Map) (*http.Response, error) {
	body, _ := json.Marshal(v)
	return http.Post(ts.URL+u, "application/json", bytes.NewReader(body))
}

func requestGET(u string, v url.Values) (*http.Response, error) {
	if v != nil {
		reqURL := v.Encode()
		return http.Get(ts.URL + u + "?" + reqURL)
	}
	return http.Get(ts.URL + u)
}

func extractJSONRes(res *http.Response, mp *map[string]interface{}) error {
	if res.StatusCode >= 300 {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(mp)
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
