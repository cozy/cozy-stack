package settings

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"

	_ "github.com/cozy/cozy-stack/worker/mails"
)

var ts *httptest.Server
var tsB *httptest.Server
var testInstance *instance.Instance
var instanceRev string
var token string
var oauthClientID string

func TestPatchWithGoodRev(t *testing.T) {
	// We are going to patch an instance with newer values, and give the good rev
	body := `{
		"data": {
			"type": "io.cozy.settings",
			"id": "io.cozy.settings.instance",
			"meta": {
				"rev": "%s"
			},
			"attributes": {
				"tz": "Europe/London",
				"email": "alice@example.org",
				"locale": "fr"
			}
		}
	}`
	body = fmt.Sprintf(body, instanceRev)
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestPatchWithBadRev(t *testing.T) {
	// We are going to patch an instance with newer values, but with a totally
	// random rev
	rev := "6-2d9b7ef014d10549c2b4e206672d3e44"
	body := `{
		"data": {
			"type": "io.cozy.settings",
			"id": "io.cozy.settings.instance",
			"meta": {
				"rev": "%s"
			},
			"attributes": {
				"tz": "Europe/London",
				"email": "alice@example.org",
				"locale": "fr"
			}
		}
	}`
	body = fmt.Sprintf(body, rev)
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, res.StatusCode)
}

func TestPatchWithBadRevNoChanges(t *testing.T) {
	// We are defining a random rev, but make no changes in the instance values
	rev := "6-2d9b7ef014d10549c2b4e206672d3e44"
	body := `{
		"data": {
			"type": "io.cozy.settings",
			"id": "io.cozy.settings.instance",
			"meta": {
				"rev": "%s"
			},
			"attributes": {
				"tz": "Europe/Berlin",
				"email": "alice@example.com",
				"locale": "en"
			}
		}
	}`
	body = fmt.Sprintf(body, rev)
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestPatchWithBadRevAndChanges(t *testing.T) {
	// We are defining a random rev, but make changes in the instance values
	rev := "6-2d9b7ef014d10549c2b4e206672d3e44"
	body := `{
			"data": {
				"type": "io.cozy.settings",
				"id": "io.cozy.settings.instance",
				"meta": {
					"rev": "%s"
				},
				"attributes": {
					"tz": "Europe/London",
					"email": "alice@example.com",
					"locale": "en"
				}
			}
		}`
	body = fmt.Sprintf(body, rev)
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, res.StatusCode)
}
func TestDiskUsage(t *testing.T) {
	res, err := http.Get(ts.URL + "/settings/disk-usage")
	assert.NoError(t, err)
	assert.Equal(t, 401, res.StatusCode)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/settings/disk-usage", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	data, ok := result["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "io.cozy.settings", data["type"].(string))
	assert.Equal(t, "io.cozy.settings.disk-usage", data["id"].(string))
	attrs, ok := data["attributes"].(map[string]interface{})
	assert.True(t, ok)
	used, ok := attrs["used"].(string)
	assert.True(t, ok)
	assert.Equal(t, "0", used)
}

func TestRegisterPassphraseWrongToken(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"passphrase":     "MyFirstPassphrase",
		"register_token": "BADBEEF",
	})
	res1, err := http.Post(ts.URL+"/settings/passphrase", "application/json", bytes.NewReader(args))
	assert.NoError(t, err)
	defer res1.Body.Close()
	assert.Equal(t, "400 Bad Request", res1.Status)

	args, _ = json.Marshal(&echo.Map{
		"passphrase":     "MyFirstPassphrase",
		"register_token": "XYZ",
	})
	res2, err := http.Post(ts.URL+"/settings/passphrase", "application/json", bytes.NewReader(args))
	assert.NoError(t, err)
	defer res2.Body.Close()
	assert.Equal(t, "400 Bad Request", res2.Status)
}

func TestRegisterPassphraseCorrectToken(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"passphrase":     "MyFirstPassphrase",
		"register_token": hex.EncodeToString(testInstance.RegisterToken),
	})
	res, err := http.Post(ts.URL+"/settings/passphrase", "application/json", bytes.NewReader(args))
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "204 No Content", res.Status)
	cookies := res.Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, cookies[0].Name, session.SessionCookieName)
	assert.NotEmpty(t, cookies[0].Value)
}

func TestUpdatePassphraseWithWrongPassphrase(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"new_passphrase":     "MyPassphrase",
		"current_passphrase": "BADBEEF",
	})
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/passphrase", bytes.NewReader(args))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
}

