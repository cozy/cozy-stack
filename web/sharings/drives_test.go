package sharings_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/notes"
	"github.com/cozy/cozy-stack/web/sharings"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper functions for file operations

// createDirectory creates a directory with the given name in the specified parent directory
// and returns the directory ID
func createDirectory(t *testing.T, client *httpexpect.Expect, parentDirID, name, token string) string {
	t.Helper()
	dirID := client.POST("/files/"+parentDirID).
		WithQuery("Name", name).
		WithQuery("Type", "directory").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(201).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object().Path("$.data.id").String().NotEmpty().Raw()
	require.NotEmpty(t, dirID, "Error creating directory")
	return dirID
}

// createRootDirectory creates a directory with the given name at the root level
// and returns the directory ID
func createRootDirectory(t *testing.T, client *httpexpect.Expect, name, token string) string {
	return createDirectory(t, client, "", name, token)
}

// createFile creates a file with the given name in the specified parent directory
// and returns the file ID
func createFile(t *testing.T, client *httpexpect.Expect, parentDirID, name, token string) string {
	t.Helper()
	fileID := client.POST("/files/"+parentDirID).
		WithQuery("Name", name).
		WithQuery("Type", "file").
		WithHeader("Content-Type", "text/plain").
		WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
		WithHeader("Authorization", "Bearer "+token).
		WithBytes([]byte("foo")).
		Expect().Status(201).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object().Path("$.data.id").String().NotEmpty().Raw()
	require.NotEmpty(t, fileID, "Error creation of the file")
	return fileID
}

// verifyFileMove verifies that a file was moved successfully by checking its attributes
func verifyFileMove(t *testing.T, inst *instance.Instance, fileID, expectedName, expectedDirID string, expectedContent string) {
	t.Helper()
	// Get the file from the VFS
	fs := inst.VFS()

	// Verify file exists in destination with correct attributes
	fileDoc, err := fs.FileByID(fileID)
	require.NoError(t, err)
	require.Equal(t, expectedName, fileDoc.DocName)
	require.Equal(t, expectedDirID, fileDoc.DirID)
	require.Equal(t, int64(len(expectedContent)), fileDoc.ByteSize)

	// Verify file content was preserved
	fileHandle, err := fs.OpenFile(fileDoc)
	require.NoError(t, err)
	defer fileHandle.Close()

	content, err := io.ReadAll(fileHandle)
	require.NoError(t, err)
	require.Equal(t, expectedContent, string(content))
}

