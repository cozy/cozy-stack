package sharings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var ts2 *httptest.Server
var testInstance *instance.Instance
var clientOAuth *oauth.Client
var clientID string
var jar http.CookieJar
var client *http.Client

func TestRecipientRefusedSharingWithNoState(t *testing.T) {
	res, err := postForm("/sharings/formRefuse", &url.Values{
		"state": {""},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
}

func TestRecipientRefusedSharingWithNoClientID(t *testing.T) {
	res, err := postForm("/sharings/formRefuse", &url.Values{
		"state":     {"dummystate"},
		"client_id": {""},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
}

func TestSharingAnswerBadState(t *testing.T) {
	state := ""
	res, err := postJSON("/sharings/answer", echo.Map{
		"state": state,
	})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestAddRecipientNoURL(t *testing.T) {
	email := "mailme@maybe"
	res, err := postJSON("/sharings/recipient", echo.Map{
		"email": email,
	})
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestAddRecipientSuccess(t *testing.T) {
	email := "mailme@maybe"
	url := strings.Split(ts2.URL, "http://")[1]
	res, err := postJSON("/sharings/recipient", echo.Map{
		"url":   url,
		"email": email,
	})

	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)
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
	urlVal := url.Values{
		"state":        {"dummystate"},
		"sharing_type": {consts.OneShotSharing},
	}
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
	noRedirectClient := http.Client{CheckRedirect: noRedirect}
	res, err := noRedirectClient.Do(req)
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
	testutils.NeedCouchdb()

	setup := testutils.NewSetup(m, "sharing_test_alice")
	setup2 := testutils.NewSetup(m, "sharing_test_bob")
	testInstance = setup.GetTestInstance(&instance.Options{
		PublicName: "Alice",
	})
	_ = setup2.GetTestInstance(&instance.Options{
		PublicName: "Bob",
	})

	jar = setup.GetCookieJar()
	client = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}
	clientOAuth, _ = setup.GetTestClient("")
	clientID = clientOAuth.ClientID

	ts = setup.GetTestServer("/sharings", Routes)
	ts2 = setup2.GetTestServer("/auth", auth.Routes)

	setup.AddCleanup(func() error { setup2.Cleanup(); return nil })
	os.Exit(setup.Run())
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

func postForm(u string, v *url.Values) (*http.Response, error) {
	req, _ := http.NewRequest("POST", ts.URL+u, bytes.NewBufferString(v.Encode()))
	req.Host = testInstance.Domain
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return client.Do(req)
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