func TestUpdatePassphraseSuccess(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"new_passphrase":     "MyPassphrase",
		"current_passphrase": "MyFirstPassphrase",
	})
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/passphrase", bytes.NewReader(args))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "204 No Content", res.Status)
	cookies := res.Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, cookies[0].Name, session.SessionCookieName)
	assert.NotEmpty(t, cookies[0].Value)
}

func TestGetInstance(t *testing.T) {
	res, err := http.Get(ts.URL + "/settings/instance")
	assert.NoError(t, err)
	assert.Equal(t, 401, res.StatusCode)

	testInstance.RegisterToken = []byte("test")
	res, err = http.Get(ts.URL + "/settings/instance?registerToken=74657374")
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	testInstance.RegisterToken = []byte{}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/settings/instance", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	data, ok := result["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "io.cozy.settings", data["type"].(string))
	assert.Equal(t, "io.cozy.settings.instance", data["id"].(string))
	meta, ok := data["meta"].(map[string]interface{})
	assert.True(t, ok)
	instanceRev = meta["rev"].(string)
	assert.NotEmpty(t, instanceRev)
	attrs, ok := data["attributes"].(map[string]interface{})
	assert.True(t, ok)
	email, ok := attrs["email"].(string)
	assert.True(t, ok)
	assert.Equal(t, "alice@example.com", email)
	tz, ok := attrs["tz"].(string)
	assert.True(t, ok)
	assert.Equal(t, "Europe/Berlin", tz)
	locale, ok := attrs["locale"].(string)
	assert.True(t, ok)
	assert.Equal(t, "en", locale)
}

func TestUpdateInstance(t *testing.T) {
	var newRev string

	checkResult := func(res *http.Response) {
		assert.Equal(t, 200, res.StatusCode)
		var result map[string]interface{}
		err := json.NewDecoder(res.Body).Decode(&result)
		assert.NoError(t, err)
		data, ok := result["data"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "io.cozy.settings", data["type"].(string))
		assert.Equal(t, "io.cozy.settings.instance", data["id"].(string))
		meta, ok := data["meta"].(map[string]interface{})
		assert.True(t, ok)
		rev := meta["rev"].(string)
		assert.NotEmpty(t, rev)
		assert.NotEqual(t, instanceRev, rev)
		newRev = rev
		attrs, ok := data["attributes"].(map[string]interface{})
		assert.True(t, ok)
		email, ok := attrs["email"].(string)
		assert.True(t, ok)
		assert.Equal(t, "alice@example.org", email)
		tz, ok := attrs["tz"].(string)
		assert.True(t, ok)
		assert.Equal(t, "Europe/London", tz)
		locale, ok := attrs["locale"].(string)
		assert.True(t, ok)
		assert.Equal(t, "fr", locale)
	}

	body := `{
		"data": {
			"type": "io.cozy.settings",
			"id": "io.cozy.settings.instance",
			"meta": {
				"rev": "%s"
			},
			"attributes": {
				"tz": "Europe/London",
				"email": "alice@example.org",
				"locale": "fr"
			}
		}
	}`
	body = fmt.Sprintf(body, instanceRev)
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	checkResult(res)

	req, _ = http.NewRequest("GET", ts.URL+"/settings/instance", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	checkResult(res)

	instanceRev = newRev
}

func TestUpdatePassphraseWithTwoFactorAuth(t *testing.T) {
	body := `{
		"auth_mode": "two_factor_mail"
	}`
	body = fmt.Sprintf(body, instanceRev)
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance/auth_mode", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	if !assert.Equal(t, "204 No Content", res.Status) {
		return
	}

	mailPassCode, err := testInstance.GenerateMailConfirmationCode()
	assert.NoError(t, err)
	body = `{
		"auth_mode": "two_factor_mail",
		"two_factor_activation_code": "%s"
	}`
	body = fmt.Sprintf(body, mailPassCode)
	req, _ = http.NewRequest("PUT", ts.URL+"/settings/instance/auth_mode", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	if !assert.Equal(t, "204 No Content", res.Status) {
		return
	}

	args, _ := json.Marshal(&echo.Map{
		"current_passphrase": "MyPassphrase",
	})
	req, _ = http.NewRequest("PUT", ts.URL+"/settings/passphrase", bytes.NewReader(args))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "200 OK", res.Status)

	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)

	{
		twoFactorToken, ok := result["two_factor_token"].(string)
		assert.True(t, ok)
		assert.NotEmpty(t, twoFactorToken)
	}

	twoFactorToken, twoFactorPasscode, err := testInstance.GenerateTwoFactorSecrets()
	assert.NoError(t, err)

	args, _ = json.Marshal(&echo.Map{
		"new_passphrase":      "MyLastPassphrase",
		"two_factor_token":    twoFactorToken,
		"two_factor_passcode": twoFactorPasscode,
	})
	req, _ = http.NewRequest("PUT", ts.URL+"/settings/passphrase", bytes.NewReader(args))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "204 No Content", res.Status)
}

