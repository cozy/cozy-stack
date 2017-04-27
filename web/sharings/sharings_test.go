package sharings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	authClient "github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/data"
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
var token string
var iocozytests = "io.cozy.tests"

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
	var recs []*sharings.RecipientStatus
	recStatus := new(sharings.RecipientStatus)
	if recipient != nil {
		ref := couchdb.DocReference{
			ID:   recipient.RID,
			Type: consts.Recipients,
		}
		recStatus.RefRecipient = ref
		recs = append(recs, recStatus)
	}

	sharing := &sharings.Sharing{
		SharingType:      consts.OneShotSharing,
		RecipientsStatus: recs,
	}
	err := sharings.CreateSharing(testInstance, sharing)
	assert.NoError(t, err)
	return sharing, err
}

func generateAccessCode(t *testing.T, clientID, scope string) (*oauth.AccessCode, error) {
	access, err := oauth.CreateAccessCode(recipientIn, clientID, scope)
	assert.NoError(t, err)
	return access, err
}

func TestReceiveDocumentSuccessJSON(t *testing.T) {
	jsondataID := "1234bepoauie"
	jsondata := echo.Map{
		"test": "test",
		"id":   jsondataID,
	}
	jsonraw, err := json.Marshal(jsondata)
	assert.NoError(t, err)

	url, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	url.Path = fmt.Sprintf("/sharings/doc/%s/%s", iocozytests, jsondataID)

	req, err := http.NewRequest(http.MethodPost, url.String(),
		bytes.NewReader(jsonraw))
	assert.NoError(t, err)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Set(echo.HeaderContentType, "application/json")

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	// Ensure that document is present by fetching it.
	doc := &couchdb.JSONDoc{}
	err = couchdb.GetDoc(testInstance, iocozytests, jsondataID, doc)
	assert.NoError(t, err)
}

func TestReceiveDocumentSuccessDir(t *testing.T) {
	id := "0987jldvnrst"

	urlDest, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	urlDest.Path = fmt.Sprintf("/sharings/doc/%s/%s", consts.Files, id)
	urlDest.RawQuery = fmt.Sprintf("Name=TestDir&Type=%s", consts.DirType)

	req, err := http.NewRequest(http.MethodPost, urlDest.String(), nil)
	assert.NoError(t, err)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Ensure that the folder was created by fetching it.
	fs := testInstance.VFS()
	_, err = fs.DirByID(id)
	assert.NoError(t, err)
}

func TestReceiveDocumentSuccessFile(t *testing.T) {
	id := "testid"
	body := "testoutest"

	urlDest, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	urlDest.Path = fmt.Sprintf("/sharings/doc/%s/%s", consts.Files, id)
	values := url.Values{
		"Name":       {"TestFile"},
		"Executable": {"false"},
		"Type":       {consts.FileType},
	}
	urlDest.RawQuery = values.Encode()
	buf := strings.NewReader(body)

	req, err := http.NewRequest(http.MethodPost, urlDest.String(), buf)
	assert.NoError(t, err)
	req.Header.Add("Content-MD5", "VkzK5Gw9aNzQdazZe4y1cw==")
	req.Header.Add(echo.HeaderContentType, "text/plain")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	fs := testInstance.VFS()
	_, err = fs.FileByID(id)
	assert.NoError(t, err)
}

func TestUpdateDocumentSuccessJSON(t *testing.T) {
	resp, err := postJSON(t, "/data/"+iocozytests+"/", echo.Map{
		"testcontent": "old",
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	doc := couchdb.JSONDoc{}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&doc)
	assert.NoError(t, err)
	doc.SetID(doc.M["id"].(string))
	doc.SetRev(doc.M["rev"].(string))
	doc.Type = doc.M["type"].(string)
	doc.M["testcontent"] = "new"
	values, err := doc.MarshalJSON()
	assert.NoError(t, err)

	path := fmt.Sprintf("/sharings/doc/%s/%s", doc.DocType(), doc.ID())
	req, err := http.NewRequest(http.MethodPut, ts.URL+path,
		bytes.NewReader(values))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, "application/json")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	updatedDoc := &couchdb.JSONDoc{}
	err = couchdb.GetDoc(testInstance, doc.DocType(), doc.ID(), updatedDoc)
	assert.NoError(t, err)
	assert.Equal(t, doc.M["testcontent"], updatedDoc.M["testcontent"])
}

