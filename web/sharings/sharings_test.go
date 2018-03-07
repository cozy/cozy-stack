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
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/sharings"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

const iocozytests = "io.cozy.tests"

var setup *testutils.TestSetup

// Things that live on Alice's Cozy
var ts *httptest.Server
var aliceInstance *instance.Instance
var aliceAppToken string
var bobContact *contacts.Contact
var sharingLink string

// Things that live on Bob's browser
var bobUA *http.Client

func assertSharingIsCorrectOnSharer(t *testing.T, body io.Reader) {
	var result map[string]interface{}
	assert.NoError(t, json.NewDecoder(body).Decode(&result))
	data := result["data"].(map[string]interface{})
	assert.Equal(t, data["type"], consts.Sharings)
	sid := data["id"].(string)
	assert.NotEmpty(t, sid)
	assert.NotEmpty(t, data["meta"].(map[string]interface{})["rev"])
	self := "/sharings/" + sid
	assert.Equal(t, data["links"].(map[string]interface{})["self"], self)

	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, attrs["description"], "this is a test")
	assert.Equal(t, attrs["app_slug"], "testapp")
	assert.Equal(t, attrs["owner"], true)
	assert.NotEmpty(t, attrs["created_at"])
	assert.NotEmpty(t, attrs["updated_at"])
	assert.Nil(t, attrs["credentials"])

	members := attrs["members"].([]interface{})
	assert.Len(t, members, 2)
	owner := members[0].(map[string]interface{})
	assert.Equal(t, owner["status"], "owner")
	assert.Equal(t, owner["name"], "Alice")
	assert.Equal(t, owner["email"], "alice@example.net")
	assert.Equal(t, owner["instance"], aliceInstance.Domain)
	recipient := members[1].(map[string]interface{})
	assert.Equal(t, recipient["status"], "mail-not-sent")
	assert.Equal(t, recipient["name"], "Bob")
	assert.Equal(t, recipient["email"], "bob@example.net")

	rules := attrs["rules"].([]interface{})
	assert.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	assert.Equal(t, rule["title"], "test one")
	assert.Equal(t, rule["doctype"], iocozytests)
	assert.Equal(t, rule["values"], []interface{}{"foobar"})
}

func assertInvitationMailWasSent(t *testing.T) {
	var jobs []jobs.Job
	couchReq := &couchdb.FindRequest{
		UseIndex: "by-worker-and-state",
		Selector: mango.And(
			mango.Equal("worker", "sendmail"),
			mango.Exists("state"),
		),
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
	assert.Equal(t, values["Description"], "this is a test")
	sharingLink = values["SharingLink"].(string)
	assert.Contains(t, sharingLink, "/discovery?state=")
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

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/sharings/", r)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+aliceAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, res.StatusCode)
	defer res.Body.Close()

	assertSharingIsCorrectOnSharer(t, res.Body)
	assertInvitationMailWasSent(t)
}

func TestDiscovery(t *testing.T) {
	parts := strings.Split(ts.URL, "://")
	u, err := url.Parse(sharingLink)
	assert.NoError(t, err)
	u.Scheme = parts[0]
	u.Host = parts[1]
	state := u.Query()["state"][0]
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	assert.NoError(t, err)
	res, err := bobUA.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "Happy to see you Bob!")
	assert.Contains(t, string(body), "Please enter your Cozy URL to receive the sharing from Alice")
	assert.Contains(t, string(body), `<input id="url" name="url"`)
	assert.Contains(t, string(body), `<input type="hidden" name="state" value="`+state)

	u.RawQuery = ""
	v := &url.Values{
		"state": {state},
		"url":   {"https://bob.example.net/"},
	}
	req, err = http.NewRequest(http.MethodPost, u.String(), bytes.NewBufferString(v.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	assert.NoError(t, err)
	res, err = bobUA.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusFound, res.StatusCode)
	assert.Equal(t, "/auth/sharing", res.Header.Get("Location"))
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	web.LoadSupportedLocales()
	testutils.NeedCouchdb()

	// Prepare Alice's instance
	setup = testutils.NewSetup(m, "sharing_test_alice")
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

	// Prepare Bob's browser
	jar := setup.GetCookieJar()
	bobUA = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}

	ts = setup.GetTestServer("/sharings", sharings.Routes)
	r, _ := statik.NewDirRenderer("../../assets")
	ts.Config.Handler.(*echo.Echo).Renderer = r
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
