package permissions

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

var ts *httptest.Server
var token string
var testInstance *instance.Instance
var clientVal *oauth.Client
var clientID string

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	testSetup := testutils.NewSetup(m, "permissions_test")

	testInstance = testSetup.GetTestInstance()
	scopes := "io.cozy.contacts io.cozy.files:GET io.cozy.events"
	client, tok := testSetup.GetTestClient(scopes)
	clientVal = client
	clientID = client.ClientID
	token = tok

	ts = testSetup.GetTestServer("/permissions", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	os.Exit(testSetup.Run())
}

func TestCreateShareSetByMobileRevokeByLinkedApp(t *testing.T) {
	// Create OAuthLinkedClient
	oauthLinkedClient := &oauth.Client{
		ClientName:   "test-linked-shareset",
		RedirectURIs: []string{"https://foobar"},
		SoftwareID:   "registry://drive",
	}
	oauthLinkedClient.Create(testInstance)

	// Install the app
	installer, err := app.NewInstaller(testInstance, testInstance.AppsCopier(consts.WebappType), &app.InstallerOptions{
		Operation:  app.Install,
		Type:       consts.WebappType,
		SourceURL:  "registry://drive",
		Slug:       "drive",
		Registries: testInstance.Registries(),
	})
	assert.NoError(t, err)
	_, err = installer.RunSync()
	assert.NoError(t, err)

	// Generate a token for the client
	tok, err := testInstance.MakeJWT(consts.AccessTokenAudience,
		oauthLinkedClient.ClientID, "@io.cozy.apps/drive", "", time.Now())
	assert.NoError(t, err)

	// Create body
	bodyReq := fmt.Sprintf(`{"data": {"id": "%s","type": "io.cozy.permissions","attributes": {"permissions": {"files": {"type": "io.cozy.files","verbs": ["GET"]}}}}}`, oauthLinkedClient.ClientID)

	// Request to create a permission
	req, err := http.NewRequest("POST", ts.URL+"/permissions?codes=email", strings.NewReader(bodyReq))
	assert.NoError(t, err)
	req.Host = testInstance.Domain
	req.Header.Add("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)

	type minPerms struct {
		Type       string                `json:"type"`
		ID         string                `json:"id"`
		Attributes permission.Permission `json:"attributes"`
	}

	var perm struct {
		Data minPerms `json:"data"`
	}
	err = json.NewDecoder(res.Body).Decode(&perm)
	assert.NoError(t, err)
	// Assert the permission received does not have the clientID as source_id
	assert.NotEqual(t, perm.Data.Attributes.SourceID, oauthLinkedClient.ClientID)

	// Create a webapp token
	webAppToken, err := testInstance.MakeJWT(consts.AppAudience, "drive", "", "", time.Now())
	assert.NoError(t, err)

	// Login to webapp and try to delete the shared link
	delReq, err := http.NewRequest("DELETE", ts.URL+"/permissions/"+perm.Data.ID, nil)
	delReq.Host = testInstance.Domain
	delReq.Header.Add("Authorization", "Bearer "+webAppToken)
	assert.NoError(t, err)
	delRes, err := http.DefaultClient.Do(delReq)
	assert.NoError(t, err)
	assert.Equal(t, 204, delRes.StatusCode)

	// Cleaning
	oauthLinkedClient, err = oauth.FindClientBySoftwareID(testInstance, "registry://drive")
	assert.NoError(t, err)
	oauthLinkedClient.Delete(testInstance)

	uninstaller, err := app.NewInstaller(testInstance, testInstance.AppsCopier(consts.WebappType),
		&app.InstallerOptions{
			Operation:  app.Delete,
			Type:       consts.WebappType,
			Slug:       "drive",
			SourceURL:  "registry://drive",
			Registries: testInstance.Registries(),
		},
	)
	assert.NoError(t, err)

	_, err = uninstaller.RunSync()
	assert.NoError(t, err)
}

func TestCreateShareSetByLinkedAppRevokeByMobile(t *testing.T) {
	// Create a webapp token
	webAppToken, err := testInstance.MakeJWT(consts.AppAudience, "drive", "", "", time.Now())
	assert.NoError(t, err)

	// Install the app
	installer, err := app.NewInstaller(testInstance, testInstance.AppsCopier(consts.WebappType), &app.InstallerOptions{
		Operation:  app.Install,
		Type:       consts.WebappType,
		SourceURL:  "registry://drive",
		Slug:       "drive",
		Registries: testInstance.Registries(),
	})
	assert.NoError(t, err)
	_, err = installer.RunSync()
	assert.NoError(t, err)

	// Create body
	bodyReq := fmt.Sprintf(`{"data": {"id": "%s","type": "io.cozy.permissions","attributes": {"permissions": {"files": {"type": "io.cozy.files","verbs": ["GET"]}}}}}`, "io.cozy.apps/drive")

	// Request to create a permission
	req, err := http.NewRequest("POST", ts.URL+"/permissions?codes=email", strings.NewReader(bodyReq))
	assert.NoError(t, err)
	req.Host = testInstance.Domain
	req.Header.Add("Authorization", "Bearer "+webAppToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)

	type minPerms struct {
		Type       string                `json:"type"`
		ID         string                `json:"id"`
		Attributes permission.Permission `json:"attributes"`
	}

	var perm struct {
		Data minPerms `json:"data"`
	}
	err = json.NewDecoder(res.Body).Decode(&perm)
	assert.NoError(t, err)

	// // Create OAuthLinkedClient
	oauthLinkedClient := &oauth.Client{
		ClientName:   "test-linked-shareset2",
		RedirectURIs: []string{"https://foobar"},
		SoftwareID:   "registry://drive",
	}
	oauthLinkedClient.Create(testInstance)

	// // Generate a token for the client
	tok, err := testInstance.MakeJWT(consts.AccessTokenAudience,
		oauthLinkedClient.ClientID, "@io.cozy.apps/drive", "", time.Now())
	assert.NoError(t, err)

	// // Assert the permission received does not have the clientID as source_id
	// assert.NotEqual(t, perm.Data.Attributes.SourceID, oauthLinkedClient.ClientID)

	// // Login to webapp and try to delete the shared link
	delReq, err := http.NewRequest("DELETE", ts.URL+"/permissions/"+perm.Data.ID, nil)
	delReq.Host = testInstance.Domain
	delReq.Header.Add("Authorization", "Bearer "+tok)
	assert.NoError(t, err)
	delRes, err := http.DefaultClient.Do(delReq)
	assert.NoError(t, err)
	assert.Equal(t, 204, delRes.StatusCode)

	// Cleaning
	oauthLinkedClient, err = oauth.FindClientBySoftwareID(testInstance, "registry://drive")
	assert.NoError(t, err)
	oauthLinkedClient.Delete(testInstance)

	uninstaller, err := app.NewInstaller(testInstance, testInstance.AppsCopier(consts.WebappType),
		&app.InstallerOptions{
			Operation:  app.Delete,
			Type:       consts.WebappType,
			Slug:       "drive",
			SourceURL:  "registry://drive",
			Registries: testInstance.Registries(),
		},
	)
	assert.NoError(t, err)

	_, err = uninstaller.RunSync()
	assert.NoError(t, err)
}

func TestGetPermissions(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/permissions/self", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if !assert.NoError(t, err) {
		return
	}
	var out map[string]interface{}
	err = json.Unmarshal(body, &out)
	assert.NoError(t, err)

	data := out["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	perms := attrs["permissions"].(map[string]interface{})

	for key, r := range perms {
		rule := r.(map[string]interface{})
		if key == "rule1" {
			assert.Equal(t, "io.cozy.files", rule["type"])
			assert.Equal(t, []interface{}{"GET"}, rule["verbs"])
		} else if key == "rule0" {
			assert.Equal(t, "io.cozy.contacts", rule["type"])
		} else {
			assert.Equal(t, "io.cozy.events", rule["type"])
		}
	}
}

func TestGetPermissionsForRevokedClient(t *testing.T) {
	tok, err := testInstance.MakeJWT(consts.AccessTokenAudience,
		"revoked-client",
		"io.cozy.contacts io.cozy.files:GET",
		"", time.Now())
	assert.NoError(t, err)
	req, _ := http.NewRequest("GET", ts.URL+"/permissions/self", nil)
	req.Header.Add("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
	body, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.Equal(t, `Invalid JWT token`, string(body))
}

func TestGetPermissionsForExpiredToken(t *testing.T) {
	pastTimestamp := time.Now().Add(-30 * 24 * time.Hour) // in seconds
	tok, err := testInstance.MakeJWT(consts.AccessTokenAudience,
		clientID, "io.cozy.contacts io.cozy.files:GET", "", pastTimestamp)
	assert.NoError(t, err)
	req, _ := http.NewRequest("GET", ts.URL+"/permissions/self", nil)
	req.Header.Add("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
	body, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.Equal(t, `Expired token`, string(body))
}

func TestBadPermissionsBearer(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/permissions/self", nil)
	req.Header.Add("Authorization", "Bearer garbage")
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	defer res.Body.Close()
	assert.Equal(t, res.StatusCode, http.StatusBadRequest)
}

func TestCreateSubPermission(t *testing.T) {
	_, codes, err := createTestSubPermissions(token, "alice,bob")
	if !assert.NoError(t, err) {
		return
	}

	aCode := codes["alice"].(string)
	bCode := codes["bob"].(string)

	assert.NotEqual(t, aCode, token)
	assert.NotEqual(t, bCode, token)
	assert.NotEqual(t, aCode, bCode)

	req, _ := http.NewRequest("GET", ts.URL+"/permissions/self", nil)
	req.Header.Add("Authorization", "Bearer "+aCode)
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if !assert.NoError(t, err) {
		return
	}
	var out map[string]interface{}
	err = json.Unmarshal(body, &out)

	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	data := out["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	perms := attrs["permissions"].(map[string]interface{})
	assert.Len(t, perms, 2)
	assert.Equal(t, "io.cozy.files", perms["whatever"].(map[string]interface{})["type"])
}

func TestCreateSubSubFail(t *testing.T) {
	_, codes, err := createTestSubPermissions(token, "eve")
	if !assert.NoError(t, err) {
		return
	}
	eveCode := codes["eve"].(string)
	_, _, err = createTestSubPermissions(eveCode, "eve")
	if !assert.Error(t, err) {
		return
	}
}

func TestPatchNoopFail(t *testing.T) {
	id, _, err := createTestSubPermissions(token, "pierre")
	if !assert.NoError(t, err) {
		return
	}

	_, err = doRequest("PATCH", ts.URL+"/permissions/"+id, token, `{
	  "data": {
	    "id": "a340d5e0-d647-11e6-b66c-5fc9ce1e17c6",
	    "type": "io.cozy.permissions",
	    "attributes": { }
	    }
	  }
	}
`)

	assert.Error(t, err)
}

func TestBadPatchAddRuleForbidden(t *testing.T) {
	id, _, err := createTestSubPermissions(token, "jacque")
	if !assert.NoError(t, err) {
		return
	}

	_, err = doRequest("PATCH", ts.URL+"/permissions/"+id, token, `{
	  "data": {
	    "attributes": {
					"permissions": {
						"otherperm": {
							"type":"io.cozy.token-cant-do-this"
						}
					}
				}
	    }
	  }
`)

	assert.Error(t, err)
}

func TestPatchAddRule(t *testing.T) {
	id, _, err := createTestSubPermissions(token, "paul")
	if !assert.NoError(t, err) {
		return
	}

	out, err := doRequest("PATCH", ts.URL+"/permissions/"+id, token, `{
	  "data": {
	    "attributes": {
					"permissions": {
						"otherperm": {
							"type":"io.cozy.contacts"
						}
					}
				}
	    }
	  }
`)

	data := out["data"].(map[string]interface{})
	assert.Equal(t, id, data["id"])
	attrs := data["attributes"].(map[string]interface{})
	perms := attrs["permissions"].(map[string]interface{})

	assert.NoError(t, err)
	assert.Len(t, perms, 3)
	assert.Equal(t, "io.cozy.files", perms["whatever"].(map[string]interface{})["type"])
	assert.Equal(t, "io.cozy.contacts", perms["otherperm"].(map[string]interface{})["type"])
}

func TestPatchRemoveRule(t *testing.T) {
	id, _, err := createTestSubPermissions(token, "paul")
	if !assert.NoError(t, err) {
		return
	}

	out, err := doRequest("PATCH", ts.URL+"/permissions/"+id, token, `{
	  "data": {
	    "attributes": {
					"permissions": {
						"otherrule": { }
					}
				}
	    }
	  }
`)

	data := out["data"].(map[string]interface{})
	assert.Equal(t, id, data["id"])
	attrs := data["attributes"].(map[string]interface{})
	perms := attrs["permissions"].(map[string]interface{})

	assert.NoError(t, err)
	assert.Len(t, perms, 1)
	assert.Equal(t, "io.cozy.files", perms["whatever"].(map[string]interface{})["type"])
}

func TestPatchChangesCodes(t *testing.T) {
	id, codes, err := createTestSubPermissions(token, "john,jane")
	if !assert.NoError(t, err) {
		return
	}

	assert.NotEmpty(t, codes["john"])
	janeToken := codes["jane"].(string)
	assert.NotEmpty(t, janeToken)

	_, err = doRequest("PATCH", ts.URL+"/permissions/"+id, janeToken, `{
		"data": {
			"attributes": {
					"codes": {
						"john": "set-token"
					}
				}
			}
		}
`)
	assert.Error(t, err)

	out, err := doRequest("PATCH", ts.URL+"/permissions/"+id, token, `{
	  "data": {
	    "attributes": {
					"codes": {
						"john": "set-token"
					}
				}
	    }
	  }
`)

	if !assert.NoError(t, err) {
		return
	}
	data := out["data"].(map[string]interface{})
	assert.Equal(t, id, data["id"])
	attrs := data["attributes"].(map[string]interface{})
	newcodes := attrs["codes"].(map[string]interface{})
	assert.NotEmpty(t, newcodes["john"])
	assert.Nil(t, newcodes["jane"])

}

func TestRevoke(t *testing.T) {
	id, codes, err := createTestSubPermissions(token, "igor")
	if !assert.NoError(t, err) {
		return
	}

	igorToken := codes["igor"].(string)
	assert.NotEmpty(t, igorToken)

	_, err = doRequest("DELETE", ts.URL+"/permissions/"+id, igorToken, "")
	assert.Error(t, err)

	out, err := doRequest("DELETE", ts.URL+"/permissions/"+id, token, "")
	assert.NoError(t, err)
	assert.Nil(t, out)

}

func createTestSubPermissions(tok string, codes string) (string, map[string]interface{}, error) {
	out, err := doRequest("POST", ts.URL+"/permissions?codes="+codes, tok, `{
"data": {
	"type": "io.cozy.permissions",
	"attributes": {
		"permissions": {
			"whatever": {
				"type":   "io.cozy.files",
				"verbs":  ["GET"],
				"values": ["io.cozy.music"]
			},
			"otherrule": {
				"type":   "io.cozy.files",
				"verbs":  ["GET"],
				"values":  ["some-other-dir"]
		  }
		}
	}
}
	}`)

	if err != nil {
		return "", nil, err
	}

	data := out["data"].(map[string]interface{})
	id := data["id"].(string)
	attrs := data["attributes"].(map[string]interface{})
	result := attrs["codes"].(map[string]interface{})
	return id, result, nil
}

func doRequest(method, url, tok, body string) (map[string]interface{}, error) {
	reqbody := strings.NewReader(body)
	req, _ := http.NewRequest(method, url, reqbody)
	req.Header.Add("Authorization", "Bearer "+tok)
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	resbody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if len(resbody) == 0 {
		return nil, nil
	}
	if res.StatusCode/200 != 1 {
		return nil, fmt.Errorf("Bad request: %d", res.StatusCode)
	}
	var out map[string]interface{}
	err = json.Unmarshal(resbody, &out)
	if err != nil {
		return nil, err
	}

	if errstr := out["message"]; errstr != nil {
		return nil, fmt.Errorf("%s", errstr)
	}

	return out, nil
}

func TestGetPermissionsWithShortCode(t *testing.T) {
	id, _, _ := createTestSubPermissions(token, "daniel")
	perm, _ := permission.GetByID(testInstance, id)

	assert.NotNil(t, perm.ShortCodes)

	req1, _ := http.NewRequest("GET", ts.URL+"/permissions/self", nil)
	req1.Header.Add("Authorization", "Bearer "+perm.ShortCodes["daniel"])
	res1, _ := http.DefaultClient.Do(req1)
	assert.Equal(t, res1.StatusCode, http.StatusOK)
}

func TestGetPermissionsWithBadShortCode(t *testing.T) {
	id, _, _ := createTestSubPermissions(token, "alice")
	perm, _ := permission.GetByID(testInstance, id)

	assert.NotNil(t, perm.ShortCodes)

	req1, _ := http.NewRequest("GET", ts.URL+"/permissions/self", nil)
	req1.Header.Add("Authorization", "Bearer "+"foobar")
	res1, _ := http.DefaultClient.Do(req1)
	assert.Equal(t, res1.StatusCode, http.StatusBadRequest)
}

func TestGetTokenFromShortCode(t *testing.T) {
	id, _, _ := createTestSubPermissions(token, "alice")
	perm, _ := permission.GetByID(testInstance, id)

	token, _ := permission.GetTokenFromShortcode(testInstance, perm.ShortCodes["alice"])
	assert.Equal(t, perm.Codes["alice"], token)
}

func TestGetBadShortCode(t *testing.T) {
	_, _, err := createTestSubPermissions(token, "alice")
	assert.NoError(t, err)
	shortcode := "coincoin"

	token, err := permission.GetTokenFromShortcode(testInstance, shortcode)
	assert.Empty(t, token)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "no permission doc for shortcode")
}

func TestGetMultipleShortCode(t *testing.T) {
	id, _, _ := createTestSubPermissions(token, "alice")
	id2, _, _ := createTestSubPermissions(token, "alice")
	perm, _ := permission.GetByID(testInstance, id)
	perm2, _ := permission.GetByID(testInstance, id2)

	perm2.ShortCodes["alice"] = perm.ShortCodes["alice"]
	assert.NoError(t, couchdb.UpdateDoc(testInstance, perm2))

	_, err := permission.GetTokenFromShortcode(testInstance, perm.ShortCodes["alice"])

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "several permission docs for shortcode")
}

func TestCannotFindToken(t *testing.T) {
	id, _, _ := createTestSubPermissions(token, "alice")
	perm, _ := permission.GetByID(testInstance, id)
	perm.Codes = map[string]string{}
	assert.NoError(t, couchdb.UpdateDoc(testInstance, perm))

	_, err := permission.GetTokenFromShortcode(testInstance, perm.ShortCodes["alice"])
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "Cannot find token for shortcode")
}

func TestGetForOauth(t *testing.T) {
	// Install app
	installer, err := app.NewInstaller(testInstance, testInstance.AppsCopier(consts.WebappType), &app.InstallerOptions{
		Operation:  app.Install,
		Type:       consts.WebappType,
		SourceURL:  "registry://settings",
		Slug:       "settings",
		Registries: testInstance.Registries(),
	})
	assert.NoError(t, err)
	installer.Run()

	// Get app manifest
	manifest, err := app.GetBySlug(testInstance, "settings", consts.WebappType)
	assert.NoError(t, err)

	// Create OAuth client
	var oauthClient oauth.Client

	u := "https://example.org/oauth/callback"

	oauthClient.RedirectURIs = []string{u}
	oauthClient.ClientName = "cozy-test-2"
	oauthClient.SoftwareID = "registry://settings"
	oauthClient.Create(testInstance)

	parent, err := middlewares.GetForOauth(testInstance, &permission.Claims{
		StandardClaims: jwt.StandardClaims{
			Audience: consts.AccessTokenAudience,
			Issuer:   testInstance.Domain,
			IssuedAt: crypto.Timestamp(),
			Subject:  clientID,
		},
		Scope: "@io.cozy.apps/settings",
	}, oauthClient)
	assert.NoError(t, err)
	assert.True(t, parent.Permissions.HasSameRules(manifest.Permissions()))
}

func TestListPermission(t *testing.T) {

	ev1, _ := createTestEvent(testInstance)
	ev2, _ := createTestEvent(testInstance)
	ev3, _ := createTestEvent(testInstance)

	parent, _ := middlewares.GetForOauth(testInstance, &permission.Claims{
		StandardClaims: jwt.StandardClaims{
			Audience: consts.AccessTokenAudience,
			Issuer:   testInstance.Domain,
			IssuedAt: crypto.Timestamp(),
			Subject:  clientID,
		},
		Scope: "io.cozy.events",
	}, clientVal)
	p1 := permission.Set{
		permission.Rule{
			Type:   "io.cozy.events",
			Verbs:  permission.Verbs(permission.DELETE, permission.PATCH),
			Values: []string{ev1.ID()},
		}}
	p2 := permission.Set{
		permission.Rule{
			Type:   "io.cozy.events",
			Verbs:  permission.Verbs(permission.GET),
			Values: []string{ev2.ID()},
		}}

	codes := map[string]string{"bob": "secret"}
	_, _ = permission.CreateShareSet(testInstance, parent, parent.SourceID, codes, nil, p1, nil)
	_, _ = permission.CreateShareSet(testInstance, parent, parent.SourceID, codes, nil, p2, nil)

	reqbody := strings.NewReader(`{
"data": [
{ "type": "io.cozy.events", "id": "` + ev1.ID() + `" },
{ "type": "io.cozy.events", "id": "` + ev2.ID() + `" },
{ "type": "io.cozy.events", "id": "non-existing-id" },
{ "type": "io.cozy.events", "id": "another-fake-id" },
{ "type": "io.cozy.events", "id": "` + ev3.ID() + `" }
]	}`)

	req, _ := http.NewRequest("POST", ts.URL+"/permissions/exists", reqbody)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if !assert.NoError(t, err) {
		return
	}
	var out jsonapi.Document
	err = json.Unmarshal(body, &out)
	if !assert.NoError(t, err) {
		return
	}
	var results []refAndVerb
	err = json.Unmarshal(*out.Data, &results)
	if !assert.NoError(t, err) {
		return
	}

	assert.Len(t, results, 2)
	for _, result := range results {
		assert.Equal(t, "io.cozy.events", result.DocType)
		if result.ID == ev1.ID() {
			assert.Equal(t, "PATCH,DELETE", result.Verbs.String())
		} else {
			assert.Equal(t, ev2.ID(), result.ID)
			assert.Equal(t, "GET", result.Verbs.String())
		}
	}

	req2, _ := http.NewRequest("GET", ts.URL+"/permissions/doctype/io.cozy.events/shared-by-link", nil)
	req2.Header.Add("Authorization", "Bearer "+token)
	req2.Header.Add("Content-Type", "application/json")
	res2, err := http.DefaultClient.Do(req2)
	if !assert.NoError(t, err) {
		return
	}
	defer res2.Body.Close()

	var resBody struct {
		Data []map[string]interface{}
	}
	err = json.NewDecoder(res2.Body).Decode(&resBody)
	assert.NoError(t, err)
	assert.Len(t, resBody.Data, 2)
	assert.NotEqual(t, resBody.Data[0]["id"], resBody.Data[1]["id"])

	req3, _ := http.NewRequest("GET", ts.URL+"/permissions/doctype/io.cozy.events/shared-by-link?page[limit]=1", nil)
	req3.Header.Add("Authorization", "Bearer "+token)
	req3.Header.Add("Content-Type", "application/json")
	res3, err := http.DefaultClient.Do(req3)
	if !assert.NoError(t, err) {
		return
	}
	defer res3.Body.Close()

	var resBody3 struct {
		Data  []interface{}
		Links *jsonapi.LinksList
	}
	err = json.NewDecoder(res3.Body).Decode(&resBody3)
	assert.NoError(t, err)
	assert.Len(t, resBody3.Data, 1)
	assert.NotEmpty(t, resBody3.Links.Next)

}

func createTestEvent(i *instance.Instance) (*couchdb.JSONDoc, error) {
	e := &couchdb.JSONDoc{
		Type: "io.cozy.events",
		M:    map[string]interface{}{"test": "value"},
	}
	err := couchdb.CreateDoc(i, e)
	return e, err
}