func TestListClients(t *testing.T) {
	res, err := http.Get(ts.URL + "/settings/clients")
	assert.NoError(t, err)
	assert.Equal(t, 401, res.StatusCode)

	client := &oauth.Client{
		RedirectURIs:    []string{"http:/localhost:4000/oauth/callback"},
		ClientName:      "Cozy-desktop on my-new-laptop",
		ClientKind:      "desktop",
		ClientURI:       "https://docs.cozy.io/en/mobile/desktop.html",
		LogoURI:         "https://docs.cozy.io/assets/images/cozy-logo-docs.svg",
		PolicyURI:       "https://cozy.io/policy",
		SoftwareID:      "/github.com/cozy-labs/cozy-desktop",
		SoftwareVersion: "0.16.0",
	}
	regErr := client.Create(testInstance)
	assert.Nil(t, regErr)
	oauthClientID = client.ClientID

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/settings/clients", nil)
	assert.NoError(t, err)
	req.Header.Add("Authorization", "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	data := result["data"].([]interface{})
	assert.Len(t, data, 2)
	obj := data[1].(map[string]interface{})
	assert.Equal(t, "io.cozy.oauth.clients", obj["type"].(string))
	assert.Equal(t, client.ClientID, obj["id"].(string))
	links := obj["links"].(map[string]interface{})
	self := links["self"].(string)
	assert.Equal(t, "/settings/clients/"+client.ClientID, self)
	attrs := obj["attributes"].(map[string]interface{})
	redirectURIs := attrs["redirect_uris"].([]interface{})
	assert.Len(t, redirectURIs, 1)
	assert.Equal(t, client.RedirectURIs[0], redirectURIs[0].(string))
	assert.Equal(t, client.ClientName, attrs["client_name"].(string))
	assert.Equal(t, client.ClientKind, attrs["client_kind"].(string))
	assert.Equal(t, client.ClientURI, attrs["client_uri"].(string))
	assert.Equal(t, client.LogoURI, attrs["logo_uri"].(string))
	assert.Equal(t, client.PolicyURI, attrs["policy_uri"].(string))
	assert.Equal(t, client.SoftwareID, attrs["software_id"].(string))
	assert.Equal(t, client.SoftwareVersion, attrs["software_version"].(string))
	assert.Nil(t, attrs["client_secret"])
}

func TestRevokeClient(t *testing.T) {
	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/settings/clients/"+oauthClientID, nil)
	assert.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 401, res.StatusCode)

	req, err = http.NewRequest(http.MethodDelete, ts.URL+"/settings/clients/"+oauthClientID, nil)
	assert.NoError(t, err)
	req.Header.Add("Authorization", "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 204, res.StatusCode)

	req, err = http.NewRequest(http.MethodGet, ts.URL+"/settings/clients", nil)
	assert.NoError(t, err)
	req.Header.Add("Authorization", "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	data := result["data"].([]interface{})
	assert.Len(t, data, 1)
}

func TestRedirectOnboardingSecret(t *testing.T) {
	url := tsB.URL + "/settings/onboarded"

	// Disable redirections
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	// Without onboarding
	res, err := client.Get(url)
	assert.NoError(t, err)
	assert.Equal(t, res.StatusCode, http.StatusSeeOther)
	loc, _ := res.Location()
	assert.Equal(t, loc.String(), testInstance.OnboardedRedirection().String())

	// With onboarding
	deeplink := "cozydrive://testinstance.com"
	oauthClient := &oauth.Client{
		RedirectURIs:     []string{deeplink},
		ClientName:       "CozyTest",
		SoftwareID:       "/github.com/cozy-labs/cozy-desktop",
		OnboardingSecret: "foobar",
		OnboardingApp:    "test",
	}

	oauthClient.Create(testInstance)
	res, err = client.Get(url)
	assert.NoError(t, err)
	assert.Equal(t, res.StatusCode, http.StatusSeeOther)

	loc, _ = res.Location()
	assert.NotEqual(t, loc.String(), testInstance.OnboardedRedirection().String())
	assert.Contains(t, loc.String(), "/auth/authorize")

	values := loc.Query()
	assert.Equal(t, values.Get("redirect_uri"), deeplink)
}

func TestPatchInstanceSameParams(t *testing.T) {
	doc1, err := testInstance.SettingsDocument()
	assert.NoError(t, err)

	body := `{
			"data": {
				"type": "io.cozy.settings",
				"id": "io.cozy.settings.instance",
				"meta": {
					"rev": "%s"
				},
				"attributes": {
					"tz": "Europe/London",
					"email": "alice@example.org",
					"locale": "en"
				}
			}
		}`
	body = fmt.Sprintf(body, doc1.Rev())
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	content, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.NotEmpty(t, content)

	doc2, err := testInstance.SettingsDocument()
	assert.NoError(t, err)
	// Assert no changes
	assert.Equal(t, doc1.Rev(), doc2.Rev())
}
func TestPatchInstanceChangeParams(t *testing.T) {
	doc, err := testInstance.SettingsDocument()
	assert.NoError(t, err)

	body := `{
			"data": {
				"type": "io.cozy.settings",
				"id": "io.cozy.settings.instance",
				"meta": {
					"rev": "%s"
				},
				"attributes": {
					"tz": "Antarctica/McMurdo",
					"email": "alice@expat.eu",
					"locale": "de"
				}
			}
		}`
	body = fmt.Sprintf(body, doc.Rev())
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	content, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.NotEmpty(t, content)

	doc, err = testInstance.SettingsDocument()
	assert.NoError(t, err)

	assert.Equal(t, "Antarctica/McMurdo", doc.M["tz"].(string))
	assert.Equal(t, "alice@expat.eu", doc.M["email"].(string))
}

func TestPatchInstanceAddParam(t *testing.T) {
	doc1, err := testInstance.SettingsDocument()
	assert.NoError(t, err)

	body := `{
			"data": {
				"type": "io.cozy.settings",
				"id": "io.cozy.settings.instance",
				"meta": {
					"rev": "%s"
				},
				"attributes": {
					"tz": "Europe/Berlin",
					"email": "alice@example.com",
					"how_old_are_you": "42"
				}
			}
		}`
	body = fmt.Sprintf(body, doc1.Rev())
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	content, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.NotEmpty(t, content)

	doc2, err := testInstance.SettingsDocument()
	assert.NoError(t, err)
	assert.NotEqual(t, doc1.Rev(), doc2.Rev())
	assert.Equal(t, "42", doc2.M["how_old_are_you"].(string))
	assert.Equal(t, "Europe/Berlin", doc2.M["tz"].(string))
	assert.Equal(t, "alice@example.com", doc2.M["email"].(string))
}
func TestPatchInstanceRemoveParams(t *testing.T) {
	doc1, err := testInstance.SettingsDocument()
	assert.NoError(t, err)

	body := `{
			"data": {
				"type": "io.cozy.settings",
				"id": "io.cozy.settings.instance",
				"meta": {
					"rev": "%s"
				},
				"attributes": {
					"tz": "Europe/Berlin"
				}
			}
		}`
	body = fmt.Sprintf(body, doc1.Rev())
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	content, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.NotEmpty(t, content)

	doc2, err := testInstance.SettingsDocument()
	assert.NoError(t, err)
	assert.NotEqual(t, doc1.Rev(), doc2.Rev())
	assert.Equal(t, "Europe/Berlin", doc2.M["tz"].(string))
	_, ok := doc2.M["email"]
	assert.False(t, ok)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "settings_test")
	testInstance = setup.GetTestInstance(&lifecycle.Options{
		Locale:   "en",
		Timezone: "Europe/Berlin",
		Email:    "alice@example.com",
	})
	scope := consts.Settings + " " + consts.OAuthClients
	_, token = setup.GetTestClient(scope)

	ts = setup.GetTestServer("/settings", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	tsB = setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth": func(g *echo.Group) {
			g.Use(fakeAuthentication)
			auth.Routes(g)
		},
		"/settings": func(g *echo.Group) {
			g.Use(fakeAuthentication)
			Routes(g)
		},
	})
	tsB.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	os.Exit(setup.Run())
}

// Fake middleware used to inject a false session to mislead the authentication
// system
func fakeAuthentication(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := c.Get("instance").(*instance.Instance)
		sess, _ := session.New(instance, true)
		c.Set("session", sess)
		return next(c)
	}
}
