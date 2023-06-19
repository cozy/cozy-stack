package bitwarden

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	_ "github.com/cozy/cozy-stack/worker/mails"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBitwarden(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var token, collID, orgaID, folderID, cipherID string

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance(&lifecycle.Options{
		Domain:     "bitwarden.example.net",
		Passphrase: "cozy",
		PublicName: "Pierre",
		Email:      "pierre@cozy.localhost",
	})

	ts := setup.GetTestServer("/bitwarden", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	t.Run("Prelogin", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/bitwarden/api/accounts/prelogin").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{ "email": "me@bitwarden.example.net" }`)).
			Expect().Status(http.StatusOK).
			JSON().Object()

		obj.ValueEqual("Kdf", 0)
		obj.ValueEqual("OIDC", false)
		obj.ValueEqual("KdfIterations", crypto.DefaultPBKDF2Iterations)
		obj.ValueEqual("HasCiphers", false)
	})

	t.Run("Connect", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		testLogger := test.NewGlobal()
		setting, err := settings.Get(inst)
		assert.NoError(t, err)
		setting.EncryptedOrgKey = ""
		err = setting.Save(inst)
		assert.NoError(t, err)

		email := inst.PassphraseSalt()
		iter := crypto.DefaultPBKDF2Iterations
		pass, _ := crypto.HashPassWithPBKDF2([]byte("cozy"), email, iter)

		obj := e.POST("/bitwarden/identity/connect/token").
			WithFormField("grant_type", "password").
			WithFormField("username", string(email)).
			WithFormField("password", string(pass)).
			WithFormField("scope", "api offline_access").
			WithFormField("client_id", "browser").
			WithFormField("deviceType", "3").
			Expect().
			Status(http.StatusOK).
			JSON().Object()

		obj.ValueEqual("token_type", "Bearer")
		obj.ValueEqual("expires_in", consts.AccessTokenValidityDuration.Seconds())
		token = obj.Value("access_token").String().NotEmpty().Raw()

		obj.Value("refresh_token").String().NotEmpty()
		obj.Value("Key").String().NotEmpty()
		obj.Value("PrivateKey").String().NotEmpty()
		obj.Value("client_id").String().NotEmpty()
		obj.Value("registration_access_token").String().NotEmpty()
		obj.Value("Kdf").Number()
		obj.Value("KdfIterations").Number()

		assert.NotZero(t, len(testLogger.Entries))
		orgKeyDoesNotExist := false
		for _, entry := range testLogger.Entries {
			if entry.Message == "Organization key does not exist" {
				orgKeyDoesNotExist = true
			}
		}
		assert.True(t, orgKeyDoesNotExist)

		setting, err = settings.Get(inst)
		assert.NoError(t, err)
		orgKey, err := setting.OrganizationKey()
		assert.NoError(t, err)
		assert.NotEmpty(t, orgKey)
	})

	t.Run("GetCozyOrg", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/bitwarden/organizations/cozy").
			WithHeader("Authorization", "Bearer invalid-token").
			Expect().Status(401)

		obj := e.GET("/bitwarden/organizations/cozy").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON().Object()

		orgaID = obj.Value("organizationId").String().NotEmpty().Raw()
		collID = obj.Value("collectionId").String().NotEmpty().Raw()
		orgKey := obj.Value("organizationKey").String().NotEmpty().Raw()

		_, err := base64.StdEncoding.DecodeString(orgKey)
		assert.NoError(t, err)
	})

	t.Run("CreateFolder", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/bitwarden/api/folders").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{ "name": "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o=" }`)).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("Name", "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o=")
		obj.ValueEqual("Object", "folder")
		obj.Value("RevisionDate").String().DateTime(time.RFC3339)

		folderID = obj.Value("Id").String().NotEmpty().Raw()
	})

	t.Run("ListFolders", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/bitwarden/api/folders").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("Object", "list")
		obj.Value("Data").Array().Length().Equal(1)

		item := obj.Value("Data").Array().First().Object()
		item.ValueEqual("Name", "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o=")
		item.ValueEqual("Object", "folder")
		item.ValueEqual("Id", folderID)
		item.Value("RevisionDate").String().DateTime(time.RFC3339)
	})

	t.Run("GetFolder", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/bitwarden/api/folders/"+folderID).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("Name", "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o=")
		obj.ValueEqual("Object", "folder")
		obj.ValueEqual("Id", folderID)
		obj.Value("RevisionDate").String().DateTime(time.RFC3339)
	})

	t.Run("RenameFolder", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.PUT("/bitwarden/api/folders/"+folderID).
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{ "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=" }`)).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("Name", "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=")
		obj.ValueEqual("Object", "folder")
		obj.ValueEqual("Id", folderID)
		obj.Value("RevisionDate").String().DateTime(time.RFC3339)
	})

	t.Run("DeleteFolder", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/bitwarden/api/folders").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{ "name": "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o=" }`)).
			Expect().Status(200).
			JSON().Object()

		id := obj.Value("Id").String().NotEmpty().Raw()

		obj = e.POST("/bitwarden/api/ciphers").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
      "type": 1,
      "favorite": false,
      "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
      "notes": null,
      "folderId": "` + id + `",
      "organizationId": null,
      "login": {
        "uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
        "username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
        "password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
        "totp": null
      }
    }`)).
			Expect().Status(200).
			JSON().Object()

		cID := obj.Value("Id").String().NotEmpty().Raw()

		e.DELETE("/bitwarden/api/folders/"+id).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200)

		// Check that the cipher in this folder has been moved out
		obj = e.GET("/bitwarden/api/ciphers/"+cID).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("Name", "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=")
		obj.Value("FolderId").Null() // is empty

		e.DELETE("/bitwarden/api/ciphers/"+cID).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200)
	})

	t.Run("CreateNoType", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/bitwarden/api/ciphers").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
      "name": "2.G38TIU3t1pGOfkzjCQE7OQ==|Xa1RupttU7zrWdzIT6oK+w==|J3C6qU1xDrfTgyJD+OrDri1GjgGhU2nmRK75FbZHXoI=",
      "organizationId": null
    }`)).
			Expect().Status(400).
			JSON().Object()

		obj.Value("error").String().NotEmpty()
	})

	t.Run("CreateLogin", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/bitwarden/api/ciphers").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
      "type": 1,
      "favorite": false,
      "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
      "notes": null,
      "folderId": null,
      "organizationId": null,
      "login": {
        "uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
        "username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
        "password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
        "passwordRevisionDate": "2019-09-13T12:26:42+02:00",
        "totp": null
      }
    }`)).
			Expect().Status(200).
			JSON().Object()

		assertCipherResponse(t, obj)

		obj.Value("OrganizationId").Null()
		cipherID = obj.Value("Id").String().NotEmpty().Raw()
	})

	t.Run("ListCiphers", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/bitwarden/api/ciphers").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("Object", "list")

		data := obj.Value("Data").Array()
		data.Length().Equal(1)

		assertCipherResponse(t, data.First().Object())

		data.First().Object().Value("OrganizationId").Null()
	})

	t.Run("GetCipher", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/bitwarden/api/ciphers/"+cipherID).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON().Object()

		assertCipherResponse(t, obj)

		obj.Value("OrganizationId").Null()
	})

	t.Run("UpdateCipher", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.PUT("/bitwarden/api/ciphers/"+cipherID).
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
      "type": 2,
      "favorite": true,
      "name": "2.G38TIU3t1pGOfkzjCQE7OQ==|Xa1RupttU7zrWdzIT6oK+w==|J3C6qU1xDrfTgyJD+OrDri1GjgGhU2nmRK75FbZHXoI=",
      "folderId": "` + folderID + `",
      "organizationId": null,
      "notes": "2.rSw0uVQEFgUCEmOQx0JnDg==|MKqHLD25aqaXYHeYJPH/mor7l3EeSQKsI7A/R+0bFTI=|ODcUScISzKaZWHlUe4MRGuTT2S7jpyDmbOHl7d+6HiM=",
      "secureNote": {
        "type": 0
      }
    }`)).
			Expect().Status(200).
			JSON().Object()

		assertUpdatedCipherResponse(t, obj, cipherID, folderID)

		obj.Value("OrganizationId").Null()
	})

	t.Run("DeleteCipher", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/bitwarden/api/ciphers").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
      "type": 1,
      "favorite": false,
      "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
      "notes": null,
      "folderId": null,
      "organizationId": null,
      "login": {
        "uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
        "username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
        "password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
        "totp": null
      }
    }`)).
			Expect().Status(200).
			JSON().Object()

		id := obj.Value("Id").String().NotEmpty().Raw()

		e.DELETE("/bitwarden/api/ciphers/"+id).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200)
	})

	t.Run("SoftDeleteCipher", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/bitwarden/api/ciphers").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
      "type": 1,
      "favorite": false,
      "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
      "notes": null,
      "folderId": null,
      "organizationId": null,
      "login": {
        "uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
        "username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
        "password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
        "totp": null
      }
    }`)).
			Expect().Status(200).
			JSON().Object()

		id := obj.Value("Id").String().NotEmpty().Raw()

		e.PUT("/bitwarden/api/ciphers/"+id+"/delete").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200)

		obj = e.GET("/bitwarden/api/ciphers/"+id).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON().Object()

		obj.Value("DeletedDate").String().NotEmpty().DateTime(time.RFC3339)
	})

	t.Run("RestoreCipher", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST(ts.URL+"/bitwarden/api/ciphers").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
        "type": 1,
        "favorite": false,
        "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
        "notes": null,
        "folderId": null,
        "organizationId": null,
        "login": {
          "uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
          "username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
          "password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
          "totp": null
        }
      }`)).
			Expect().Status(200).
			JSON().Object()

		id := obj.Value("Id").String().NotEmpty().Raw()

		e.PUT(ts.URL+"/bitwarden/api/ciphers/"+id+"/delete").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200)

		e.PUT(ts.URL+"/bitwarden/api/ciphers/"+id+"/restore").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200)

		obj = e.GET(ts.URL+"/bitwarden/api/ciphers/"+id).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON().Object()

		obj.NotContainsKey("DeletedDate")
	})

	t.Run("Sync", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/bitwarden/api/sync").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("Object", "sync")

		profile := obj.Value("Profile").Object()
		profile.Value("Id").NotNull()
		profile.ValueEqual("Name", "Pierre")
		profile.ValueEqual("Email", "me@bitwarden.example.net")
		profile.ValueEqual("EmailVerified", false)
		profile.ValueEqual("Premium", true)
		profile.ValueEqual("MasterPasswordHint", nil)
		profile.ValueEqual("Culture", "en")
		profile.ValueEqual("TwoFactorEnabled", false)
		profile.Value("Key").NotNull()
		profile.Value("PrivateKey").NotNull()
		profile.Value("SecurityStamp").NotNull()
		profile.ValueEqual("Object", "profile")

		ciphers := obj.Value("Ciphers").Array()
		ciphers.Length().Equal(3)
		assertUpdatedCipherResponse(t, ciphers.First().Object(), cipherID, folderID)

		folders := obj.Value("Folders").Array()
		folders.Length().Equal(1)
		folders.First().Object().ValueEqual("Name", "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=")
		folders.First().Object().ValueEqual("Object", "folder")
		folders.First().Object().Value("RevisionDate").String().NotEmpty().DateTime(time.RFC3339)
		folders.First().Object().ValueEqual("Id", folderID)

		domains := obj.Value("Domains").Object()
		domains.Value("EquivalentDomains").Null()
		domains.Value("GlobalEquivalentDomains").Array().NotEmpty()
		domains.ValueEqual("Object", "domains")
	})

	t.Run("BulkDeleteCiphers", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Setup
		nbCiphersToDelete := 5
		nbCiphers, err := couchdb.CountAllDocs(inst, consts.BitwardenCiphers)
		require.NoError(t, err)

		var ids []string
		for i := 0; i < nbCiphersToDelete; i++ {
			obj := e.POST("/bitwarden/api/ciphers").
				WithHeader("Content-Type", "application/json").
				WithHeader("Authorization", "Bearer "+token).
				WithBytes([]byte(`{
        "type": 1,
        "favorite": false,
        "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
        "notes": null,
        "folderId": null,
        "organizationId": null,
        "login": {
          "uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
          "username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
          "password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
          "totp": null
        }
      }`)).
				Expect().Status(200).
				JSON().Object()

			ids = append(ids, obj.Value("Id").String().NotEmpty().Raw())
		}

		nb, err := couchdb.CountAllDocs(inst, consts.BitwardenCiphers)
		assert.NoError(t, err)
		assert.Equal(t, nbCiphers+nbCiphersToDelete, nb)

		body, _ := json.Marshal(map[string][]string{"ids": ids})

		// Test soft delete in bulk
		t.Run("Soft delete in bulk", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.PUT("/bitwarden/api/ciphers/delete").
				WithHeader("Content-Type", "application/json").
				WithHeader("Authorization", "Bearer "+token).
				WithBytes(body).
				Expect().Status(200)

			for _, id := range ids {
				obj := e.GET("/bitwarden/api/ciphers/"+id).
					WithHeader("Authorization", "Bearer "+token).
					Expect().Status(200).
					JSON().Object()

				obj.Value("DeletedDate").String().NotEmpty().DateTime(time.RFC3339)
			}
		})

		t.Run("Restore in bulk", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.PUT("/bitwarden/api/ciphers/restore").
				WithHeader("Content-Type", "application/json").
				WithHeader("Authorization", "Bearer "+token).
				WithBytes(body).
				Expect().Status(200).
				JSON().Object()

			obj.ValueEqual("Object", "list")
			data := obj.Value("Data").Array()
			data.Length().Equal(nbCiphersToDelete)

			for i, item := range data.Iter() {
				item.Object().ValueEqual("Id", ids[i])
				item.Object().NotContainsKey("DeletedDate")
			}
		})

		t.Run("Delete in bulk", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.DELETE("/bitwarden/api/ciphers").
				WithHeader("Content-Type", "application/json").
				WithHeader("Authorization", "Bearer "+token).
				WithBytes(body).
				Expect().Status(200)

			nb, err = couchdb.CountAllDocs(inst, consts.BitwardenCiphers)
			assert.NoError(t, err)
			assert.Equal(t, nbCiphers, nb)
		})
	})

	t.Run("SharedCipher", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/bitwarden/api/ciphers/create").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
        "cipher": {
          "type": 1,
          "favorite": false,
          "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
          "notes": null,
          "folderId": null,
          "organizationId": "` + orgaID + `",
          "login": {
            "uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
            "username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
            "password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
            "passwordRevisionDate": "2019-09-13T12:26:42+02:00",
            "totp": null
          }
        },
        "collectionIds": ["` + collID + `"]
      }`)).
			Expect().Status(200).
			JSON().Object()

		assertCipherResponse(t, obj)
		obj.ValueEqual("OrganizationId", orgaID)
		cipherID := obj.Value("Id").String().NotEmpty().Raw()

		obj = e.PUT("/bitwarden/api/ciphers/"+cipherID).
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
        "type": 2,
        "favorite": true,
        "name": "2.G38TIU3t1pGOfkzjCQE7OQ==|Xa1RupttU7zrWdzIT6oK+w==|J3C6qU1xDrfTgyJD+OrDri1GjgGhU2nmRK75FbZHXoI=",
        "folderId": "` + folderID + `",
        "organizationId": "` + orgaID + `",
        "notes": "2.rSw0uVQEFgUCEmOQx0JnDg==|MKqHLD25aqaXYHeYJPH/mor7l3EeSQKsI7A/R+0bFTI=|ODcUScISzKaZWHlUe4MRGuTT2S7jpyDmbOHl7d+6HiM=",
        "secureNote": {
          "type": 0
        }
      }`)).
			Expect().Status(200).
			JSON().Object()

		assertUpdatedCipherResponse(t, obj, cipherID, folderID)
		obj.ValueEqual("OrganizationId", orgaID)
	})

	t.Run("SetKeyPair", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Needs to be marshaled in order to avoid encoding issues
		body, _ := json.Marshal(map[string]string{
			"encryptedPrivateKey": "2.demXNYbv8o47sG+fhYYvhg==|jXpxet7AApeIzrC3Yr752LwmjBdCZn6HJl6SjEOVP3rrOpGu5qV2rN0dBH5yXXWHusfxM7IvXkdC/fzBUAmFFOU5ubTp9kHFBqIn51tiJG6BRs5aTm7kF6TYSHVDIP5kUdX4O7DcmD23dqtq/8211DSAFR/DK1QDm5Da77Clh7NHxQE9Z9RTW1PBGV56DfzrY3N06H6vI+V6fTZ6HJRD2pdPczR2ZNC0ziQP7qCUYNlSjEv70O4VoYMSUsdb4UUE1YetcSdZ+dIAy+V2KHfoHmTFYI4DtMCW6WpDzp0ufPvszFjt1EwaMr78hujMrQr1gFWxgN8kOLJyYCrd1F5aIxWXHghBH/t+QU31gyQOxCdj18f10ssfuY/y7vocSJQ9pTRRPNh4beGAijV1AETaXWLK1L6oMnkbdhr9ZA2I6cZaHNCaHIynHQH7NUqKKQUJL/FyZ8rBv4YNnxCMRi9p88IoTb0oPsUCoNCaIZ2cvzXz+0VpU6zxj4ke7H6Bu7H46MSB1P+YHzGLtFNzZJVsUBEkz7dotUDeTeqlYKnq7oldWJ4HlqODevzCev+FRnYgrYpoXmYC/dxa1R5IlKCu6rEmP05A7Nw4h9cymnTwRMEoZRSppJ2O5FlSx/Go9Jz12g2Tfiaf+RvO7nkIb2qKiz7Jo2aJgakL5lMOlEdBA2+dsYSyX4Tvu8Ua4p0GcYaGOgSjXH27lQ73ZpHSicf4Q1kAooVl+6zTOPAqgMXOnyyVSRqBPse28HuDwGtmD8BAeVDIfkMW+a+PlWa+yoEWKfDHRduoxNod7Pc9xlNFt6eOeGoBQTEIiF7ccBDtNiSU1yfvqBZEgI8QF0QiGUo9eP7+59so5eu9/DuzjdqFMmGPtG3zHifMxuMzO5+E9UxTyHuCwvxuH93F4vmPC8zzXXn8/ErhEeqmYl1lxZbfJDm1qcjTkJibNKJ9+CXUeP0hq8yi07SEN1xJSZpupf90EUjrdFd3impz3gZKummEjTvzr3J1JX4gC/wD0mGkROHQwb0jCTDJNC18cX4usPYtNr3FxLZmxCGgPmZhkzFjF0qppN1aXTxQskdorEejQUwLL5EnWJySd9/W2P6PmjkJTAwKYUNHsmVUAfbMA7y7QBIjVFFWS4xYy0GJcc8NaLKkFMkGv/oluw552prWAJZ4aM2asoNgyv/JARrAF+JbOPSpax+CtUMO+LCFlBITHopbkHz0TwI1UMj/vIOh9wxDiMqe3YBiviymudX+B7awCaUPTLubWW1jwC4kBnXmRGAKyyIvzgOvwkdcKfQRxoTxq7JFTL/hWk7x4HlQqviSWGY166CLIp6SydCT+cqHMf3MHhe8AQZVC+nIDVNQZWfpFbOFb3nNDwlT+laWrtsiuX7hHiL0VLaCU4xzup5m4zvi59/Qxj0+d8n6M/3GP3/Tvp/bKY9m7CHoeimtGF9Ai2QFJFMOEQw3S1SUBL62ZsezKgBap6y1RqmMzdz/h3f5mhHxRMoQ0kgzZwMNWJvi2acGoIttcmBU7Cn6fqxYNi11dg17M7cFJAQCMicvd4pEwl8IBrm7uFrzbLvuLeolyiDx8GX3jfIo//Ceqa6P/RIqN8jKzH3nTSePuVqkXYiIdxhlAeF//EYW0CwOjd3GEoc=|aUt6NKqrLW4HeprkbwjuBzSQbR84imTujhUPxK17eX4=",
			"publicKey":           "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAmAYrTtY4FBJL/TeTGqr1uHCoMCzUDgwvgq7gBGiNrk24gPbb3xreM+HxubBvkzTlgoS6m1KKKKtD4tWrLU33Xc+PevbKSZDLvBfUe+golGU1XKFxUcIkgINtB0i8LmCVCShiCrlhn2VorcAbekR/1RXtoJqpqq1urhI+RdGVXy8HBBoULA7BoV7wC8dBdkRtnQMNuvGyHclV7yjgealKGqgxz4aNcgsfybquKvYg6PUj8dAxUy7KlmMR7klPyO8nahYqyhpQ/t0xle0WyCkdx5YuYhRSA67Tok+E8fCW5WXOPfIdPZDXS+6/wW1NhcQEa5j6EW11PF/Xq0awBUFwnwIDAQAB",
		})

		e.POST("/bitwarden/api/accounts/keys").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes(body).
			Expect().Status(200)

		setting, err := settings.Get(inst)
		assert.NoError(t, err)
		orgKey, err := setting.OrganizationKey()
		assert.NoError(t, err)
		assert.NotEmpty(t, orgKey)
	})

	t.Run("SettingsDomains", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/bitwarden/api/settings/domains").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
        "equivalentDomains": [ ["stackoverflow.com", "serverfault.com", "superuser.com"] ],
        "globalEquivalentDomains": [42, 69]
      }`)).
			Expect().Status(200).
			JSON().Object()

		assertDomainsReponse(t, obj)

		obj = e.GET("/bitwarden/api/settings/domains").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON().Object()

		assertDomainsReponse(t, obj)
	})

	t.Run("ImportCiphers", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		nbCiphers, err := couchdb.CountAllDocs(inst, consts.BitwardenCiphers)
		assert.NoError(t, err)

		nbFolders, err := couchdb.CountAllDocs(inst, consts.BitwardenFolders)
		assert.NoError(t, err)

		e.POST("/bitwarden/api/ciphers/import").
			WithHeader("Content-Type", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
      "ciphers": [{
        "type": 2,
        "favorite": true,
        "name": "2.G38TIU3t1pGOfkzjCQE7OQ==|Xa1RupttU7zrWdzIT6oK+w==|J3C6qU1xDrfTgyJD+OrDri1GjgGhU2nmRK75FbZHXoI=",
        "folderId": null,
        "organizationId": null,
        "notes": "2.rSw0uVQEFgUCEmOQx0JnDg==|MKqHLD25aqaXYHeYJPH/mor7l3EeSQKsI7A/R+0bFTI=|ODcUScISzKaZWHlUe4MRGuTT2S7jpyDmbOHl7d+6HiM=",
        "secureNote": {
          "type": 0
        }
      }, {
        "type": 1,
        "favorite": false,
        "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
        "folderId": null,
        "organizationId": null,
        "notes": null,
        "login": {
          "uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
          "username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
          "password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
          "totp": null
        }
      }],
      "folders": [{
        "name": "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o="
      }],
      "folderRelationships": [
        {"key": 1, "value": 0}
      ]
    }`)).
			Expect().Status(200)

		nb, err := couchdb.CountAllDocs(inst, consts.BitwardenCiphers)
		assert.NoError(t, err)
		assert.Equal(t, nbCiphers+2, nb)

		nb, err = couchdb.CountAllDocs(inst, consts.BitwardenFolders)
		assert.NoError(t, err)
		assert.Equal(t, nbFolders+1, nb)
	})

	t.Run("Organization", func(t *testing.T) {
		var orgaID string

		t.Run("Create", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.POST("/bitwarden/api/organizations").
				WithHeader("Content-Type", "application/json").
				WithHeader("Authorization", "Bearer "+token).
				WithBytes([]byte(`{
        "name": "Family Organization",
        "key": "bmFjbF53D9mrdGbVqQzMB54uIg678EIpU/uHFYjynSPSA6vIv5/6nUy4Uk22SjIuDB3pZ679wLE3o7R/Imzn47OjfT6IrJ8HaysEhsZA25Dn8zwEtTMtgNepUtH084wAMgNeIcElW24U/MfRscjAk8cDUIm5xnzyi2vtJfe9PcHTmzRXyng=",
        "collectionName": "2.rrpSDDODsWZqL7EhLVsu/Q==|OSuh+MmmR89ppdb/A7KxBg==|kofpAocL2G4a3P1C2R1U+i9hWbhfKfsPKM6kfoyCg/M="
      }`)).
				Expect().Status(200).
				JSON().Object()

			obj.ValueEqual("Name", "Family Organization")
			obj.ValueEqual("Object", "profileOrganization")
			obj.ValueEqual("Enabled", true)
			obj.ValueEqual("Status", 2)
			obj.ValueEqual("Type", 0)

			orgaID = obj.Value("Id").String().NotEmpty().Raw()

			obj.Value("Key").String().NotEmpty()
		})

		t.Run("Get", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.GET("/bitwarden/api/organizations/"+orgaID).
				WithHeader("Authorization", "Bearer "+token).
				Expect().Status(200).
				JSON().Object()

			obj.ValueEqual("Name", "Family Organization")
			obj.ValueEqual("Object", "profileOrganization")
			obj.ValueEqual("Enabled", true)
			obj.ValueEqual("Status", 2)
			obj.ValueEqual("Type", 0)
			obj.ValueEqual("Id", orgaID)
			obj.Value("Key").String().NotEmpty()
		})

		t.Run("ListCollections", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.GET("/bitwarden/api/organizations/"+orgaID+"/collections").
				WithHeader("Authorization", "Bearer "+token).
				Expect().Status(200).
				JSON().Object()

			obj.ValueEqual("Object", "list")
			data := obj.Value("Data").Array()
			data.Length().Equal(1)

			coll := data.First().Object()
			coll.Value("Id").String().NotEmpty()
			coll.ValueEqual("Name", "2.rrpSDDODsWZqL7EhLVsu/Q==|OSuh+MmmR89ppdb/A7KxBg==|kofpAocL2G4a3P1C2R1U+i9hWbhfKfsPKM6kfoyCg/M=")
			coll.ValueEqual("Object", "collection")
			coll.ValueEqual("OrganizationId", orgaID)
			coll.ValueEqual("ReadOnly", false)
		})

		t.Run("SyncOrganizationAndCollection", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.GET("/bitwarden/api/sync").
				WithHeader("Authorization", "Bearer "+token).
				Expect().Status(200).
				JSON().Object()

			obj.ValueEqual("Object", "sync")
			profile := obj.Value("Profile").Object()
			orgs := profile.Value("Organizations").Array()
			orgs.Length().Equal(2)

			for _, item := range orgs.Iter() {
				org := item.Object()

				if org.Value("Id").String().Raw() == orgaID {
					org.ValueEqual("Name", "Family Organization")
				} else {
					org.ValueEqual("Name", "Cozy")
				}

				org.Value("Key").String().NotEmpty()
				org.ValueEqual("Object", "profileOrganization")
			}

			colls := obj.Value("Collections").Array()
			colls.Length().Equal(2)
			for _, item := range colls.Iter() {
				coll := item.Object()

				if coll.Value("Id").String().Raw() != collID {
					coll.ValueEqual("OrganizationId", orgaID)
					coll.ValueEqual("Name", "2.rrpSDDODsWZqL7EhLVsu/Q==|OSuh+MmmR89ppdb/A7KxBg==|kofpAocL2G4a3P1C2R1U+i9hWbhfKfsPKM6kfoyCg/M=")
				}

				coll.ValueEqual("Object", "collection")
			}
		})

		t.Run("DeleteOrganization", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			email := inst.PassphraseSalt()
			iter := crypto.DefaultPBKDF2Iterations
			pass, _ := crypto.HashPassWithPBKDF2([]byte("cozy"), email, iter)

			e.DELETE("/bitwarden/api/organizations/"+orgaID).
				WithHeader("Content-Type", "application/json").
				WithHeader("Authorization", "Bearer "+token).
				WithBytes([]byte(fmt.Sprintf(`{"masterPasswordHash": "%s"}`, pass))).
				Expect().Status(200)
		})
	})

	t.Run("ChangeSecurityStamp", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		email := inst.PassphraseSalt()
		iter := crypto.DefaultPBKDF2Iterations
		pass, _ := crypto.HashPassWithPBKDF2([]byte("cozy"), email, iter)

		e.POST("/bitwarden/api/accounts/security-stamp").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(fmt.Sprintf(`{"masterPasswordHash": %q}`, pass))).Expect().Status(204)

		// Check that token is no longer valid
		e.GET("/bitwarden/api/folders").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(401)
	})

	t.Run("SendHint", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/bitwarden/api/accounts/password-hint").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{ "email": "me@bitwarden.example.net" }`)).
			Expect().Status(200)
	})
}

