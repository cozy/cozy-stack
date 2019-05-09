package sharings_test

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/cozy-stack/web/sharings"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

const iocozytests = "io.cozy.tests"

// Things that live on Alice's Cozy
var tsA *httptest.Server
var aliceInstance *instance.Instance
var aliceAppToken string
var bobContact, charlieContact, daveContact, edwardContact *contact.Contact
var sharingID, state, aliceAccessToken string

// Things that live on Bob's Cozy
var tsB *httptest.Server
var bobInstance *instance.Instance
var bobAppToken string

// Bob's browser
var bobUA *http.Client
var discoveryLink, authorizeLink string
var csrfToken string

func assertSharingByAliceToBobAndDave(t *testing.T, members []interface{}) {
	assert.Len(t, members, 3)
	owner := members[0].(map[string]interface{})
	assert.Equal(t, owner["status"], "owner")
	assert.Equal(t, owner["public_name"], "Alice")
	assert.Equal(t, owner["email"], "alice@example.net")
	assert.Equal(t, owner["instance"], "https://"+aliceInstance.Domain)
	recipient := members[1].(map[string]interface{})
	assert.Equal(t, recipient["status"], "pending")
	assert.Equal(t, recipient["name"], "Bob")
	assert.Equal(t, recipient["email"], "bob@example.net")
	assert.NotEqual(t, recipient["read_only"], true)
	recipient = members[2].(map[string]interface{})
	assert.Equal(t, recipient["status"], "pending")
	assert.Equal(t, recipient["name"], "Dave")
	assert.Equal(t, recipient["email"], "dave@example.net")
	assert.Equal(t, recipient["read_only"], true)
}

func assertSharingIsCorrectOnSharer(t *testing.T, body io.Reader) {
	var result map[string]interface{}
	assert.NoError(t, json.NewDecoder(body).Decode(&result))
	data := result["data"].(map[string]interface{})
	assert.Equal(t, data["type"], consts.Sharings)
	sharingID = data["id"].(string)
	assert.NotEmpty(t, sharingID)
	assert.NotEmpty(t, data["meta"].(map[string]interface{})["rev"])
	self := "/sharings/" + sharingID
	assert.Equal(t, data["links"].(map[string]interface{})["self"], self)

	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, attrs["description"], "this is a test")
	assert.Equal(t, attrs["app_slug"], "testapp")
	assert.Equal(t, attrs["owner"], true)
	assert.NotEmpty(t, attrs["created_at"])
	assert.NotEmpty(t, attrs["updated_at"])
	assert.Nil(t, attrs["credentials"])

	members := attrs["members"].([]interface{})
	assertSharingByAliceToBobAndDave(t, members)

	rules := attrs["rules"].([]interface{})
	assert.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	assert.Equal(t, rule["title"], "test one")
	assert.Equal(t, rule["doctype"], iocozytests)
	assert.Equal(t, rule["values"], []interface{}{"foobar"})
}

func assertInvitationMailWasSent(t *testing.T) string {
	var jobs []job.Job
	couchReq := &couchdb.FindRequest{
		UseIndex: "by-worker-and-state",
		Selector: mango.And(
			mango.Equal("worker", "sendmail"),
			mango.Exists("state"),
		),
		Sort: mango.SortBy{
			mango.SortByField{Field: "worker", Direction: "desc"},
		},
		Limit: 2,
	}
	err := couchdb.FindDocs(aliceInstance, consts.Jobs, couchReq, &jobs)
	assert.NoError(t, err)
	assert.Len(t, jobs, 2)
	var msg map[string]interface{}
	// Ignore the mail sent to Dave
	err = json.Unmarshal(jobs[0].Message, &msg)
	assert.NoError(t, err)
	if msg["template_values"].(map[string]interface{})["RecipientName"] == "Dave" {
		err = json.Unmarshal(jobs[1].Message, &msg)
		assert.NoError(t, err)
	}
	assert.Equal(t, msg["mode"], "from")
	assert.Equal(t, msg["template_name"], "sharing_request")
	values := msg["template_values"].(map[string]interface{})
	assert.Equal(t, values["RecipientName"], "Bob")
	assert.Equal(t, values["SharerPublicName"], "Alice")
	discoveryLink = values["SharingLink"].(string)
	return values["Description"].(string)
}