// verifyFileDeleted verifies that a file was deleted from the source instance
func verifyFileDeleted(t *testing.T, inst *instance.Instance, fileID string) {
	t.Helper()
	_, err := inst.VFS().FileByID(fileID)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

// createSharedDriveForAcme creates the base directory and the sharing from ACME to Betty.
// It returns the created sharing ID and the shared root directory ID.
func createSharedDriveForAcme(
	t *testing.T,
	acmeInstance *instance.Instance,
	acmeAppToken string,
	tsAURL string,
	driveName string,
	description string,
) (
	sharingID string,
	productID string,
	discovery string,
) {
	t.Helper()

	eA := httpexpect.Default(t, tsAURL)

	// Prepare contacts and the directory that will be shared
	contact := createContact(t, acmeInstance, "Betty", "betty@example.net")
	require.NotNil(t, contact)
	daveContact := createContact(t, acmeInstance, "Dave", "dave@example.net")
	require.NotNil(t, daveContact)

	productID = eA.POST("/files/").
		WithQuery("Name", driveName).
		WithQuery("Type", "directory").
		WithHeader("Authorization", "Bearer "+acmeAppToken).
		Expect().Status(201).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object().Path("$.data.id").String().NotEmpty().Raw()

	// Create the shared drive
	obj := eA.POST("/sharings/").
		WithHeader("Authorization", "Bearer "+acmeAppToken).
		WithHeader("Content-Type", "application/vnd.api+json").
		WithBytes([]byte(`{
        "data": {
          "type": "` + consts.Sharings + `",
          "attributes": {
            "description": "` + description + `",
            "drive": true,
            "rules": [{
                "title": "` + driveName + `",
                "doctype": "` + consts.Files + `",
                "values": ["` + productID + `"]
              }]
          },
          "relationships": {
            "recipients": {
              "data": [{"id": "` + contact.ID() + `", "type": "` + contact.DocType() + `"}]
            },
            "read_only_recipients": {
              "data": [{"id": "` + daveContact.ID() + `", "type": "` + daveContact.DocType() + `"}]
            }
          }
        }
      }`)).
		Expect().Status(201).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object()

	sharingID = obj.Value("data").Object().Value("id").String().NotEmpty().Raw()

	// pull fresh invitation data without relying on globals
	sentDescription, disco := extractInvitationLink(t, acmeInstance, "ACME")
	assert.Equal(t, sentDescription, description)
	assert.Contains(t, disco, "/discovery?state=")
	discovery = disco

	return
}

// acceptSharedDrive performs the acceptance flow on the recipient side using
// the previously generated discovery and authorize links.
func acceptSharedDrive(
	t *testing.T,
	recipientInstance *instance.Instance,
	tsAURL string,
	tsRecipientURL string,
	sharingID string,
	discoveryLink string,
) {
	t.Helper()
	eA := httpexpect.Default(t, tsAURL)
	eR := httpexpect.Default(t, tsRecipientURL)

	// Recipient login
	token := eR.GET("/auth/login").
		Expect().Status(200).
		Cookie("_csrf").Value().NotEmpty().Raw()
	eR.POST("/auth/login").
		WithCookie("_csrf", token).
		WithFormField("passphrase", "MyPassphrase").
		WithFormField("csrf_token", token).
		WithRedirectPolicy(httpexpect.DontFollowRedirects).
		Expect().Status(303).
		Header("Location").Contains("home")

	// Recipient goes to the discovery link on owner host (provided by caller)
	u, err := url.Parse(discoveryLink)
	assert.NoError(t, err)
	state = u.Query()["state"][0]

	eA.GET(u.Path).
		WithQuery("state", state).
		Expect().Status(200).
		HasContentType("text/html", "utf-8").
		Body().
		Contains("Connect to your Twake").
		Contains(`<input type="hidden" name="state" value="` + state)

	redirectHeader := eA.POST(u.Path).
		WithFormField("state", state).
		WithFormField("slug", tsRecipientURL).
		WithRedirectPolicy(httpexpect.DontFollowRedirects).
		Expect().Status(302).Header("Location")

	redirectHeader.Contains(tsRecipientURL)
	redirectHeader.Contains("/auth/authorize/sharing")
	authorizeLink = redirectHeader.NotEmpty().Raw()

	// Ensure the owner instance URL is set for this specific sharing
	FakeOwnerInstanceForSharing(t, recipientInstance, tsAURL, sharingID)

	u, err = url.Parse(authorizeLink)
	assert.NoError(t, err)
	st := u.Query()["state"][0]

	// Perform authorize request without following redirect to drive subdomain
	eR.GET(u.Path).
		WithQuery("sharing_id", sharingID).
		WithQuery("state", st).
		WithRedirectPolicy(httpexpect.DontFollowRedirects).
		Expect().Status(303).
		Header("Location").Contains("#/folder/io.cozy.files.shared-drives-dir")
}

// acceptSharedDriveForBetty is kept for convenience and delegates to acceptSharedDrive.
func acceptSharedDriveForBetty(
	t *testing.T,
	bettyInstance *instance.Instance,
	tsAURL string,
	tsBURL string,
	sharingID string,
	discoveryLink string,
) {
	acceptSharedDrive(t, bettyInstance, tsAURL, tsBURL, sharingID, discoveryLink)
}

// removed createSharedDriveForAcmeWithName in favor of createSharedDriveForAcme with parameters

func TestSharedDrives(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var productID,
		meetingsID,
		checklistID,
		outsideOfShareID,
		otherSharedFileThenTrashedID,
		otherSharedFileThenDeletedID,
		sharingID string
	var checklistName = "Checklist.txt"

	config.UseTestFile(t)
	build.BuildMode = build.ModeDev
	config.GetConfig().Assets = "../../assets"
	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb(t)
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	// Prepare the Twake instance for the ACME organization
	setup := testutils.NewSetup(t, t.Name()+"_acme")
	acmeInstance := setup.GetTestInstance(&lifecycle.Options{
		Email:      "acme@example.net",
		PublicName: "ACME",
	})
	acmeAppToken := generateAppToken(acmeInstance, "drive", "io.cozy.files")
	tsA := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/files":    files.Routes,
		"/notes":    notes.Routes,
		"/sharings": sharings.Routes,
	})
	tsA.Config.Handler.(*echo.Echo).Renderer = render
	tsA.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsA.Close)

	// Prepare Betty's instance
	bettySetup := testutils.NewSetup(t, t.Name()+"_betty")
	bettyInstance := bettySetup.GetTestInstance(&lifecycle.Options{
		Email:         "betty@example.net",
		PublicName:    "Betty",
		Passphrase:    "MyPassphrase",
		KdfIterations: 5000,
		Key:           "xxx",
	})
	bettyAppToken := generateAppToken(bettyInstance, "drive", consts.Files)
	tsB := bettySetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth": func(g *echo.Group) {
			g.Use(middlewares.LoadSession)
			auth.Routes(g)
		},
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsB.Config.Handler.(*echo.Echo).Renderer = render
	tsB.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsB.Close)

	// Prepare Dave's instance (read-only recipient)
	daveSetupName := strings.ReplaceAll(t.Name(), "/", "_") + "_dave"
	daveSetup := testutils.NewSetup(t, daveSetupName)
	daveInstance := daveSetup.GetTestInstance(&lifecycle.Options{
		Email:         "dave@example.net",
		PublicName:    "Dave",
		Passphrase:    "MyPassphrase",
		KdfIterations: 5000,
		Key:           "xxx",
	})
	daveAppToken := generateAppToken(daveInstance, "drive", consts.Files)
	tsD := daveSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth": func(g *echo.Group) {
			g.Use(middlewares.LoadSession)
			auth.Routes(g)
		},
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsD.Config.Handler.(*echo.Echo).Renderer = render
	tsD.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsD.Close)

	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()), "Could not init dynamic FS")

	type clientInfo struct {
		client *httpexpect.Expect
		token  string
	}
	type clientMaker func(t *testing.T) clientInfo

	makeClientToActAsACMETheOwner := func(t *testing.T) clientInfo {
		return clientInfo{client: httpexpect.Default(t, tsA.URL), token: acmeAppToken}
	}
	makeClientToActAsBettyTheRecipient := func(t *testing.T) clientInfo {
		return clientInfo{client: httpexpect.Default(t, tsB.URL), token: bettyAppToken}
	}
	makeClientToActAsAnonymous := func(t *testing.T) clientInfo {
		return clientInfo{client: httpexpect.Default(t, tsB.URL)}
	}
	runTestAsACMEThenAgainAsBetty := func(t *testing.T, runner func(t *testing.T, clientMaker clientMaker)) {
		for name, maker := range map[string]clientMaker{
			"AsACMETheOwner":      makeClientToActAsACMETheOwner,
			"AsBettyTheRecipient": makeClientToActAsBettyTheRecipient,
		} {
			t.Run(name, func(t *testing.T) {
				runner(t, maker)
			})
		}
	}

	t.Run("CreateSharedDrive", func(t *testing.T) {
		// Create the shared drive on ACME side only
		sid, dirID, disco := createSharedDriveForAcme(t, acmeInstance, acmeAppToken, tsA.URL, "Product team", "Drive for the product team")
		sharingID, productID = sid, dirID
		_ = disco

		// Prepare additional folders/files used by subsequent tests
		eA := httpexpect.Default(t, tsA.URL)

		outsideOfShareID = eA.POST("/files/").
			WithQuery("Name", "Unshared directory at the root of ACME").
			WithQuery("Type", "directory").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		meetingsID = eA.POST("/files/"+productID).
			WithQuery("Name", "Meetings").
			WithQuery("Type", "directory").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		checklistID = eA.POST("/files/"+meetingsID).
			WithQuery("Name", checklistName).
			WithQuery("Type", "file").
			WithHeader("Content-Type", "text/plain").
			WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			WithBytes([]byte("foo")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		otherSharedFileThenDeletedID = eA.POST("/files/"+meetingsID).
			WithQuery("Name", "Shared but then deleted.txt").
			WithQuery("Type", "file").
			WithHeader("Content-Type", "text/plain").
			WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			WithBytes([]byte("foo")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		httpexpect.Default(t, tsA.URL).DELETE("/files/"+otherSharedFileThenDeletedID).
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			Expect().Status(200)

		// Empty Trash
		httpexpect.Default(t, tsA.URL).DELETE("/files/trash").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			Expect().Status(204)

		otherSharedFileThenTrashedID = eA.POST("/files/"+meetingsID).
			WithQuery("Name", "Shared but then trashed.txt").
			WithQuery("Type", "file").
			WithHeader("Content-Type", "text/plain").
			WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			WithBytes([]byte("foo")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		httpexpect.Default(t, tsA.URL).DELETE("/files/"+otherSharedFileThenTrashedID).
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			Expect().Status(200)
	})

	t.Run("ListSharedDrives", func(t *testing.T) {
		e := httpexpect.Default(t, tsA.URL)

		obj := e.GET("/sharings/drives").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Array()
		data.Length().IsEqual(1)

		sharingObj := data.Value(0).Object()
		sharingObj.Value("type").IsEqual("io.cozy.sharings")
		sharingObj.Value("id").String().NotEmpty()

		attrs := sharingObj.Value("attributes").Object()
		attrs.Value("description").IsEqual("Drive for the product team")
		attrs.Value("app_slug").IsEqual("drive")
		attrs.Value("owner").IsEqual(true)
		attrs.Value("drive").IsEqual(true)

		members := attrs.Value("members").Array()
		members.Length().IsEqual(3)

		owner := members.Value(0).Object()
		owner.Value("status").IsEqual("owner")
		owner.Value("public_name").IsEqual("ACME")

		recipient := members.Value(1).Object()
		recipient.Value("name").IsEqual("Betty")
		recipient.Value("email").IsEqual("betty@example.net")

		members.Value(2).Object().Value("name").IsEqual("Dave")

		rules := attrs.Value("rules").Array()
		rules.Length().IsEqual(1)
		rule := rules.Value(0).Object()
		rule.Value("title").IsEqual("Product team")
		rule.Value("doctype").IsEqual("io.cozy.files")
		rule.Value("values").Array().Value(0).IsEqual(productID)
	})

	t.Run("AcceptSharedDrive", func(t *testing.T) {
		// fetch fresh discovery link for this sharing
		_, disco := extractInvitationLink(t, acmeInstance, "ACME")
		acceptSharedDriveForBetty(t, bettyInstance, tsA.URL, tsB.URL, sharingID, disco)
	})

	t.Run("HeadDirOrFile", func(t *testing.T) {
		eB := httpexpect.Default(t, tsB.URL)

		// HEAD request on non-existing file should return 404 for recipient
		eB.HEAD("/sharings/drives/"+sharingID+"/nonexistent").
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(404)

		// HEAD request on directory should return 200 for recipient
		eB.HEAD("/sharings/drives/"+sharingID+"/"+productID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200)

		// HEAD request on file should return 200 for recipient
		eB.HEAD("/sharings/drives/"+sharingID+"/"+meetingsID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200)

		// HEAD request without authentication should fail
		eB.HEAD("/sharings/drives/" + sharingID + "/" + checklistID).
			Expect().Status(401)

		// HEAD request with wrong sharing ID should fail
		eB.HEAD("/sharings/drives/wrong-id/"+checklistID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(404)
	})

	t.Run("GetDirOrFile", func(t *testing.T) {
		eB := httpexpect.Default(t, tsB.URL)

		// GET request on non-existing file should return 404 for recipient
		eB.GET("/sharings/drives/"+sharingID+"/nonexistent").
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(404)

		// GET request on directory should return 200 and directory data for recipient
		obj := eB.GET("/sharings/drives/"+sharingID+"/"+meetingsID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.Value("type").String().IsEqual("io.cozy.files")
		data.Value("id").String().IsEqual(meetingsID)
		attrs := data.Value("attributes").Object()
		attrs.Value("type").String().IsEqual("directory")
		attrs.Value("name").String().IsEqual("Meetings")
		attrs.Value("path").String().IsEqual("/Product team/Meetings")
		attrs.Value("driveId").String().IsEqual(sharingID)

		contents := data.Path("$.relationships.contents.data").Array()
		contents.Length().IsEqual(1)
		fileRef := contents.Value(0).Object()
		fileRef.Value("type").String().IsEqual("io.cozy.files")
		fileRef.Value("id").String().IsEqual(checklistID)

		// GET request on file should return 200 and file data for recipient
		obj = eB.GET("/sharings/drives/"+sharingID+"/"+checklistID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data = obj.Value("data").Object()
		data.Value("type").String().IsEqual("io.cozy.files")
		data.Value("id").String().IsEqual(checklistID)
		attrs = data.Value("attributes").Object()
		attrs.Value("type").String().IsEqual("file")
		attrs.Value("name").String().IsEqual("Checklist.txt")
		attrs.Value("mime").String().IsEqual("text/plain")
		attrs.Value("size").String().IsEqual("3")

		// GET request without authentication should fail
		eB.GET("/sharings/drives/" + sharingID + "/" + meetingsID).
			Expect().Status(401)

		// GET request with wrong sharing ID should fail
		eB.GET("/sharings/drives/wrong-id/"+meetingsID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(404)
	})

	t.Run("GetMetadata", func(t *testing.T) {
		runTestAsACMEThenAgainAsBetty(t, func(t *testing.T, clientMaker clientMaker) {
			clientInfo := clientMaker(t)

			// GET metadata request with path should return file metadata
			obj := clientInfo.client.GET("/sharings/drives/"+sharingID+"/metadata").
				WithQuery("Path", "/Product team/Meetings/Checklist.txt").
				WithHeader("Authorization", "Bearer "+clientInfo.token).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()

			data := obj.Value("data").Object()
			data.Value("type").String().IsEqual("io.cozy.files")
			data.Value("id").String().IsEqual(checklistID)
			attrs := data.Value("attributes").Object()
			attrs.Value("type").String().IsEqual("file")
			attrs.Value("name").String().IsEqual("Checklist.txt")

			// GET metadata request with directory path should return directory metadata
			obj = clientInfo.client.GET("/sharings/drives/"+sharingID+"/metadata").
				WithQuery("Path", "/Product team/Meetings").
				WithHeader("Authorization", "Bearer "+clientInfo.token).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()

			data = obj.Value("data").Object()
			data.Value("type").String().IsEqual("io.cozy.files")
			data.Value("id").String().IsEqual(meetingsID)
			attrs = data.Value("attributes").Object()
			attrs.Value("type").String().IsEqual("directory")
			attrs.Value("name").String().IsEqual("Meetings")

			// GET metadata request with non-existent path should return 404
			clientInfo.client.GET("/sharings/drives/"+sharingID+"/metadata").
				WithQuery("Path", "/Product team/NonExistent").
				WithHeader("Authorization", "Bearer "+clientInfo.token).
				Expect().Status(404)

			// GET metadata request without path should return 400
			clientInfo.client.GET("/sharings/drives/"+sharingID+"/metadata").
				WithHeader("Authorization", "Bearer "+clientInfo.token).
				Expect().Status(400)
		})

		// GET metadata request without authentication should fail
		eB := httpexpect.Default(t, tsB.URL)
		eB.GET("/sharings/drives/"+sharingID+"/metadata").
			WithQuery("Path", "/Product team/Meetings/Checklist.txt").
			Expect().Status(401)
	})

	t.Run("GetDirSize", func(t *testing.T) {
		eB := httpexpect.Default(t, tsB.URL)
		u := eB.GET("/sharings/drives/"+sharingID+"/"+productID+"/size").
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := u.Value("data").Object()
		data.Value("id").IsEqual(productID)
		data.Value("type").IsEqual("io.cozy.files.sizes")
		data.Value("attributes").Object().Value("size").IsEqual("3")
	})

	t.Run("ModifyMetadataByIDHandler", func(t *testing.T) {
		t.Run("CanMoveSharedFile", func(t *testing.T) {
			eA := httpexpect.Default(t, tsA.URL)
			eB := httpexpect.Default(t, tsB.URL)

			// Create a file in the shared directory
			notesID := createFile(t, eA, meetingsID, "Meeting notes.txt", acmeAppToken)

			// Move the file to another directory within the shared drive
			movedNotes := eB.PATCH("/sharings/drives/"+sharingID+"/"+notesID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
				  "data": {
				    "type": "io.cozy.files",
					"id": "` + notesID + `",
					"attributes": {
					  "dir_id": "` + productID + `"
					}
				  }
				}`)).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()
			movedNotes.Path("$.data.attributes.dir_id").String().NotEmpty().IsEqual(productID)
		})

		t.Run("CannotMoveSharedFileOutsideSharedDrive", func(t *testing.T) {
			eA := httpexpect.Default(t, tsA.URL)
			eB := httpexpect.Default(t, tsB.URL)

			// Create a file in the shared directory
			notesID := createFile(t, eA, meetingsID, "Meeting notes.txt", acmeAppToken)

			// Attempt to move the file outside the shared drive (should fail)
			eB.PATCH("/sharings/drives/"+sharingID+"/"+notesID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
				  "data": {
				    "type": "io.cozy.files",
					"id": "` + notesID + `",
					"attributes": {
					  "dir_id": "` + outsideOfShareID + `"
					}
				  }
				}`)).
				Expect().Status(403)
		})

		t.Run("CannotMoveUnsharedFileToSharedDrive", func(t *testing.T) {
			eA := httpexpect.Default(t, tsA.URL)
			eB := httpexpect.Default(t, tsB.URL)

			// Create a file outside the shared directory
			notesID := createFile(t, eA, outsideOfShareID, "Meeting notes.txt", acmeAppToken)

			// Attempt to move the unshared file into the shared drive (should fail)
			eB.PATCH("/sharings/drives/"+sharingID+"/"+notesID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
				  "data": {
				    "type": "io.cozy.files",
					"id": "` + notesID + `",
					"attributes": {
					  "dir_id": "` + meetingsID + `"
					}
				  }
				}`)).
				Expect().Status(403)
		})

		t.Run("CanRenameSharedFile", func(t *testing.T) {
			eA := httpexpect.Default(t, tsA.URL)
			eB := httpexpect.Default(t, tsB.URL)

			todoID := eA.POST("/files/"+meetingsID).
				WithQuery("Name", "TODO.txt").
				WithQuery("Type", "file").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithBytes([]byte("foo")).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().Path("$.data.id").String().NotEmpty().Raw()

			renamedTodo := eB.PATCH("/sharings/drives/"+sharingID+"/"+todoID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
				  "data": {
				    "type": "io.cozy.files",
					"id": "` + todoID + `",
					"attributes": {
					  "name": "Minutes.txt"
					}
				  }
				}`)).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()
			renamedTodo.Path("$.data.attributes.name").String().NotEmpty().IsEqual("Minutes.txt")
		})

		t.Run("CannotRenameSharedFileWithConflictingName", func(t *testing.T) {
			eA := httpexpect.Default(t, tsA.URL)
			eB := httpexpect.Default(t, tsB.URL)

			todoID := eA.POST("/files/"+meetingsID).
				WithQuery("Name", "TODO.txt").
				WithQuery("Type", "file").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithBytes([]byte("foo")).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().Path("$.data.id").String().NotEmpty().Raw()

			eA.POST("/files/"+meetingsID).
				WithQuery("Name", "Next steps.txt").
				WithQuery("Type", "file").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithBytes([]byte("foo")).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().Path("$.data.id").String().NotEmpty().Raw()

			eB.PATCH("/sharings/drives/"+sharingID+"/"+todoID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
				  "data": {
				    "type": "io.cozy.files",
					"id": "` + todoID + `",
					"attributes": {
					  "name": "Next steps.txt"
					}
				  }
				}`)).
				Expect().Status(409)
		})

		t.Run("CanModifyFileMetadata", func(t *testing.T) {
			eA := httpexpect.Default(t, tsA.URL)
			eB := httpexpect.Default(t, tsB.URL)

			script := eA.POST("/files/"+meetingsID).
				WithQuery("Name", "script.sh").
				WithQuery("Type", "file").
				WithQuery("CreatedAt", "2025-06-17T01:12:47.982Z").
				WithQuery("UpdatedAt", "2025-06-17T01:12:47.982Z").
				WithQuery("Encrypted", "true").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithBytes([]byte("foo")).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().Path("$.data").Object()

			attrs := script.Value("attributes").Object()
			attrs.NotContainsKey("tags")
			attrs.HasValue("updated_at", "2025-06-17T01:12:47.982Z")
			attrs.HasValue("executable", false)
			attrs.HasValue("encrypted", true)
			attrs.HasValue("class", "text")

			scriptID := script.Value("id").String().NotEmpty().Raw()

			eB.PATCH("/sharings/drives/"+sharingID+"/"+scriptID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
				  "data": {
				    "type": "io.cozy.files",
					"id": "` + scriptID + `",
					"attributes": {
					  "tags": ["foo", "bar", "baz"],
					  "updated_at": "2025-07-23T11:22:37.382Z",
					  "executable": true,
					  "encrypted": false,
					  "class": "text/x-shellscript"
					}
				  }
				}`)).
				Expect().Status(200)
		})
	})

	t.Run("SharedDriveFileCopy", func(t *testing.T) {
		t.Run("CannotCopyUnsharedFile", func(t *testing.T) {
			eA := httpexpect.Default(t, tsA.URL)
			eB := httpexpect.Default(t, tsB.URL)
			nonSharedID := eA.POST("/files/").
				WithQuery("Name", "Non shared file").
				WithQuery("Type", "file").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithBytes([]byte("foo")).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().Path("$.data.id").String().NotEmpty().Raw()

			eB.POST("/sharings/drives/"+sharingID+"/"+nonSharedID+"/copy").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithBytes([]byte("")).
				Expect().Status(403)
		})

		t.Run("CanCopySharedFile", func(t *testing.T) {
			eB := httpexpect.Default(t, tsB.URL)

			// 1. Create copy without name
			attrs := eB.POST("/sharings/drives/"+sharingID+"/"+checklistID+"/copy").
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithBytes([]byte("")).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().
				Path("$.data.attributes").Object()

			attrs.Value("name").String().IsEqual("Checklist (copy).txt")
			attrs.Value("driveId").String().IsEqual(sharingID)

			// 2. Create copy with same name
			eB.POST("/sharings/drives/"+sharingID+"/"+checklistID+"/copy").
				WithQuery("Name", "Checklist.txt").
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithBytes([]byte("")).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().
				Path("$.data.attributes.name").String().IsEqual("Checklist (2).txt")

			// 2. Create copy with different name
			eB.POST("/sharings/drives/"+sharingID+"/"+checklistID+"/copy").
				WithQuery("Name", "My list.txt").
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithBytes([]byte("")).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().
				Path("$.data.attributes.name").String().IsEqual("My list.txt")
		})
	})

	t.Run("ReadFileContentFromVersion", func(t *testing.T) {
		t.Run("CanReadVersionOfSharedFile", func(t *testing.T) {
			eA := httpexpect.Default(t, tsA.URL)
			eB := httpexpect.Default(t, tsB.URL)

			cfg := config.GetConfig()
			cfg.Fs.Versioning.MinDelayBetweenTwoVersions = 0
			cfg.Fs.Versioning.MaxNumberToKeep = 2

			groceriesID := eA.POST("/files/"+meetingsID).
				WithQuery("Name", "Groceries.txt").
				WithQuery("Type", "file").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithBytes([]byte("foo")).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().Path("$.data.id").String().NotEmpty().Raw()

			oldVersionID := eA.PUT("/files/"+groceriesID).
				WithHeader("Content-Type", "text/plain").
				WithHeader("Content-MD5", "YlBr401XTaSg0VimclPqmQ==").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithBytes([]byte("food")).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().Path("$.data.relationships.old_versions.data[0].id").String().NotEmpty().Raw()

			eB.GET("/sharings/drives/"+sharingID+"/download/"+oldVersionID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithBytes([]byte("")).
				Expect().Status(200).
				Body().IsEqual("foo")
		})

		t.Run("CannotReadVersionOfUnsharedFile", func(t *testing.T) {
			eA := httpexpect.Default(t, tsA.URL)
			eB := httpexpect.Default(t, tsB.URL)

			cfg := config.GetConfig()
			cfg.Fs.Versioning.MinDelayBetweenTwoVersions = 0
			cfg.Fs.Versioning.MaxNumberToKeep = 2

			groceriesID := eA.POST("/files/"+outsideOfShareID).
				WithQuery("Name", "Groceries.txt").
				WithQuery("Type", "file").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithBytes([]byte("foo")).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().Path("$.data.id").String().NotEmpty().Raw()

			oldVersionID := eA.PUT("/files/"+groceriesID).
				WithHeader("Content-Type", "text/plain").
				WithHeader("Content-MD5", "YlBr401XTaSg0VimclPqmQ==").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithBytes([]byte("food")).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().Path("$.data.relationships.old_versions.data[0].id").String().NotEmpty().Raw()

			eB.GET("/sharings/drives/"+sharingID+"/download/"+oldVersionID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				WithBytes([]byte("")).
				Expect().Status(403)
		})
	})

	t.Run("SharedDriveChangesFeed", func(t *testing.T) {
		// Request to GET /files/_changes using the given client and token
		getChangeFeedLocally := func(client clientInfo) *httpexpect.Response {
			return client.client.GET("/files/_changes").
				WithQuery("include_docs", true).
				WithHeader("Authorization", "Bearer "+client.token).
				Expect()
		}
		// Request to GET /sharings/drives/:sharing-id/_changes using the given client and token
		getChangeFeedBySharing := func(client clientInfo, sharingID string) *httpexpect.Response {
			return client.client.GET("/sharings/drives/"+sharingID+"/_changes").
				WithHeader("Authorization", "Bearer "+client.token).
				Expect()
		}
		// From the result of getChangeFeedBySharing, expect a successful response and return its results
		expectChangesResult := func(changesFromSharingResponse *httpexpect.Response) *httpexpect.Array {
			return changesFromSharingResponse.
				Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/json"}).
				Object().
				Value("results").
				Array()
		}
		// Create a matcher function to find a document by the given field and value
		makeDocFieldMatcherFn := func(field, value string) func(index int, value *httpexpect.Value) bool {
			return func(index int, change *httpexpect.Value) bool {
				return change.Object().Value(field).String().Raw() == value
			}
		}

		expectInChangesByDocId := func(changesFeedResults *httpexpect.Array, value string) *httpexpect.Object {
			return changesFeedResults.Find(makeDocFieldMatcherFn("id", value)).Object()
		}
		expectDeletionInChangesByDocId := func(changesFeedResults *httpexpect.Array, value string) {
			expectInChangesByDocId(changesFeedResults, value).Value("deleted").Boolean().IsTrue()
		}
		expectNotInChangesByDocId := func(changesFeedResults *httpexpect.Array, value string) {
			changesFeedResults.Filter(makeDocFieldMatcherFn("id", value)).IsEmpty()
		}

		t.Run("FilesChangesFeedAsExpectedForThisSetup", func(t *testing.T) {
			localChangesFeedResponse := getChangeFeedLocally(makeClientToActAsACMETheOwner(t))
			changes := expectChangesResult(localChangesFeedResponse)

			expectInChangesByDocId(changes, consts.RootDirID)
			expectInChangesByDocId(changes, outsideOfShareID)
			expectInChangesByDocId(changes, otherSharedFileThenTrashedID).Path("$.doc.trashed").Boolean().IsTrue()
			expectDeletionInChangesByDocId(changes, otherSharedFileThenDeletedID)
		})

		runTestAsACMEThenAgainAsBetty(t, func(t *testing.T, clientMaker clientMaker) {
			t.Run("RootIsNotInShared", func(t *testing.T) {
				changesFeedResponse := getChangeFeedBySharing(clientMaker(t), sharingID)
				changes := expectChangesResult(changesFeedResponse)

				expectNotInChangesByDocId(changes, consts.RootDirID)
			})
			t.Run("UnsharedOtherRootFileAndTrashedShouldBeDeleted", func(t *testing.T) {
				changesFeedResponse := getChangeFeedBySharing(clientMaker(t), sharingID)
				changes := expectChangesResult(changesFeedResponse)

				expectInChangesByDocId(changes, checklistID)
				expectDeletionInChangesByDocId(changes, outsideOfShareID)
				expectDeletionInChangesByDocId(changes, otherSharedFileThenDeletedID)
				expectDeletionInChangesByDocId(changes, otherSharedFileThenTrashedID)
				expectInChangesByDocId(changes, meetingsID).Path("$.doc.driveId").String().IsEqual(sharingID)
				expectInChangesByDocId(changes, checklistID).Path("$.doc.driveId").String().IsEqual(sharingID)
			})
		})

		t.Run("ReadFileContentFromIDHandler", func(t *testing.T) {
			// Request to GET /sharings/drives/:sharing-id/_changes using the given client and token
			downloadFile := func(client clientInfo, sharingID string, fileID string) *httpexpect.Response {
				return client.client.GET("/sharings/drives/"+sharingID+"/download/"+fileID).
					WithHeader("Authorization", "Bearer "+client.token).
					Expect()
			}

			t.Run("CanReceiveContentOfSharedFile", func(t *testing.T) {
				// Download the file
				res := downloadFile(makeClientToActAsBettyTheRecipient(t), sharingID, checklistID).
					Status(200)

				// Check the response headers
				res.Header("Content-Disposition").HasPrefix("inline")
				res.Header("Content-Disposition").Contains(`filename="` + checklistName + `"`)
				res.Header("Content-Type").IsEqual("text/plain")
				res.Header("Etag").NotEmpty()
				res.Header("Content-Length").IsEqual("3")
				res.Body().IsEqual("foo")
			})
		})

		// Test two-step download endpoints:
		// 1. POST FileDownloadCreateHandler
		// 2. GET FileDownloadHandler
		t.Run("DownloadsEndpoints", func(t *testing.T) {
			ci := makeClientToActAsBettyTheRecipient(t)

			// Create the two-step download using the file ID
			related := ci.client.POST("/sharings/drives/"+sharingID+"/downloads").
				WithQuery("Id", checklistID).
				WithHeader("Authorization", "Bearer "+ci.token).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().Path("$.links.related").String().NotEmpty().Raw()

			// GET the file via the returned related link (inline)
			res := ci.client.GET(related).
				WithHeader("Authorization", "Bearer "+ci.token).
				Expect().Status(200)
			res.Header("Content-Disposition").IsEqual(`inline; filename="` + checklistName + `"`)

			// GET the file via the returned related link (attachment)
			res = ci.client.GET(related).
				WithQuery("Dl", "1").
				WithHeader("Authorization", "Bearer "+ci.token).
				Expect().Status(200)
			res.Header("Content-Disposition").IsEqual(`attachment; filename="` + checklistName + `"`)

			// GET the file as anonymous user via the returned link
			anon := makeClientToActAsAnonymous(t)
			res = anon.client.GET(related).
				WithQuery("Dl", "1").
				Expect().Status(200)
			res.Header("Content-Disposition").IsEqual(`attachment; filename="` + checklistName + `"`)
		})
	})

	t.Run("RealtimeInSharedDrive", func(t *testing.T) {
		eA := httpexpect.Default(t, tsA.URL)
		eB := httpexpect.Default(t, tsB.URL)

		ws := eB.GET("/sharings/drives/" + sharingID + "/realtime").
			WithWebsocketUpgrade().
			Expect().Status(http.StatusSwitchingProtocols).
			Websocket()
		defer ws.Disconnect()

		ws.WriteText(fmt.Sprintf(`{"method": "AUTH", "payload": "%s"}`, bettyAppToken))
		time.Sleep(50 * time.Millisecond)

		newFileID := eA.POST("/files/"+meetingsID).
			WithQuery("Name", "Realtime test file.txt").
			WithQuery("Type", "file").
			WithHeader("Content-Type", "text/plain").
			WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			WithBytes([]byte("foo")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		obj := ws.Expect().TextMessage().JSON().Object()
		obj.HasValue("event", "CREATED")
		payload := obj.Value("payload").Object()
		payload.HasValue("type", consts.Files)
		payload.HasValue("id", newFileID)
	})

	t.Run("CreateAndOpenNote", func(t *testing.T) {
		eA := httpexpect.Default(t, tsA.URL)
		eB := httpexpect.Default(t, tsB.URL)

		t.Run("CreateNoteInSharedDrive", func(t *testing.T) {
			// Create a note in the shared drive as the owner
			obj := eA.POST("/sharings/drives/"+sharingID+"/notes").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
					"data": {
						"type": "io.cozy.notes.documents",
						"attributes": {
							"title": "Meeting Minutes",
							"dir_id": "` + meetingsID + `",
							"schema": {
								"nodes": [
									["doc", { "content": "block+" }],
									["paragraph", { "content": "inline*", "group": "block" }],
									["text", { "group": "inline" }],
									["bullet_list", { "content": "list_item+", "group": "block" }],
									["list_item", { "content": "paragraph block*" }]
								],
								"marks": [
									["em", {}],
									["strong", {}]
								],
								"topNode": "doc"
							}
						}
					}
				}`)).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()

			data := obj.Value("data").Object()
			data.HasValue("type", "io.cozy.files")
			noteID := data.Value("id").String().NotEmpty().Raw()

			attrs := data.Value("attributes").Object()
			attrs.HasValue("type", "file")
			attrs.HasValue("name", "Meeting Minutes.cozy-note")
			attrs.HasValue("mime", "text/vnd.cozy.note+markdown")

			meta := attrs.Value("metadata").Object()
			meta.HasValue("title", "Meeting Minutes")
			meta.HasValue("version", 0)
			meta.Value("schema").Object().NotEmpty()
			meta.Value("content").Object().NotEmpty()

			t.Run("OpenNoteFromSharedDrive", func(t *testing.T) {
				obj := eB.GET("/sharings/drives/"+sharingID+"/notes/"+noteID+"/open").
					WithHeader("Authorization", "Bearer "+bettyAppToken).
					Expect().Status(200).
					JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
					Object()

				data := obj.Value("data").Object()
				data.HasValue("type", consts.NotesURL)
				data.HasValue("id", noteID)

				attrs := data.Value("attributes").Object()
				attrs.HasValue("note_id", noteID)
				attrs.HasValue("subdomain", "nested")
				attrs.HasValue("instance", acmeInstance.Domain)
				attrs.Value("public_name").String().NotEmpty()
				attrs.Value("sharecode").String().NotEmpty()
			})
		})

		t.Run("CreateNoteWithoutAuthentication", func(t *testing.T) {
			eB.POST("/sharings/drives/"+sharingID+"/notes").
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
					"data": {
						"type": "io.cozy.notes.documents",
						"attributes": {
							"title": "Unauthorized Note",
							"dir_id": "` + meetingsID + `"
						}
					}
				}`)).
				Expect().Status(401)
		})
	})

	t.Run("RevokeRecipientAccess", func(t *testing.T) {
		eA := httpexpect.Default(t, tsA.URL)
		eB := httpexpect.Default(t, tsB.URL)

		t.Run("RecipientCanInitiallyAccessSharedDrive", func(t *testing.T) {
			eB.GET("/sharings/drives/"+sharingID+"/"+checklistID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object().
				Path("$.data.attributes.name").String().IsEqual("Checklist.txt")
			obj := eB.GET("/sharings/drives").
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()
			obj.Value("data").Array().Length().IsEqual(1)
		})

		t.Run("RemoveRecipientFromSharing", func(t *testing.T) {
			eA.DELETE("/sharings/"+sharingID+"/recipients/1").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				Expect().Status(204)
		})

		t.Run("RevokedRecipientCannotAccessSharedDrive", func(t *testing.T) {
			eB.GET("/sharings/drives/"+sharingID+"/"+checklistID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(403)
			eB.GET("/sharings/drives/"+sharingID+"/metadata").
				WithQuery("Path", "/Product team/Meetings/Checklist.txt").
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(403)
			eB.GET("/sharings/drives/"+sharingID+"/_changes").
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(403)
			obj := eB.GET("/sharings/drives").
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()
			obj.Value("data").Array().Length().IsEqual(0)
		})
	})

	t.Run("CreateAnotherSharedDriveAndAccessIt", func(t *testing.T) {
		// Create a new shared drive with custom name
		secondName := "Marketing team" + strings.ReplaceAll(t.Name(), "/", "-")
		secondDesc := "Drive for the marketing team"
		sid, rootID, disco := createSharedDriveForAcme(t, acmeInstance, acmeAppToken, tsA.URL, secondName, secondDesc)
		// Keep local variables to avoid interfering with global firstSharingID/productDirID
		secondSharingID := sid
		secondRootDirID := rootID

		// Accept it as Betty
		acceptSharedDriveForBetty(t, bettyInstance, tsA.URL, tsB.URL, secondSharingID, disco)

		// Fetch a fresh discovery link for Dave and accept as Dave (read-only)
		_, discoDave := extractInvitationLink(t, acmeInstance, "ACME")
		acceptSharedDrive(t, daveInstance, tsA.URL, tsD.URL, secondSharingID, discoDave)

		// ACME uploads a file into this new shared drive
		eA := httpexpect.Default(t, tsA.URL)
		fileID := eA.POST("/files/"+secondRootDirID).
			WithQuery("Name", "welcome.txt").
			WithQuery("Type", "file").
			WithHeader("Content-Type", "text/plain").
			WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			WithBytes([]byte("foo")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		// Betty can GET the file through shared drives endpoint
		eB := httpexpect.Default(t, tsB.URL)
		obj := eB.GET("/sharings/drives/"+secondSharingID+"/"+fileID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		data := obj.Value("data").Object()
		data.Value("type").String().IsEqual("io.cozy.files")
		data.Value("id").String().IsEqual(fileID)
		attrs := data.Value("attributes").Object()
		attrs.Value("type").String().IsEqual("file")
		attrs.Value("name").String().IsEqual("welcome.txt")

		// Dave can read; verify access
		eD := httpexpect.Default(t, tsD.URL)
		// Dave can GET the same file
		eD.GET("/sharings/drives/"+secondSharingID+"/"+fileID).
			WithHeader("Authorization", "Bearer "+daveAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().IsEqual(fileID)
		// Dave can POST to create a new file in the shared drive (current behavior)
		createdID := eD.POST("/sharings/drives/"+secondSharingID+"/"+secondRootDirID).
			WithQuery("Name", "dave-note.txt").
			WithQuery("Type", "file").
			WithHeader("Content-Type", "text/plain").
			WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
			WithHeader("Authorization", "Bearer "+daveAppToken).
			WithBytes([]byte("foo")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		// Dave can also rename metadata on his created file (current behavior)
		eD.PATCH("/sharings/drives/"+secondSharingID+"/"+createdID).
			WithHeader("Authorization", "Bearer "+daveAppToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
			  "data": {
			    "type": "io.cozy.files",
				"id": "` + createdID + `",
				"attributes": {"name": "renamed-by-dave.txt"}
			  }
			}`)).
			Expect().Status(200)

		// Dave can delete his file in the shared drive (current behavior)
		eD.DELETE("/sharings/drives/"+secondSharingID+"/"+createdID).
			WithHeader("Authorization", "Bearer "+daveAppToken).
			Expect().Status(200)

		// And Dave cannot overwrite file content in the shared drive
		eD.PUT("/sharings/drives/"+secondSharingID+"/"+createdID).
			WithHeader("Authorization", "Bearer "+daveAppToken).
			WithHeader("Content-Type", "text/plain").
			WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
			WithBytes([]byte("foo")).
			Expect().Status(403)
	})
}

func mockAcmeClient(uA *url.URL) func(u *url.URL, bearer string) *client.Client {
	return func(u *url.URL, bearer string) *client.Client {
		tr := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, uA.Host)
			},
		}
		c := &client.Client{
			Scheme:    uA.Scheme,
			Addr:      uA.Host,
			Domain:    u.Hostname(),
			Transport: tr,
		}
		if bearer != "" {
			c.Authorizer = &request.BearerAuthorizer{Token: bearer}
		}
		return c
	}
}