func assertCipherResponse(t *testing.T, obj *httpexpect.Object) {
	t.Helper()

	obj.ValueEqual("Object", "cipher")
	obj.Value("Id").String().NotEmpty()
	obj.ValueEqual("Type", 1.0)
	obj.ValueEqual("Favorite", false)
	obj.ValueEqual("Name", "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=")
	obj.Value("Notes").Null()
	obj.Value("FolderId").Null()

	loginObj := obj.Value("Login").Object().NotEmpty()
	loginObj.ValueEqual("PasswordRevisionDate", "2019-09-13T12:26:42+02:00")
	loginObj.Value("Totp").Null()
	loginObj.ValueEqual("Username", "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=")
	loginObj.ValueEqual("Password", "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=")

	loginObj.Value("Uris").Array().Length().Equal(1)
	uriObj := loginObj.Value("Uris").Array().First().Object()
	uriObj.ValueEqual("Uri", "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=")
	uriObj.Value("Match").Null()

	obj.Value("Fields").Null()
	obj.Value("Attachments").Null()
	obj.Value("RevisionDate").String().DateTime(time.RFC3339)
	obj.ValueEqual("Edit", true)
	obj.ValueEqual("OrganizationUseTotp", false)
}

func assertUpdatedCipherResponse(t *testing.T, obj *httpexpect.Object, cipherID, folderID string) {
	t.Helper()

	obj.ValueEqual("Object", "cipher")
	obj.ValueEqual("Id", cipherID)
	obj.ValueEqual("Type", 2.0)
	obj.ValueEqual("Favorite", true)
	obj.ValueEqual("Name", "2.G38TIU3t1pGOfkzjCQE7OQ==|Xa1RupttU7zrWdzIT6oK+w==|J3C6qU1xDrfTgyJD+OrDri1GjgGhU2nmRK75FbZHXoI=")
	obj.ValueEqual("FolderId", folderID)
	obj.ValueEqual("Notes", "2.rSw0uVQEFgUCEmOQx0JnDg==|MKqHLD25aqaXYHeYJPH/mor7l3EeSQKsI7A/R+0bFTI=|ODcUScISzKaZWHlUe4MRGuTT2S7jpyDmbOHl7d+6HiM=")
	obj.Value("SecureNote").Object().NotEmpty().ValueEqual("Type", 0.0)
	obj.NotContainsKey("Login")
	obj.Value("Fields").Null()
	obj.Value("Attachments").Null()
	obj.Value("RevisionDate").String().DateTime(time.RFC3339)
	obj.ValueEqual("Edit", true)
	obj.ValueEqual("OrganizationUseTotp", false)
}

func assertDomainsReponse(t *testing.T, obj *httpexpect.Object) {
	obj.ValueEqual("Object", "domains")
	equivalent := obj.Value("EquivalentDomains").Array()
	equivalent.Length().Equal(1)
	domains := equivalent.First().Array()
	domains.Length().Equal(3)
	domains.Element(0).Equal("stackoverflow.com")
	domains.Element(1).Equal("serverfault.com")
	domains.Element(2).Equal("superuser.com")

	global := obj.Value("GlobalEquivalentDomains").Array()
	global.Length().Equal(len(bitwarden.GlobalDomains))

	for _, item := range global.Iter() {
		k := int(item.Object().Value("Type").Number().Raw())
		excluded := (k == 42) || (k == 69)
		item.Object().ValueEqual("Excluded", excluded)
		item.Object().Value("Domains").Array().Length().Gt(0)
	}
}
