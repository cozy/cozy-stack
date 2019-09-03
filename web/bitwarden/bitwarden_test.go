package bitwarden

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var inst *instance.Instance
var token string

func TestPrelogin(t *testing.T) {
	body := `{ "email": "me@cozy.example.net" }`
	req, _ := http.NewRequest("POST", ts.URL+"/bitwarden/api/accounts/prelogin", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]int
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	assert.Equal(t, 0, result["Kdf"])
	assert.Equal(t, crypto.DefaultPBKDF2Iterations, result["KdfIterations"])
}

func TestConnect(t *testing.T) {
	email := inst.PassphraseSalt()
	iter := crypto.DefaultPBKDF2Iterations
	pass, _ := crypto.HashPassWithPBKDF2([]byte("cozy"), email, iter)
	v := url.Values{
		"grant_type": {"password"},
		"username":   {string(email)},
		"password":   {string(pass)},
		"scope":      {"api offline_access"},
		"client_id":  {"browser"},
		"deviceType": {"3"},
	}
	res, err := http.PostForm(ts.URL+"/bitwarden/identity/connect/token", v)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	expiresIn := consts.AccessTokenValidityDuration.Seconds()
	assert.Equal(t, "Bearer", result["token_type"])
	assert.Equal(t, expiresIn, result["expires_in"])
	if assert.NotEmpty(t, result["access_token"]) {
		token = result["access_token"].(string)
	}
	assert.NotEmpty(t, result["refresh_token"])
	assert.NotEmpty(t, result["Key"])
}

func TestCreateFolder(t *testing.T) {
	body := `{ "name": "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o=" }`
	req, _ := http.NewRequest("POST", ts.URL+"/bitwarden/api/folders", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]string
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	assert.Equal(t, "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o=", result["Name"])
	assert.Equal(t, "folder", result["Object"])
	assert.NotEmpty(t, result["Id"])
	assert.NotEmpty(t, result["RevisionDate"])
}

func TestChangeSecurityHash(t *testing.T) {
	email := inst.PassphraseSalt()
	iter := crypto.DefaultPBKDF2Iterations
	pass, _ := crypto.HashPassWithPBKDF2([]byte("cozy"), email, iter)
	body, _ := json.Marshal(map[string]string{
		"masterPasswordHash": string(pass),
	})
	buf := bytes.NewBuffer(body)
	res, err := http.Post(ts.URL+"/bitwarden/api/accounts/security-stamp", "application/json", buf)
	assert.NoError(t, err)
	assert.Equal(t, 204, res.StatusCode)
	// TODO check that token is no longer valid
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "bitwarden_test")
	inst = setup.GetTestInstance(&lifecycle.Options{
		Domain:     "cozy.example.net",
		Passphrase: "cozy",
	})

	ts = setup.GetTestServer("/bitwarden", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	os.Exit(setup.Run())
}