func TestDeleteDocumentSuccessJSON(t *testing.T) {
	// To delete a JSON we need to create one and get its revision.
	resp, err := postJSON(t, "/data/"+iocozytests+"/", echo.Map{
		"test": "content",
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	doc := couchdb.JSONDoc{}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&doc)
	assert.NoError(t, err)

	delURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	delURL.Path = fmt.Sprintf("/sharings/doc/%s/%s", doc.M["type"], doc.M["id"])
	delURL.RawQuery = url.Values{"rev": {doc.M["rev"].(string)}}.Encode()

	req, err := http.NewRequest("DELETE", delURL.String(), nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	delDoc := &couchdb.JSONDoc{}
	err = couchdb.GetDoc(testInstance, doc.DocType(), doc.ID(), delDoc)
	assert.Error(t, err)
}

func TestAddSharingRecipientNoSharing(t *testing.T) {
	res, err := putJSON(t, "/sharings/fakeid/recipient", echo.Map{})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestAddSharingRecipientBadRecipient(t *testing.T) {
	sharing, err := createSharing(t, nil)
	assert.NoError(t, err)
	args := echo.Map{
		"ID":   "fakeid",
		"Type": "io.cozy.recipients",
	}
	url := "/sharings/" + sharing.ID() + "/recipient"
	res, err := putJSON(t, url, args)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestAddSharingRecipientSuccess(t *testing.T) {
	recipient, err := createRecipient(t)
	assert.NoError(t, err)
	sharing, err := createSharing(t, recipient)
	assert.NoError(t, err)
	args := echo.Map{
		"ID":   recipient.ID(),
		"Type": "io.cozy.recipients",
	}
	url := "/sharings/" + sharing.ID() + "/recipient"
	res, err := putJSON(t, url, args)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
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

func TestCreateRecipientNoURL(t *testing.T) {
	email := "mailme@maybe"
	res, err := postJSON(t, "/sharings/recipient", echo.Map{
		"email": email,
	})
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestCreateRecipientSuccess(t *testing.T) {
	email := "mailme@maybe"
	url := strings.Split(ts2.URL, "http://")[1]
	res, err := postJSON(t, "/sharings/recipient", echo.Map{
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
	assert.Equal(t, 400, res.StatusCode)
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
	res, err := postJSON(t, "/sharings/", echo.Map{
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

	res, err := postJSON(t, "/sharings/", echo.Map{
		"sharing_type": consts.OneShotSharing,
		"recipients":   recipients,
	})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestCreateSharingSuccess(t *testing.T) {
	res, err := postJSON(t, "/sharings/", echo.Map{
		"sharing_type": consts.OneShotSharing,
	})
	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)
}

func TestReceiveClientIDBadSharing(t *testing.T) {
	recipient, err := createRecipient(t)
	assert.NoError(t, err)
	sharing, err := createSharing(t, recipient)
	assert.NoError(t, err)
	assert.NotNil(t, sharing)
	authCli := &authClient.Client{
		ClientID: "myclientid",
	}
	sharing.RecipientsStatus[0].Client = authCli
	err = couchdb.UpdateDoc(testInstance, sharing)
	assert.NoError(t, err)
	res, err := postJSON(t, "/sharings/access/client", echo.Map{
		"state":          "fakestate",
		"client_id":      "fakeclientid",
		"host_client_id": "newclientid",
	})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestReceiveClientIDSuccess(t *testing.T) {
	recipient, err := createRecipient(t)
	assert.NoError(t, err)
	sharing, err := createSharing(t, recipient)
	assert.NoError(t, err)
	assert.NotNil(t, sharing)
	authCli := &authClient.Client{
		ClientID: "myclientid",
	}
	sharing.RecipientsStatus[0].Client = authCli
	err = couchdb.UpdateDoc(testInstance, sharing)
	assert.NoError(t, err)
	res, err := postJSON(t, "/sharings/access/client", echo.Map{
		"state":          sharing.SharingID,
		"client_id":      sharing.RecipientsStatus[0].Client.ClientID,
		"host_client_id": "newclientid",
	})
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()

	setup := testutils.NewSetup(m, "sharing_test_alice")
	setup2 := testutils.NewSetup(m, "sharing_test_bob")
	var settings couchdb.JSONDoc
	settings.M = make(map[string]interface{})
	settings.M["public_name"] = "Alice"
	testInstance = setup.GetTestInstance(&instance.Options{
		Settings: settings,
	})
	var settings2 couchdb.JSONDoc
	settings2.M = make(map[string]interface{})
	settings2.M["public_name"] = "Bob"
	recipientIn = setup2.GetTestInstance(&instance.Options{
		Settings: settings2,
	})

	err := couchdb.ResetDB(testInstance, iocozytests)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.ResetDB(testInstance, consts.Files)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	jar = setup.GetCookieJar()
	client = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}

	scope := consts.Files + " " + iocozytests + " " + consts.Sharings
	clientOAuth, token = setup.GetTestClient(scope)
	clientID = clientOAuth.ClientID

	routes := map[string]func(*echo.Group){
		"/sharings": Routes,
		"/data":     data.Routes,
	}
	ts = setup.GetTestServerMultipleRoutes(routes)
	ts2 = setup2.GetTestServer("/auth", auth.Routes)
	recipientURL = strings.Split(ts2.URL, "http://")[1]

	setup.AddCleanup(func() error { setup2.Cleanup(); return nil })

	os.Exit(setup.Run())
}

func postJSON(t *testing.T, path string, v echo.Map) (*http.Response, error) {
	body, _ := json.Marshal(v)
	req, err := http.NewRequest(http.MethodPost, ts.URL+path,
		bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, "application/json")

	return http.DefaultClient.Do(req)
}

func putJSON(t *testing.T, path string, v echo.Map) (*http.Response, error) {
	body, _ := json.Marshal(v)
	req, err := http.NewRequest(http.MethodPut, ts.URL+path,
		bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, "application/json")

	return http.DefaultClient.Do(req)
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
