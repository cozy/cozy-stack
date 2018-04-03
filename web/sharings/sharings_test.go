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

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharing"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/sharings"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

const iocozytests = "io.cozy.tests"

// Things that live on Alice's Cozy
var tsA *httptest.Server
var aliceInstance *instance.Instance
var aliceAppToken string
var bobContact *contacts.Contact
var sharingID, state, aliceAccessToken string

// Things that live on Bob's Cozy
var tsB *httptest.Server
var bobInstance *instance.Instance

// Bob's browser
var bobUA *http.Client
var discoveryLink, authorizeLink string
var csrfToken string

func assertSharingByAliceToBob(t *testing.T, members []interface{}) {
	assert.Len(t, members, 2)
	owner := members[0].(map[string]interface{})
	assert.Equal(t, owner["status"], "owner")
	assert.Equal(t, owner["name"], "Alice")
	assert.Equal(t, owner["email"], "alice@example.net")
	assert.Equal(t, owner["instance"], "https://"+aliceInstance.Domain)
	recipient := members[1].(map[string]interface{})
	assert.Equal(t, recipient["status"], "mail-not-sent")
	assert.Equal(t, recipient["name"], "Bob")
	assert.Equal(t, recipient["email"], "bob@example.net")
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
	assertSharingByAliceToBob(t, members)

	rules := attrs["rules"].([]interface{})
	assert.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	assert.Equal(t, rule["title"], "test one")
	assert.Equal(t, rule["doctype"], iocozytests)
	assert.Equal(t, rule["values"], []interface{}{"foobar"})
}

func assertInvitationMailWasSent(t *testing.T) string {
	var jobs []jobs.Job
	couchReq := &couchdb.FindRequest{
		UseIndex: "by-worker-and-state",
		Selector: mango.And(
			mango.Equal("worker", "sendmail"),
			mango.Exists("state"),
		),
		Sort: mango.SortBy{
			mango.SortByField{Field: "worker", Direction: "desc"},
		},
		Limit: 1,
	}
	err := couchdb.FindDocs(aliceInstance, consts.Jobs, couchReq, &jobs)
	assert.NoError(t, err)
	assert.Len(t, jobs, 1)
	var msg map[string]interface{}
	err = json.Unmarshal(jobs[0].Message, &msg)
	assert.NoError(t, err)
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
				"description": "this is a test",
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

	assert.Len(t, s.Members, 2)
	owner := s.Members[0]
	assert.Equal(t, owner.Status, "owner")
	assert.Equal(t, owner.Name, "Alice")
	assert.Equal(t, owner.Email, "alice@example.net")
	assert.Equal(t, owner.Instance, "https://"+aliceInstance.Domain)
	recipient := s.Members[1]
	assert.Equal(t, recipient.Status, "mail-not-sent")
	assert.Equal(t, recipient.Name, "Bob")
	assert.Equal(t, recipient.Email, "bob@example.net")
	assert.Equal(t, recipient.Instance, tsB.URL)

	assert.Len(t, s.Rules, 1)
	rule := s.Rules[0]
	assert.Equal(t, rule.Title, "test one")
	assert.Equal(t, rule.DocType, iocozytests)
	assert.Equal(t, rule.Values, []string{"foobar"})
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
	assert.Contains(t, string(body), "Happy to see you Bob!")
	assert.Contains(t, string(body), "Please enter your Cozy URL to receive the sharing from Alice")
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
	assert.Contains(t, res.Header.Get("Location"), "drive")
}

func fakeAliceInstance(t *testing.T) {
	var results []*sharing.Sharing
	req := couchdb.AllDocsRequest{}
	err := couchdb.GetAllDocs(bobInstance, consts.Sharings, &req, &results)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	s := results[0]
	assert.Len(t, s.Members, 2)
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
	assert.Len(t, sharingsA[0].Credentials, 1)
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
	assert.Contains(t, location, "drive."+bobInstance.Domain)

	assertCredentialsHasBeenExchanged(t)
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
	assertSharingByAliceToBob(t, members)

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
	assert.NoError(t, other.BeOwner(aliceInstance, "drive"))
	assert.NoError(t, other.AddContact(aliceInstance, bobContact.ID()))
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

func TestMain(m *testing.M) {
	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	web.LoadSupportedLocales()
	testutils.NeedCouchdb()
	render, _ := statik.NewDirRenderer("../../assets")

	// Prepare Alice's instance
	setup := testutils.NewSetup(m, "sharing_test_alice")
	var settings couchdb.JSONDoc
	settings.M = map[string]interface{}{
		"email":       "alice@example.net",
		"public_name": "Alice",
	}
	aliceInstance = setup.GetTestInstance(&instance.Options{
		Settings: settings,
	})
	aliceAppToken = generateAppToken(aliceInstance, "testapp")
	bobContact = createContact(aliceInstance, "Bob", "bob@example.net")
	tsA = setup.GetTestServer("/sharings", sharings.Routes)
	tsA.Config.Handler.(*echo.Echo).Renderer = render

	// Prepare Bob's browser
	jar := setup.GetCookieJar()
	bobUA = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}

	// Prepare Bob's instance
	bobSetup := testutils.NewSetup(m, "sharing_test_bob")
	var settingsB couchdb.JSONDoc
	settingsB.M = map[string]interface{}{
		"email":       "bob@example.net",
		"public_name": "Bob",
	}
	bobInstance = bobSetup.GetTestInstance(&instance.Options{
		Settings: settingsB,
	})
	bobInstance.RegisterPassphrase([]byte("MyPassphrase"), bobInstance.RegisterToken)
	bobInstance.OnboardingFinished = true
	if err := instance.Update(bobInstance); err != nil {
		testutils.Fatal(err)
	}
	tsB = bobSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth": func(g *echo.Group) {
			g.Use(middlewares.LoadSession)
			auth.Routes(g)
		},
		"/sharings": sharings.Routes,
	})
	tsB.Config.Handler.(*echo.Echo).Renderer = render

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

func createContact(inst *instance.Instance, name, email string) *contacts.Contact {
	c := &contacts.Contact{
		FullName: name,
		Email: []contacts.Email{
			contacts.Email{Address: email},
		},
	}
	err := couchdb.CreateDoc(inst, c)
	if err != nil {
		return nil
	}
	return c
}

func generateAppToken(inst *instance.Instance, slug string) string {
	rules := permissions.Set{
		permissions.Rule{
			Type:  iocozytests,
			Verbs: permissions.ALL,
		},
	}
	permReq := permissions.Permission{
		Permissions: rules,
		Type:        permissions.TypeWebapp,
		SourceID:    consts.Apps + "/" + slug,
	}
	err := couchdb.CreateDoc(inst, &permReq)
	if err != nil {
		return ""
	}
	manifest := &apps.WebappManifest{
		DocSlug:        slug,
		DocPermissions: rules,
	}
	err = couchdb.CreateNamedDocWithDB(inst, manifest)
	if err != nil {
		return ""
	}
	return inst.BuildAppToken(manifest, "")
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}