func TestCreateSharingSuccess(t *testing.T) {
	assert.NotEmpty(t, aliceAppToken)
	assert.NotNil(t, bobContact)

	v := echo.Map{
		"data": echo.Map{
			"type": consts.Sharings,
			"attributes": echo.Map{
				"description":  "this is a test",
				"open_sharing": true,
				"rules": []interface{}{
					echo.Map{
						"title":   "test one",
						"doctype": iocozytests,
						"values":  []string{"foobar"},
						"add":     "sync",
					},
				},
			},
			"relationships": echo.Map{
				"recipients": echo.Map{
					"data": []interface{}{
						echo.Map{
							"id":      bobContact.ID(),
							"doctype": bobContact.DocType(),
						},
					},
				},
				"read_only_recipients": echo.Map{
					"data": []interface{}{
						echo.Map{
							"id":      daveContact.ID(),
							"doctype": daveContact.DocType(),
						},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(v)
	r := bytes.NewReader(body)

	req, err := http.NewRequest(http.MethodPost, tsA.URL+"/sharings/", r)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, res.StatusCode)
	defer res.Body.Close()

	assertSharingIsCorrectOnSharer(t, res.Body)
	description := assertInvitationMailWasSent(t)
	assert.Equal(t, description, "this is a test")
	assert.Contains(t, discoveryLink, "/discovery?state=")
}

func TestGetSharing(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, tsA.URL+"/sharings/"+sharingID, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	defer res.Body.Close()

	assertSharingIsCorrectOnSharer(t, res.Body)
}

func assertSharingRequestHasBeenCreated(t *testing.T) {
	var results []*sharing.Sharing
	req := couchdb.AllDocsRequest{}
	err := couchdb.GetAllDocs(bobInstance, consts.Sharings, &req, &results)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	s := results[0]
	assert.Equal(t, s.SID, sharingID)
	assert.False(t, s.Active)
	assert.False(t, s.Owner)
	assert.Equal(t, s.Description, "this is a test")
	assert.Equal(t, s.AppSlug, "testapp")

	assert.Len(t, s.Members, 3)
	owner := s.Members[0]
	assert.Equal(t, owner.Status, "owner")
	assert.Equal(t, owner.PublicName, "Alice")
	assert.Equal(t, owner.Email, "alice@example.net")
	assert.Equal(t, owner.Instance, "https://"+aliceInstance.Domain)
	recipient := s.Members[1]
	assert.Equal(t, recipient.Status, "pending")
	assert.Equal(t, recipient.Email, "bob@example.net")
	assert.Equal(t, recipient.Instance, tsB.URL)
	recipient = s.Members[2]
	assert.Equal(t, recipient.Status, "pending")
	assert.Equal(t, recipient.Email, "dave@example.net")
	assert.Equal(t, recipient.ReadOnly, true)

	assert.Len(t, s.Rules, 1)
	rule := s.Rules[0]
	assert.Equal(t, rule.Title, "test one")
	assert.Equal(t, rule.DocType, iocozytests)
	assert.Equal(t, rule.Values, []string{"foobar"})
}

func assertSharingInfoRequestIsCorrect(t *testing.T, body io.Reader, s1, s2 string) {
	var result map[string]interface{}
	assert.NoError(t, json.NewDecoder(body).Decode(&result))
	d := result["data"].([]interface{})
	data := make([]map[string]interface{}, len(d))
	s1Found := false
	s2Found := false
	for i := range d {
		data[i] = d[i].(map[string]interface{})
		assert.Equal(t, consts.Sharings, data[i]["type"])
		sharingID = data[i]["id"].(string)
		assert.NotEmpty(t, sharingID)
		rel := data[i]["relationships"].(map[string]interface{})
		sharedDocs := rel["shared_docs"].(map[string]interface{})
		assert.NotEmpty(t, sharedDocs)

		if sharingID == s1 {
			sharedDocsData := sharedDocs["data"].([]interface{})
			assert.Equal(t, "fakeid1", sharedDocsData[0].(map[string]interface{})["id"])
			assert.Equal(t, "fakeid2", sharedDocsData[1].(map[string]interface{})["id"])
			assert.Equal(t, "fakeid3", sharedDocsData[2].(map[string]interface{})["id"])
			s1Found = true
		} else if sharingID == s2 {
			sharedDocsData := sharedDocs["data"].([]interface{})
			assert.Equal(t, "fakeid4", sharedDocsData[0].(map[string]interface{})["id"])
			assert.Equal(t, "fakeid5", sharedDocsData[1].(map[string]interface{})["id"])
			s2Found = true
		}
	}
	assert.Equal(t, true, s1Found)
	assert.Equal(t, true, s2Found)
}

func TestDiscovery(t *testing.T) {
	parts := strings.Split(tsA.URL, "://")
	u, err := url.Parse(discoveryLink)
	assert.NoError(t, err)
	u.Scheme = parts[0]
	u.Host = parts[1]
	state = u.Query()["state"][0]
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	assert.NoError(t, err)
	res, err := bobUA.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "Connect to your Cozy")
	assert.Contains(t, string(body), `<input id="url" name="url"`)
	assert.Contains(t, string(body), `<input type="hidden" name="state" value="`+state)

	u.RawQuery = ""
	v := &url.Values{
		"state": {state},
		"url":   {tsB.URL},
	}
	req, err = http.NewRequest(http.MethodPost, u.String(), bytes.NewBufferString(v.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	assert.NoError(t, err)
	res, err = bobUA.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusFound, res.StatusCode)
	authorizeLink = res.Header.Get("Location")
	assert.Contains(t, authorizeLink, tsB.URL)
	assert.Contains(t, authorizeLink, "/auth/authorize/sharing")

	assertSharingRequestHasBeenCreated(t)
}

func bobLogin(t *testing.T) {
	res, err := bobUA.Get(tsB.URL + "/auth/login")
	assert.NoError(t, err)
	res.Body.Close()
	token := res.Cookies()[0].Value

	v := &url.Values{
		"passphrase": {"MyPassphrase"},
		"csrf_token": {token},
	}
	req, err := http.NewRequest(http.MethodPost, tsB.URL+"/auth/login", bytes.NewBufferString(v.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	assert.NoError(t, err)
	res, err = bobUA.Do(req)
	assert.NoError(t, err)
	res.Body.Close()
	assert.Equal(t, http.StatusSeeOther, res.StatusCode)
	assert.Contains(t, res.Header.Get("Location"), "home")
}

func fakeAliceInstance(t *testing.T) {
	var results []*sharing.Sharing
	req := couchdb.AllDocsRequest{}
	err := couchdb.GetAllDocs(bobInstance, consts.Sharings, &req, &results)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	s := results[0]
	assert.Len(t, s.Members, 3)
	s.Members[0].Instance = tsA.URL
	err = couchdb.UpdateDoc(bobInstance, s)
	assert.NoError(t, err)
}

func assertAuthorizePageShowsTheSharing(t *testing.T, body string) {
	assert.Contains(t, body, "would like to share the following data with you")
	assert.Contains(t, body, `<input type="hidden" name="sharing_id" value="`+sharingID)
	assert.Contains(t, body, `<input type="hidden" name="state" value="`+state)
	re := regexp.MustCompile(`<input type="hidden" name="csrf_token" value="(\w+)"`)
	matches := re.FindStringSubmatch(body)
	if assert.Len(t, matches, 2) {
		csrfToken = matches[1]
	}
	assert.Contains(t, body, `<li class="io.cozy.tests">test one</li>`)
	assert.Contains(t, body, `<li>Your Cozy: `+bobInstance.Domain+`</li>`)
	assert.Contains(t, body, `<li>Your contact&#39;s Cozy: 127.0.0.1:`)
}

func assertCredentialsHasBeenExchanged(t *testing.T) {
	var resultsA []map[string]interface{}
	req := couchdb.AllDocsRequest{}
	err := couchdb.GetAllDocs(bobInstance, consts.OAuthClients, &req, &resultsA)
	assert.NoError(t, err)
	assert.True(t, len(resultsA) > 0)
	clientA := resultsA[len(resultsA)-1]
	assert.Equal(t, clientA["client_kind"], "sharing")
	assert.Equal(t, clientA["client_uri"], tsA.URL+"/")
	assert.Equal(t, clientA["client_name"], "Sharing Alice")

	var resultsB []map[string]interface{}
	err = couchdb.GetAllDocs(aliceInstance, consts.OAuthClients, &req, &resultsB)
	assert.NoError(t, err)
	assert.True(t, len(resultsB) > 0)
	clientB := resultsB[len(resultsB)-1]
	assert.Equal(t, clientB["client_kind"], "sharing")
	assert.Equal(t, clientB["client_uri"], tsB.URL+"/")
	assert.Equal(t, clientB["client_name"], "Sharing Bob")

	var sharingsA []*sharing.Sharing
	err = couchdb.GetAllDocs(aliceInstance, consts.Sharings, &req, &sharingsA)
	assert.NoError(t, err)
	assert.True(t, len(sharingsA) > 0)
	assert.Len(t, sharingsA[0].Credentials, 2)
	credentials := sharingsA[0].Credentials[0]
	if assert.NotNil(t, credentials.Client) {
		assert.Equal(t, credentials.Client.ClientID, clientA["_id"])
	}
	if assert.NotNil(t, credentials.AccessToken) {
		assert.NotEmpty(t, credentials.AccessToken.AccessToken)
		assert.NotEmpty(t, credentials.AccessToken.RefreshToken)
		aliceAccessToken = credentials.AccessToken.AccessToken
	}
	assert.Equal(t, sharingsA[0].Members[1].Status, "ready")
	assert.Equal(t, sharingsA[0].Members[2].Status, "pending")

	var sharingsB []*sharing.Sharing
	err = couchdb.GetAllDocs(bobInstance, consts.Sharings, &req, &sharingsB)
	assert.NoError(t, err)
	assert.True(t, len(sharingsB) > 0)
	assert.Len(t, sharingsB[0].Credentials, 1)
	credentials = sharingsB[0].Credentials[0]
	if assert.NotNil(t, credentials.Client) {
		assert.Equal(t, credentials.Client.ClientID, clientB["_id"])
	}
	if assert.NotNil(t, credentials.AccessToken) {
		assert.NotEmpty(t, credentials.AccessToken.AccessToken)
		assert.NotEmpty(t, credentials.AccessToken.RefreshToken)
	}
}

func TestAuthorizeSharing(t *testing.T) {
	bobLogin(t)
	fakeAliceInstance(t)

	res, err := bobUA.Get(authorizeLink)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "text/html; charset=UTF-8", res.Header.Get("Content-Type"))
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	assertAuthorizePageShowsTheSharing(t, string(body))

	v := &url.Values{
		"state":      {state},
		"sharing_id": {sharingID},
		"csrf_token": {csrfToken},
	}
	buf := bytes.NewBufferString(v.Encode())
	req, err := http.NewRequest(http.MethodPost, tsB.URL+"/auth/authorize/sharing", buf)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	assert.NoError(t, err)
	res, err = bobUA.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusSeeOther, res.StatusCode)
	location := res.Header.Get("Location")
	assert.Contains(t, location, "testapp."+bobInstance.Domain)

	assertCredentialsHasBeenExchanged(t)
}

func assertSharingByAliceToBobDaveAndEdward(t *testing.T, members []interface{}) {
	assert.Len(t, members, 4)
	owner := members[0].(map[string]interface{})
	assert.Equal(t, owner["status"], "owner")
	assert.Equal(t, owner["public_name"], "Alice")
	assert.Equal(t, owner["email"], "alice@example.net")
	bob := members[1].(map[string]interface{})
	assert.Equal(t, bob["status"], "ready")
	assert.Equal(t, bob["email"], "bob@example.net")
	dave := members[2].(map[string]interface{})
	assert.Equal(t, dave["status"], "pending")
	assert.Equal(t, dave["email"], "dave@example.net")
	assert.Equal(t, dave["read_only"], true)
	edward := members[3].(map[string]interface{})
	assert.Equal(t, edward["status"], "pending")
	assert.Equal(t, edward["name"], "Edward")
	assert.Equal(t, edward["instance"], "https://edward.example.net/")
}

func TestDelegateAddRecipientByCozyURL(t *testing.T) {
	assert.NotEmpty(t, bobAppToken)
	assert.NotNil(t, edwardContact)

	v := echo.Map{
		"data": echo.Map{
			"type": consts.Sharings,
			"relationships": echo.Map{
				"recipients": echo.Map{
					"data": []interface{}{
						echo.Map{
							"id":      edwardContact.ID(),
							"doctype": edwardContact.DocType(),
						},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(v)
	r := bytes.NewReader(body)
	req, err := http.NewRequest(http.MethodPost, tsB.URL+"/sharings/"+sharingID+"/recipients", r)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+bobAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	defer res.Body.Close()

	var result map[string]interface{}
	assert.NoError(t, json.NewDecoder(res.Body).Decode(&result))
	data := result["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	members := attrs["members"].([]interface{})
	assertSharingByAliceToBobDaveAndEdward(t, members)
}

func assertSharingWithPreviewIsCorrect(t *testing.T, body io.Reader) {
	var result map[string]interface{}
	assert.NoError(t, json.NewDecoder(body).Decode(&result))
	data := result["data"].(map[string]interface{})
	assert.Equal(t, data["type"], consts.Sharings)
	sharingID = data["id"].(string)
	assert.NotEmpty(t, sharingID)
	assert.NotEmpty(t, data["meta"].(map[string]interface{})["rev"])
	self := "/sharings/" + sharingID
	assert.Equal(t, data["links"].(map[string]interface{})["self"], self)

	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, attrs["description"], "this is a test with preview")
	assert.Equal(t, attrs["app_slug"], "testapp")
	assert.Equal(t, attrs["preview_path"], "/preview")
	assert.Equal(t, attrs["owner"], true)
	assert.NotEmpty(t, attrs["created_at"])
	assert.NotEmpty(t, attrs["updated_at"])
	assert.Nil(t, attrs["credentials"])

	members := attrs["members"].([]interface{})
	assertSharingByAliceToBobAndDave(t, members)

	rules := attrs["rules"].([]interface{})
	assert.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	assert.Equal(t, rule["title"], "test two")
	assert.Equal(t, rule["doctype"], iocozytests)
	assert.Equal(t, rule["values"], []interface{}{"foobaz"})
}

func TestCreateSharingWithPreview(t *testing.T) {
	assert.NotEmpty(t, aliceAppToken)
	assert.NotNil(t, bobContact)

	v := echo.Map{
		"data": echo.Map{
			"type": consts.Sharings,
			"attributes": echo.Map{
				"description":  "this is a test with preview",
				"preview_path": "/preview",
				"rules": []interface{}{
					echo.Map{
						"title":   "test two",
						"doctype": iocozytests,
						"values":  []string{"foobaz"},
					},
				},
			},
			"relationships": echo.Map{
				"recipients": echo.Map{
					"data": []interface{}{
						echo.Map{
							"id":      bobContact.ID(),
							"doctype": bobContact.DocType(),
						},
					},
				},
				"read_only_recipients": echo.Map{
					"data": []interface{}{
						echo.Map{
							"id":      daveContact.ID(),
							"doctype": daveContact.DocType(),
						},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(v)
	r := bytes.NewReader(body)

	req, err := http.NewRequest(http.MethodPost, tsA.URL+"/sharings/", r)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, res.StatusCode)
	defer res.Body.Close()

	assertSharingWithPreviewIsCorrect(t, res.Body)
	description := assertInvitationMailWasSent(t)
	assert.Equal(t, description, "this is a test with preview")
	assert.Contains(t, discoveryLink, aliceInstance.Domain)
	assert.Contains(t, discoveryLink, "/preview?sharecode=")
}

func assertCorrectRedirection(t *testing.T, body io.Reader) {
	var result map[string]interface{}
	assert.NoError(t, json.NewDecoder(body).Decode(&result))
	redirectURI := result["redirect"]
	assert.NotEmpty(t, redirectURI)
	assert.Contains(t, redirectURI, tsB.URL)
	u, err := url.Parse(redirectURI.(string))
	assert.NoError(t, err)
	assert.Equal(t, u.Path, "/auth/authorize/sharing")
	assert.Equal(t, u.Query()["sharing_id"][0], sharingID)
	assert.NotEmpty(t, u.Query()["state"][0])
}

func TestDiscoveryWithPreview(t *testing.T) {
	parts := strings.Split(tsA.URL, "://")
	u, err := url.Parse(discoveryLink)
	assert.NoError(t, err)
	u.Scheme = parts[0]
	u.Host = parts[1]
	u.Path = "/sharings/" + sharingID + "/discovery"
	sharecode := u.Query()["sharecode"][0]
	u.RawQuery = ""
	v := &url.Values{
		"sharecode": {sharecode},
		"url":       {tsB.URL},
	}
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewBufferString(v.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	assert.NoError(t, err)
	res, err := bobUA.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	assertCorrectRedirection(t, res.Body)
}

func assertSharingByAliceToBobDaveAndCharlie(t *testing.T, members []interface{}) {
	assert.Len(t, members, 4)
	owner := members[0].(map[string]interface{})
	assert.Equal(t, owner["status"], "owner")
	assert.Equal(t, owner["public_name"], "Alice")
	assert.Equal(t, owner["email"], "alice@example.net")
	assert.Equal(t, owner["instance"], "https://"+aliceInstance.Domain)
	bob := members[1].(map[string]interface{})
	assert.Equal(t, bob["status"], "pending")
	assert.Equal(t, bob["name"], "Bob")
	assert.Equal(t, bob["email"], "bob@example.net")
	dave := members[2].(map[string]interface{})
	assert.Equal(t, dave["status"], "pending")
	assert.Equal(t, dave["name"], "Dave")
	assert.Equal(t, dave["email"], "dave@example.net")
	assert.Equal(t, dave["read_only"], true)
	charlie := members[3].(map[string]interface{})
	assert.Equal(t, charlie["status"], "pending")
	assert.Equal(t, charlie["name"], "Charlie")
	assert.Equal(t, charlie["email"], "charlie@example.net")
}

func TestAddRecipient(t *testing.T) {
	assert.NotEmpty(t, aliceAppToken)
	assert.NotNil(t, charlieContact)

	v := echo.Map{
		"data": echo.Map{
			"type": consts.Sharings,
			"relationships": echo.Map{
				"recipients": echo.Map{
					"data": []interface{}{
						echo.Map{
							"id":      charlieContact.ID(),
							"doctype": charlieContact.DocType(),
						},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(v)
	r := bytes.NewReader(body)
	req, err := http.NewRequest(http.MethodPost, tsA.URL+"/sharings/"+sharingID+"/recipients", r)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	defer res.Body.Close()

	var result map[string]interface{}
	assert.NoError(t, json.NewDecoder(res.Body).Decode(&result))
	data := result["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	members := attrs["members"].([]interface{})
	assertSharingByAliceToBobDaveAndCharlie(t, members)
}

func TestRevokedSharingWithPreview(t *testing.T) {
	sharecode := strings.Split(discoveryLink, "=")[1]

	// Assert the link is available (equivalent to doc perms created)
	req2, err := http.NewRequest(http.MethodGet, tsA.URL+"/permissions/self", nil)
	req2.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req2.Header.Add(echo.HeaderAuthorization, "Bearer "+sharecode)
	assert.NoError(t, err)
	res2, err := http.DefaultClient.Do(req2)
	content, _ := ioutil.ReadAll(res2.Body)
	assert.NoError(t, err)
	defer res2.Body.Close()
	assert.Equal(t, http.StatusOK, res2.StatusCode)

	// Adding a new member to the sharing
	newMemberMail := "foo@bar.com"
	data := map[string]interface{}{}
	err = json.Unmarshal(content, &data)
	assert.NoError(t, err)
	d := data["data"].(map[string]interface{})
	a := d["attributes"].(map[string]interface{})
	sourceID := a["source_id"].(string)
	sharingID := strings.Split(sourceID, "/")[1]
	sharingDoc, err := sharing.FindSharing(aliceInstance, sharingID)
	assert.NoError(t, err)

	sharingDoc.AddDelegatedContact(aliceInstance, newMemberMail, "", true)
	perms, err := permission.GetForSharePreview(aliceInstance, sharingID)
	assert.NoError(t, err)
	fooShareCode, err := aliceInstance.CreateShareCode(newMemberMail)
	assert.NoError(t, err)

	// Adding its sharecode
	codes := perms.Codes
	codes[newMemberMail] = fooShareCode
	perms.PatchCodes(codes)
	assert.NoError(t, couchdb.UpdateDoc(aliceInstance, perms))
	assert.NoError(t, couchdb.UpdateDoc(aliceInstance, sharingDoc))

	// Assert he has access to the sharing preview
	req2, err = http.NewRequest(http.MethodGet, tsA.URL+"/permissions/self", nil)
	req2.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req2.Header.Add(echo.HeaderAuthorization, "Bearer "+fooShareCode)
	assert.NoError(t, err)
	res2, err = http.DefaultClient.Do(req2)
	assert.NoError(t, err)
	defer res2.Body.Close()
	assert.Equal(t, http.StatusOK, res2.StatusCode)

	// Now, revoking the fresh user from the sharing
	member, err := sharingDoc.FindMemberBySharecode(aliceInstance, fooShareCode)
	assert.NoError(t, err)
	err = sharingDoc.RevokeMember(aliceInstance, member, &sharing.Credentials{})
	assert.NoError(t, err)
	assert.Equal(t, "revoked", member.Status)

	// Try to get permissions/self, we should get a 400
	req2, err = http.NewRequest(http.MethodGet, tsA.URL+"/permissions/self", nil)
	req2.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req2.Header.Add(echo.HeaderAuthorization, "Bearer "+fooShareCode)
	assert.NoError(t, err)
	res2, err = http.DefaultClient.Do(req2)
	assert.NoError(t, err)
	badRequestContent, err := ioutil.ReadAll(res2.Body)
	assert.NoError(t, err)
	defer res2.Body.Close()
	assert.Equal(t, http.StatusBadRequest, res2.StatusCode)
	assert.Contains(t, string(badRequestContent), "Invalid JWT")
}

func TestCheckPermissions(t *testing.T) {
	v := echo.Map{
		"data": echo.Map{
			"type": consts.Sharings,
			"attributes": echo.Map{
				"description": "this is a test",
				"rules": []interface{}{
					echo.Map{
						"title":   "test one",
						"doctype": iocozytests,
						"values":  []string{"foobar"},
						"add":     "sync",
					},
					echo.Map{
						"title":   "test two",
						"doctype": consts.Contacts,
						"values":  []string{"foobar"},
						"add":     "sync",
					},
				},
			},
			"relationships": echo.Map{
				"recipients": echo.Map{
					"data": []interface{}{
						echo.Map{
							"id":      bobContact.ID(),
							"doctype": bobContact.DocType(),
						},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(v)
	r := bytes.NewReader(body)

	req, err := http.NewRequest(http.MethodPost, tsA.URL+"/sharings/", r)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
	defer res.Body.Close()

	other := &sharing.Sharing{
		Description: "Another sharing",
		Rules: []sharing.Rule{
			{
				Title:   "a directory",
				DocType: consts.Files,
				Values:  []string{"6836cc06-33e9-11e8-8157-dfc1aca099b6"},
			},
		},
	}
	assert.NoError(t, other.BeOwner(aliceInstance, "home"))
	assert.NoError(t, other.AddContact(aliceInstance, bobContact.ID(), false))
	_, err = other.Create(aliceInstance)
	assert.NoError(t, err)

	req, err = http.NewRequest(http.MethodGet, tsA.URL+"/sharings/"+other.ID(), nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
	defer res.Body.Close()
}

func TestCheckSharingInfoByDocType(t *testing.T) {
	sharedDocs1 := []string{"fakeid1", "fakeid2", "fakeid3"}
	sharedDocs2 := []string{"fakeid4", "fakeid5"}
	s1 := createSharing(t, aliceInstance, sharedDocs1)
	s2 := createSharing(t, aliceInstance, sharedDocs2)

	for _, id := range sharedDocs1 {
		sid := iocozytests + "/" + id
		sd, errs := createSharedDoc(aliceInstance, sid, s1.ID())
		assert.NoError(t, errs)
		assert.NotNil(t, sd)
	}
	for _, id := range sharedDocs2 {
		sid := iocozytests + "/" + id
		sd, errs := createSharedDoc(aliceInstance, sid, s2.ID())
		assert.NoError(t, errs)
		assert.NotNil(t, sd)
	}
	req, err := http.NewRequest(http.MethodGet, tsA.URL+"/sharings/doctype/"+iocozytests, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	defer res.Body.Close()

	assertSharingInfoRequestIsCorrect(t, res.Body, s1.ID(), s2.ID())

	req2, err := http.NewRequest(http.MethodGet, tsA.URL+"/sharings/doctype/io.cozy.notyet", nil)
	assert.NoError(t, err)
	req2.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req2.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res2, err := http.DefaultClient.Do(req2)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res2.StatusCode)
	res2.Body.Close()
}

func TestRevokeSharing(t *testing.T) {
	sharedDocs := []string{"mygreatid1", "mygreatid2"}
	sharedRefs := []*sharing.SharedRef{}
	s := createSharing(t, aliceInstance, sharedDocs)
	for _, id := range sharedDocs {
		sid := iocozytests + "/" + id
		sd, errs := createSharedDoc(aliceInstance, sid, s.SID)
		sharedRefs = append(sharedRefs, sd)
		assert.NoError(t, errs)
		assert.NotNil(t, sd)
	}

	cli, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[1])
	assert.NoError(t, err)
	s.Credentials[0].Client = sharing.ConvertOAuthClient(cli)
	token, err := sharing.CreateAccessToken(aliceInstance, cli, s.SID, permission.ALL)
	assert.NoError(t, err)
	s.Credentials[0].AccessToken = token
	s.Members[1].Status = sharing.MemberStatusReady

	err = couchdb.UpdateDoc(aliceInstance, s)
	assert.NoError(t, err)

	err = s.AddTrackTriggers(aliceInstance)
	assert.NoError(t, err)
	err = s.AddReplicateTrigger(aliceInstance)
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, tsA.URL+"/sharings/"+s.ID()+"/recipients", nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 204, res.StatusCode)

	var sRevoke sharing.Sharing
	err = couchdb.GetDoc(aliceInstance, s.DocType(), s.SID, &sRevoke)
	assert.NoError(t, err)

	assert.Equal(t, "", sRevoke.Triggers.TrackID)
	assert.Equal(t, "", sRevoke.Triggers.ReplicateID)
	assert.Equal(t, "", sRevoke.Triggers.UploadID)
	assert.Equal(t, false, sRevoke.Active)

	var sdoc sharing.SharedRef
	err = couchdb.GetDoc(aliceInstance, sharedRefs[0].DocType(), sharedRefs[0].ID(), &sdoc)
	assert.EqualError(t, err, "CouchDB(not_found): deleted")
	err = couchdb.GetDoc(aliceInstance, sharedRefs[1].DocType(), sharedRefs[1].ID(), &sdoc)
	assert.EqualError(t, err, "CouchDB(not_found): deleted")
}

func assertOneRecipientIsRevoked(t *testing.T, s *sharing.Sharing) {
	var sRevoked sharing.Sharing
	err := couchdb.GetDoc(aliceInstance, s.DocType(), s.SID, &sRevoked)
	assert.NoError(t, err)

	assert.Equal(t, sharing.MemberStatusRevoked, sRevoked.Members[1].Status)
	assert.Equal(t, sharing.MemberStatusReady, sRevoked.Members[2].Status)
	assert.NotEmpty(t, sRevoked.Triggers.TrackID)
	assert.NotEmpty(t, sRevoked.Triggers.ReplicateID)
	assert.True(t, sRevoked.Active)
}

func assertLastRecipientIsRevoked(t *testing.T, s *sharing.Sharing, refs []*sharing.SharedRef) {
	var sRevoked sharing.Sharing
	err := couchdb.GetDoc(aliceInstance, s.DocType(), s.SID, &sRevoked)
	assert.NoError(t, err)

	assert.Equal(t, sharing.MemberStatusRevoked, sRevoked.Members[1].Status)
	assert.Equal(t, sharing.MemberStatusRevoked, sRevoked.Members[2].Status)
	assert.Empty(t, sRevoked.Triggers.TrackID)
	assert.Empty(t, sRevoked.Triggers.ReplicateID)
	assert.False(t, sRevoked.Active)

	var sdoc sharing.SharedRef
	err = couchdb.GetDoc(aliceInstance, refs[0].DocType(), refs[0].ID(), &sdoc)
	assert.EqualError(t, err, "CouchDB(not_found): deleted")
	err = couchdb.GetDoc(aliceInstance, refs[1].DocType(), refs[1].ID(), &sdoc)
	assert.EqualError(t, err, "CouchDB(not_found): deleted")
}

func TestRevokeRecipient(t *testing.T) {
	sharedDocs := []string{"mygreatid3", "mygreatid4"}
	sharedRefs := []*sharing.SharedRef{}
	s := createSharing(t, aliceInstance, sharedDocs)
	for _, id := range sharedDocs {
		sid := iocozytests + "/" + id
		sd, errs := createSharedDoc(aliceInstance, sid, s.SID)
		sharedRefs = append(sharedRefs, sd)
		assert.NoError(t, errs)
		assert.NotNil(t, sd)
	}

	cli, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[1])
	assert.NoError(t, err)
	s.Credentials[0].Client = sharing.ConvertOAuthClient(cli)
	token, err := sharing.CreateAccessToken(aliceInstance, cli, s.SID, permission.ALL)
	assert.NoError(t, err)
	s.Credentials[0].AccessToken = token
	s.Members[1].Status = sharing.MemberStatusReady

	s.Members = append(s.Members, sharing.Member{
		Status:   sharing.MemberStatusReady,
		Name:     "Charlie",
		Email:    "charlie@cozy.local",
		Instance: tsB.URL,
	})
	clientC, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[2])
	assert.NoError(t, err)
	tokenC, err := sharing.CreateAccessToken(aliceInstance, clientC, s.SID, permission.ALL)
	assert.NoError(t, err)
	s.Credentials = append(s.Credentials, sharing.Credentials{
		Client:      sharing.ConvertOAuthClient(clientC),
		AccessToken: tokenC,
	})

	err = couchdb.UpdateDoc(aliceInstance, s)
	assert.NoError(t, err)

	err = s.AddTrackTriggers(aliceInstance)
	assert.NoError(t, err)
	err = s.AddReplicateTrigger(aliceInstance)
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, tsA.URL+"/sharings/"+s.ID()+"/recipients/1", nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 204, res.StatusCode)
	assertOneRecipientIsRevoked(t, s)

	req2, err := http.NewRequest(http.MethodDelete, tsA.URL+"/sharings/"+s.ID()+"/recipients/2", nil)
	assert.NoError(t, err)
	req2.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req2.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res2, err := http.DefaultClient.Do(req2)
	assert.NoError(t, err)
	assert.Equal(t, 204, res2.StatusCode)
	assertLastRecipientIsRevoked(t, s, sharedRefs)
}

func TestRevocationFromRecipient(t *testing.T) {
	sharedDocs := []string{"mygreatid5", "mygreatid6"}
	sharedRefs := []*sharing.SharedRef{}
	s := createSharing(t, aliceInstance, sharedDocs)
	for _, id := range sharedDocs {
		sid := iocozytests + "/" + id
		sd, errs := createSharedDoc(aliceInstance, sid, s.SID)
		sharedRefs = append(sharedRefs, sd)
		assert.NoError(t, errs)
		assert.NotNil(t, sd)
	}

	cli, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[1])
	assert.NoError(t, err)
	s.Credentials[0].InboundClientID = cli.ClientID
	s.Credentials[0].Client = sharing.ConvertOAuthClient(cli)
	token, err := sharing.CreateAccessToken(aliceInstance, cli, s.SID, permission.ALL)
	assert.NoError(t, err)
	s.Credentials[0].AccessToken = token
	s.Members[1].Status = sharing.MemberStatusReady

	s.Members = append(s.Members, sharing.Member{
		Status:   sharing.MemberStatusReady,
		Name:     "Charlie",
		Email:    "charlie@cozy.local",
		Instance: tsB.URL,
	})
	clientC, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[2])
	assert.NoError(t, err)
	tokenC, err := sharing.CreateAccessToken(aliceInstance, clientC, s.SID, permission.ALL)
	assert.NoError(t, err)
	s.Credentials = append(s.Credentials, sharing.Credentials{
		Client:          sharing.ConvertOAuthClient(clientC),
		AccessToken:     tokenC,
		InboundClientID: clientC.ClientID,
	})

	err = couchdb.UpdateDoc(aliceInstance, s)
	assert.NoError(t, err)

	err = s.AddTrackTriggers(aliceInstance)
	assert.NoError(t, err)
	err = s.AddReplicateTrigger(aliceInstance)
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, tsA.URL+"/sharings/"+s.ID()+"/answer", nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+s.Credentials[0].AccessToken.AccessToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 204, res.StatusCode)
	assertOneRecipientIsRevoked(t, s)

	req, err = http.NewRequest(http.MethodDelete, tsA.URL+"/sharings/"+s.ID()+"/answer", nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+s.Credentials[1].AccessToken.AccessToken)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 204, res.StatusCode)
	assertLastRecipientIsRevoked(t, s, sharedRefs)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb()
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	// Prepare Alice's instance
	setup := testutils.NewSetup(m, "sharing_test_alice")
	aliceInstance = setup.GetTestInstance(&lifecycle.Options{
		Email:      "alice@example.net",
		PublicName: "Alice",
	})
	aliceAppToken = generateAppToken(aliceInstance, "testapp")
	bobContact = createContact(aliceInstance, "Bob", "bob@example.net")
	charlieContact = createContact(aliceInstance, "Charlie", "charlie@example.net")
	daveContact = createContact(aliceInstance, "Dave", "dave@example.net")
	tsA = setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/sharings":    sharings.Routes,
		"/permissions": permissions.Routes,
	})
	tsA.Config.Handler.(*echo.Echo).Renderer = render
	tsA.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	// Prepare Bob's browser
	jar := setup.GetCookieJar()
	bobUA = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}

	// Prepare Bob's instance
	bobSetup := testutils.NewSetup(m, "sharing_test_bob")
	bobInstance = bobSetup.GetTestInstance(&lifecycle.Options{
		Email:      "bob@example.net",
		PublicName: "Bob",
		Passphrase: "MyPassphrase",
	})
	bobAppToken = generateAppToken(bobInstance, "testapp")
	edwardContact = createContactWithCozyURL(bobInstance, "Edward", "https://edward.example.net/")
	tsB = bobSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth": func(g *echo.Group) {
			g.Use(middlewares.LoadSession)
			auth.Routes(g)
		},
		"/sharings": sharings.Routes,
	})
	tsB.Config.Handler.(*echo.Echo).Renderer = render
	tsB.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	// Prepare another instance for the replicator tests
	replSetup := testutils.NewSetup(m, "sharing_test_replicator")
	replInstance = replSetup.GetTestInstance()
	tsR = replSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/sharings": sharings.Routes,
	})

	setup.AddCleanup(func() error {
		bobSetup.Cleanup()
		replSetup.Cleanup()
		return nil
	})
	os.Exit(setup.Run())
}

func createContact(inst *instance.Instance, name, email string) *contact.Contact {
	mail := map[string]interface{}{"address": email}
	c := contact.New()
	c.M["fullname"] = name
	c.M["email"] = []interface{}{mail}
	err := couchdb.CreateDoc(inst, c)
	if err != nil {
		return nil
	}
	return c
}

func createContactWithCozyURL(inst *instance.Instance, name, instanceURL string) *contact.Contact {
	cozy := map[string]interface{}{"url": instanceURL}
	c := contact.New()
	c.M["fullname"] = name
	c.M["cozy"] = []interface{}{cozy}
	err := couchdb.CreateDoc(inst, c)
	if err != nil {
		return nil
	}
	return c
}

func createSharing(t *testing.T, inst *instance.Instance, values []string) *sharing.Sharing {
	r := sharing.Rule{
		Title:   "test",
		DocType: iocozytests,
		Values:  values,
		Add:     sharing.ActionRuleSync,
	}
	mail, err := bobContact.ToMailAddress()
	assert.NoError(t, err)
	m := sharing.Member{
		Name:     bobContact.Get("fullname").(string),
		Email:    mail.Email,
		Instance: tsB.URL,
	}
	s := &sharing.Sharing{
		Owner: true,
		Rules: []sharing.Rule{r},
	}
	s.Credentials = append(s.Credentials, sharing.Credentials{})
	err = s.BeOwner(aliceInstance, "")
	assert.NoError(t, err)
	s.Members = append(s.Members, m)

	err = couchdb.CreateDoc(inst, s)
	assert.NoError(t, err)
	assert.NotNil(t, s)
	return s
}

func createSharedDoc(inst *instance.Instance, id, sharingID string) (*sharing.SharedRef, error) {
	ref := &sharing.SharedRef{
		SID:       id,
		Revisions: &sharing.RevsTree{Rev: "1-aaa"},
		Infos: map[string]sharing.SharedInfo{
			sharingID: {Rule: 0},
		},
	}
	err := couchdb.CreateNamedDocWithDB(inst, ref)
	if err != nil {
		return nil, err
	}
	return ref, nil
}

func generateAppToken(inst *instance.Instance, slug string) string {
	rules := permission.Set{
		permission.Rule{
			Type:  iocozytests,
			Verbs: permission.ALL,
		},
	}
	permReq := permission.Permission{
		Permissions: rules,
		Type:        permission.TypeWebapp,
		SourceID:    consts.Apps + "/" + slug,
	}
	err := couchdb.CreateDoc(inst, &permReq)
	if err != nil {
		return ""
	}
	manifest := &app.WebappManifest{
		DocID:          consts.Apps + "/" + slug,
		DocSlug:        slug,
		DocPermissions: rules,
	}
	err = couchdb.CreateNamedDocWithDB(inst, manifest)
	if err != nil {
		return ""
	}
	return inst.BuildAppToken(manifest.Slug(), "")
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}
