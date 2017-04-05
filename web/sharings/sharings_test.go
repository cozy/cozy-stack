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
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var ts2 *httptest.Server
var testInstance *instance.Instance
var recipientIn *instance.Instance
var clientOAuth *oauth.Client
var clientID string
var jar http.CookieJar
var client *http.Client
var recipientURL string

func createRecipient(t *testing.T) (*sharings.Recipient, error) {
	recipient := &sharings.Recipient{
		Email: "test.fr",
		URL:   "http://" + recipientURL,
	}
	err := sharings.CreateRecipient(testInstance, recipient)
	assert.NoError(t, err)
	return recipient, err
}

func createSharing(t *testing.T, recipient *sharings.Recipient) (*sharings.Sharing, error) {
	recStatus := &sharings.RecipientStatus{
		RefRecipient: couchdb.DocReference{
			ID:   recipient.RID,
			Type: consts.Recipients,
		},
	}

	sharing := &sharings.Sharing{
		SharingType:      consts.OneShotSharing,
		RecipientsStatus: []*sharings.RecipientStatus{recStatus},
	}
	err := sharings.CreateSharingAndRegisterSharer(testInstance, sharing)
	assert.NoError(t, err)
	return sharing, err
}

func generateAccessCode(t *testing.T, clientID, scope string) (*oauth.AccessCode, error) {
	access, err := oauth.CreateAccessCode(recipientIn, clientID, scope)
	assert.NoError(t, err)
	return access, err
}

func TestRecipientRefusedSharingWhenThereIsNoState(t *testing.T) {
	urlVal := url.Values{
		"state":     {""},
		"client_id": {"randomclientid"},
	}

	resp, err := formPOST("/sharings/formRefuse", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 400)
}

func TestRecipientRefusedSharingWhenThereIsNoClientID(t *testing.T) {
	urlVal := url.Values{
		"state":     {"randomsharingid"},
		"client_id": {""},
	}

	resp, err := formPOST("/sharings/formRefuse", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 400)
}

func TestRecipientRefusedSharingSuccess(t *testing.T) {
	// To be able to refuse a sharing we first need to receive a sharing
	// request… This is a copy/paste of the code found in the test:
	// TestSharingRequestSuccess.
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

	resp, err := formPOST("/sharings/formRefuse", url.Values{
		"state":     {state},
		"client_id": {clientID},
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusFound, resp.StatusCode)
}

func TestSharingAnswerBadState(t *testing.T) {
	urlVal := url.Values{
		"state": {""},
	}
	res, err := requestGET("/sharings/answer", urlVal)
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
	urlVal := url.Values{
		"state":     {"stateoftheart"},
		"client_id": {"myclient"},
	}
	res, err := requestGET("/sharings/answer", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestSharingAnswerBadCode(t *testing.T) {
	recipient, err := createRecipient(t)
	assert.NoError(t, err)
	assert.NotNil(t, recipient)
	sharing, err := createSharing(t, recipient)
	assert.NoError(t, err)
	assert.NotNil(t, sharing)

	urlVal := url.Values{
		"state":       {sharing.SharingID},
		"client_id":   {sharing.RecipientsStatus[0].Client.ClientID},
		"access_code": {"fakeaccess"},
	}
	res, err := requestGET("/sharings/answer", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func TestSharingAnswerSuccess(t *testing.T) {
	recipient, err := createRecipient(t)
	assert.NoError(t, err)
	assert.NotNil(t, recipient)
	sharing, err := createSharing(t, recipient)
	assert.NoError(t, err)
	assert.NotNil(t, sharing)

	cID := sharing.RecipientsStatus[0].Client.ClientID

	access, err := generateAccessCode(t, cID, "")
	assert.NoError(t, err)
	assert.NotNil(t, access)

	urlVal := url.Values{
		"state":       {sharing.SharingID},
		"client_id":   {cID},
		"access_code": {access.Code},
	}
	_, err = requestGET("/sharings/answer", urlVal)
	assert.NoError(t, err)
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
		"scope":        {"dummyscope"},
		"sharing_type": {consts.OneShotSharing},
		"client_id":    {"dummyclientid"},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestNoSharingType(t *testing.T) {
	urlVal := url.Values{
		"scope":     {"dummyscope"},
		"state":     {"dummystate"},
		"client_id": {"dummyclientid"},
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
		"client_id":    {"dummyclientid"},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func TestSharingRequestNoClientID(t *testing.T) {
	urlVal := url.Values{
		"scope":        {"dummyscope"},
		"state":        {"dummystate"},
		"sharing_type": {consts.OneShotSharing},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestBadClientID(t *testing.T) {
	urlVal := url.Values{
		"scope":        {"dummyscope"},
		"state":        {"dummystate"},
		"sharing_type": {consts.OneShotSharing},
		"client_id":    {"badclientid"},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
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
	recipientIn = setup2.GetTestInstance(&instance.Options{
		PublicName: "Bob",
	})

	clientOAuth, _ = setup.GetTestClient("")
	clientID = clientOAuth.ClientID

	ts = setup.GetTestServer("/sharings", Routes)
	ts2 = setup2.GetTestServer("/auth", auth.Routes)
	recipientURL = strings.Split(ts2.URL, "http://")[1]

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

func formPOST(u string, v url.Values) (*http.Response, error) {
	req, _ := http.NewRequest("POST", ts.URL+u, strings.NewReader(v.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Host = testInstance.Domain
	noRedirectClient := http.Client{CheckRedirect: noRedirect}
	return noRedirectClient.Do(req)
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
