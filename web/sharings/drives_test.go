package sharings_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"

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
	return client.POST("/files/"+parentDirID).
		WithQuery("Name", name).
		WithQuery("Type", "directory").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(201).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object().Path("$.data.id").String().NotEmpty().Raw()
}

// createRootDirectory creates a directory with the given name at the root level
// and returns the directory ID
func createRootDirectory(t *testing.T, client *httpexpect.Expect, name, token string) string {
	return createDirectory(t, client, "", name, token)
}

// createFile creates a file with the given name in the specified parent directory
// and returns the file ID
func createFile(t *testing.T, client *httpexpect.Expect, parentDirID, name, token string) string {
	return client.POST("/files/"+parentDirID).
		WithQuery("Name", name).
		WithQuery("Type", "file").
		WithHeader("Content-Type", "text/plain").
		WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
		WithHeader("Authorization", "Bearer "+token).
		WithBytes([]byte("foo")).
		Expect().Status(201).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object().Path("$.data.id").String().NotEmpty().Raw()
}

// moveFileDownstream moves a file from the source instance to the target directory
func moveFileDownstream(t *testing.T, client *httpexpect.Expect, sharingID, targetDirID, sourceInstanceURL, sourceFileID, token string) *httpexpect.Response {
	return client.POST("/sharings/drives/"+sharingID+"/"+targetDirID+"/downstream").
		WithQuery("source-instance", sourceInstanceURL).
		WithQuery("file-id", sourceFileID).
		WithHeader("Authorization", "Bearer "+token).
		Expect()
}

// moveFileUpstream moves a file from the source instance to the target directory
func moveFileUpstream(t *testing.T, client *httpexpect.Expect, sharingID, fileID, destInstanceURL, targetDirID, token string) *httpexpect.Response {
	return client.POST("/sharings/drives/"+sharingID+"/"+fileID+"/upstream").
		WithQuery("dest-instance", destInstanceURL).
		WithQuery("dir-id", targetDirID).
		WithHeader("Authorization", "Bearer "+token).
		Expect()
}

// verifyFileMove verifies that a file was moved successfully by checking its attributes
func verifyFileMove(t *testing.T, inst *instance.Instance, fileID, expectedName, expectedDirID string, expectedContent string) {
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
	_, err := inst.VFS().FileByID(fileID)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

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
		"/sharings": sharings.Routes,
	})
	tsB.Config.Handler.(*echo.Echo).Renderer = render
	tsB.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsB.Close)

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
		eA := httpexpect.Default(t, tsA.URL)

		// Prepare a few things
		contact := createContact(t, acmeInstance, "Betty", "betty@example.net")
		require.NotNil(t, contact)
		daveContact := createContact(t, acmeInstance, "Dave", "dave@example.net")
		require.NotNil(t, daveContact)
		productID = eA.POST("/files/").
			WithQuery("Name", "Product team").
			WithQuery("Type", "directory").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()
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

		eA.DELETE("/files/"+otherSharedFileThenDeletedID).
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			Expect().Status(200)

		// Empty Trash
		eA.DELETE("/files/trash").
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

		eA.DELETE("/files/"+otherSharedFileThenTrashedID).
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			Expect().Status(200)

		// Create a shared drive
		obj := eA.POST("/sharings/").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "` + consts.Sharings + `",
          "attributes": {
            "description": "Drive for the product team",
            "drive": true,
            "rules": [{
                "title": "Product team",
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

		description := assertInvitationMailWasSent(t, acmeInstance, "ACME")
		assert.Equal(t, description, "Drive for the product team")
		assert.Contains(t, discoveryLink, "/discovery?state=")
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
		// Betty login
		eB := httpexpect.Default(t, tsB.URL)
		token := eB.GET("/auth/login").
			Expect().Status(200).
			Cookie("_csrf").Value().NotEmpty().Raw()
		eB.POST("/auth/login").
			WithCookie("_csrf", token).
			WithFormField("passphrase", "MyPassphrase").
			WithFormField("csrf_token", token).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Contains("home")

		// Betty goes to the discovery link
		u, err := url.Parse(discoveryLink)
		assert.NoError(t, err)
		state = u.Query()["state"][0]

		// Take only the path and query from the `disoveryLink` and redirect
		// to the tsA host.
		eA := httpexpect.Default(t, tsA.URL)
		eA.GET(u.Path).
			WithQuery("state", state).
			Expect().Status(200).
			HasContentType("text/html", "utf-8").
			Body().
			Contains("Connect to your Twake").
			Contains(`<input type="hidden" name="state" value="` + state)

		redirectHeader := eA.POST(u.Path).
			WithFormField("state", state).
			WithFormField("slug", tsB.URL).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(302).Header("Location")

		redirectHeader.Contains(tsB.URL)
		redirectHeader.Contains("/auth/authorize/sharing")
		authorizeLink = redirectHeader.NotEmpty().Raw()

		FakeOwnerInstance(t, bettyInstance, tsA.URL)

		u, err = url.Parse(authorizeLink)
		assert.NoError(t, err)
		sharingID = u.Query()["sharing_id"][0]
		state := u.Query()["state"][0]
		redirectHeader = eB.GET(u.Path).
			WithQuery("sharing_id", sharingID).
			WithQuery("state", state).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).Header("Location")

		redirectHeader.Contains("#/folder/io.cozy.files.shared-drives-dir")

		// TODO check the rules/members/credentials of the sharing on the recipient
		// TODO check that a shortcut has been created
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

		// Move a file from one shared drive to another using the downstream endpoint
		t.Run("MoveBetweenSharedDrivesDownstream", func(t *testing.T) {
			eA := httpexpect.Default(t, tsA.URL)

			// Create a second directory to be shared as another drive
			secondRootDirID := createRootDirectory(t, eA, "Marketing team", acmeAppToken)

			// Create a contact for the recipient and share the second directory
			contact2 := createContact(t, acmeInstance, "Betty Marketing", "betty.marketing@example.net")
			require.NotNil(t, contact2)
			sharing2 := eA.POST("/sharings/").
				WithHeader("Authorization", "Bearer "+acmeAppToken).
				WithHeader("Content-Type", "application/vnd.api+json").
				WithBytes([]byte(`{
			  "data": {
			    "type": "` + consts.Sharings + `",
			    "attributes": {
			      "description": "Drive for the marketing team",
			      "drive": true,
			      "rules": [{
			          "title": "Marketing team",
			          "doctype": "` + consts.Files + `",
			          "values": ["` + secondRootDirID + `"]
			        }]
			    },
			    "relationships": {
			      "recipients": {
			        "data": [{"id": "` + contact2.ID() + `", "type": "` + contact2.DocType() + `"}]
			      }
			    }
			  }
			}`)).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()
			sharingID2 := sharing2.Path("$.data.id").String().NotEmpty().Raw()
			// Ensure we got a valid sharing id before proceeding
			require.NotEmpty(t, sharingID2)

			// Create a file inside the first shared drive that we'll move to the second
			sourceFileID := createFile(t, eA, meetingsID, "to-move-between-drives.txt", acmeAppToken)

			// Move downstream from the first drive to the second drive's root
			res := moveFileDownstream(t, eA, sharingID2, secondRootDirID,
				"https://"+acmeInstance.Domain, sourceFileID, acmeAppToken).Status(201)

			// Verify JSON:API and destination
			obj := res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).Object()
			obj.Path("$.data.type").String().IsEqual("io.cozy.files")
			obj.Path("$.data.attributes.name").String().IsEqual("to-move-between-drives.txt")
			obj.Path("$.data.attributes.dir_id").String().IsEqual(secondRootDirID)

			// Verify file exists in destination and content preserved
			movedFileID := obj.Path("$.data.id").String().Raw()
			verifyFileMove(t, acmeInstance, movedFileID, "to-move-between-drives.txt", secondRootDirID, "foo")

			// Verify original file is deleted
			verifyFileDeleted(t, acmeInstance, sourceFileID)
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

		t.Run("DownloadFile", func(t *testing.T) {
			// Request to GET /sharings/drives/:sharing-id/_changes using the given client and token
			downloadFile := func(client clientInfo, sharingID string, fileID string) *httpexpect.Response {
				return client.client.GET("/sharings/drives/"+sharingID+"/download/"+fileID).
					WithHeader("Authorization", "Bearer "+client.token).
					Expect()
			}
			t.Run("DownloadFile", func(t *testing.T) {
				// Download the file
				res := downloadFile(makeClientToActAsBettyTheRecipient(t), sharingID, checklistID).
					Status(200)

				// Check the response headers
				res.Header("Content-Disposition").HasPrefix("inline")
				res.Header("Content-Disposition").Contains(`filename="` + checklistName + `"`)
				res.Header("Content-Type").Equal("text/plain")
				res.Header("Etag").NotEmpty()
				res.Header("Content-Length").Equal("3")
				res.Body().Equal("foo")
			})

			// Test two-step download endpoints: POST /downloads then GET /downloads/:secret/:fake-name
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
			})
		})

		t.Run("MoveDownstreamHandler", func(t *testing.T) {
			var fileToMoveSameStack string
			var fileToMoveDifferentStack string
			// Test missing parameters
			t.Run("CreateSharedDataToMoveDownstream", func(t *testing.T) {
				eA := httpexpect.Default(t, tsA.URL)

				//as a target directory to move the file use Product directory "productID"
				fileToMoveSameStack = createFile(t, eA, outsideOfShareID, "file-to-upload.txt", acmeAppToken)
				fileToMoveDifferentStack = createFile(t, eA, outsideOfShareID, "file-to-upload-diff.txt", acmeAppToken)
			})

			t.Run("MissingSourceInstance", func(t *testing.T) {
				res := httpexpect.Default(t, tsA.URL).POST("/sharings/drives/"+sharingID+"/"+productID+"/downstream").
					WithQuery("file-id", fileToMoveSameStack).
					Expect().Status(400)

				res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
					Object().Path("$.errors[0].detail").String().Contains("missing source-instance param")
			})

			t.Run("MissingFileID", func(t *testing.T) {
				res := httpexpect.Default(t, tsA.URL).POST("/sharings/drives/"+sharingID+"/"+productID+"/downstream").
					WithQuery("source-instance", "https://"+acmeInstance.Domain).
					Expect().Status(400)

				res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
					Object().Path("$.errors[0].detail").String().Contains("missing file-id parameter")
			})

			t.Run("FileNotFound", func(t *testing.T) {
				_ = moveFileDownstream(t, httpexpect.Default(t, tsA.URL), sharingID, productID,
					"https://"+acmeInstance.Domain, "non-existent-file", "").Status(404)
			})

			t.Run("DirectoryNotFound", func(t *testing.T) {
				_ = moveFileDownstream(t, httpexpect.Default(t, tsA.URL), sharingID, "non-existent-dir",
					"https://"+acmeInstance.Domain, fileToMoveSameStack, "").Status(404)
			})

			t.Run("SuccessfulMove", func(t *testing.T) {
				// Perform the move operation
				eA := httpexpect.Default(t, tsA.URL)
				res := moveFileDownstream(t, eA, sharingID, productID,
					"https://"+acmeInstance.Domain, fileToMoveSameStack, acmeAppToken).Status(201)

				// Verify the response contains the new file
				responseObj := res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).Object()
				responseObj.Path("$.data.type").String().IsEqual("io.cozy.files")
				responseObj.Path("$.data.attributes.name").String().IsEqual("file-to-upload.txt")
				responseObj.Path("$.data.attributes.dir_id").String().IsEqual(productID)

				// Verify the file was moved and content preserved
				movedFileID := responseObj.Path("$.data.id").String().Raw()
				verifyFileMove(t, acmeInstance, movedFileID, "file-to-upload.txt", productID, "foo")

				// Verify the original file was deleted
				verifyFileDeleted(t, acmeInstance, fileToMoveSameStack)
			})

			// Force the cross-stack path even if instances are on the same server
			t.Run("SuccessfulMoveForcedDifferentStack", func(t *testing.T) {
				// mock to perform http call to different stack
				prev := sharings.OnSameStackCheck
				defer func() { sharings.OnSameStackCheck = prev }()
				sharings.OnSameStackCheck = func(_, _ *instance.Instance) bool { return false }

				// mock remote client to route *.cozy.local to the local test server while preserving Host
				prevClient := sharings.NewRemoteClient
				defer func() { sharings.NewRemoteClient = prevClient }()
				uA, _ := url.Parse(tsA.URL)
				sharings.NewRemoteClient = func(u *url.URL, bearer string) *client.Client {
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

				eA := httpexpect.Default(t, tsA.URL)
				res := moveFileDownstream(t, eA, sharingID, productID,
					"https://"+acmeInstance.Domain, fileToMoveDifferentStack, acmeAppToken).Status(201)

				responseObj := res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).Object()
				responseObj.Path("$.data.type").String().IsEqual("io.cozy.files")
				responseObj.Path("$.data.attributes.name").String().IsEqual("file-to-upload-diff.txt")
				responseObj.Path("$.data.attributes.dir_id").String().IsEqual(productID)

				// Verify the file was moved and content preserved
				movedFileID := responseObj.Path("$.data.id").String().Raw()
				verifyFileMove(t, acmeInstance, movedFileID, "file-to-upload-diff.txt", productID, "foo")

				// Verify the original file was deleted
				verifyFileDeleted(t, acmeInstance, fileToMoveDifferentStack)
			})
		})

		t.Run("MoveUpstreamHandler", func(t *testing.T) {
			var fileToMoveUpstream string
			// Test missing parameters
			t.Run("CreateSharedDataToMoveUpstream", func(t *testing.T) {
				eA := httpexpect.Default(t, tsA.URL)

				// Create a file in the shared directory that we'll move upstream
				fileToMoveUpstream = createFile(t, eA, meetingsID, "file-to-move-upstream.txt", acmeAppToken)
			})

			t.Run("MissingDestInstance", func(t *testing.T) {
				res := httpexpect.Default(t, tsA.URL).POST("/sharings/drives/"+sharingID+"/"+fileToMoveUpstream+"/upstream").
					WithQuery("dir-id", productID).
					Expect().Status(400)

				res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
					Object().Path("$.errors[0].detail").String().Contains("missing dest-instance parameter")
			})

			t.Run("MissingDirID", func(t *testing.T) {
				res := httpexpect.Default(t, tsA.URL).POST("/sharings/drives/"+sharingID+"/"+fileToMoveUpstream+"/upstream").
					WithQuery("dest-instance", "https://"+bettyInstance.Domain).
					Expect().Status(400)

				res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
					Object().Path("$.errors[0].detail").String().Contains("missing dir-id parameter")
			})

			t.Run("FileNotFound", func(t *testing.T) {
				_ = moveFileUpstream(t, httpexpect.Default(t, tsA.URL), sharingID, "non-existent-file",
					"https://"+bettyInstance.Domain, productID, "").Status(404)
			})

			t.Run("SuccessfulMoveToSameStack", func(t *testing.T) {
				eA := httpexpect.Default(t, tsA.URL)

				// Create a test file to move
				testFileID := createFile(t, eA, meetingsID, "test-upstream-move.txt", acmeAppToken)

				// Create destination directory on the target instance
				destDirID := createRootDirectory(t, eA, "Destination Dir", acmeAppToken)

				// Perform the upstream move operation (same-stack scenario)
				res := moveFileUpstream(t, eA, sharingID, testFileID,
					"https://"+acmeInstance.Domain, destDirID, acmeAppToken).Status(201)

				// Verify the response contains the new file
				responseObj := res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).Object()
				responseObj.Path("$.data.type").String().IsEqual("io.cozy.files")
				responseObj.Path("$.data.attributes.name").String().IsEqual("test-upstream-move.txt")
				responseObj.Path("$.data.attributes.dir_id").String().IsEqual(destDirID)

				// Verify the file was moved to the destination
				movedFileID := responseObj.Path("$.data.id").String().Raw()
				verifyFileMove(t, acmeInstance, movedFileID, "test-upstream-move.txt", destDirID, "foo")

				// Verify the original file was deleted from source
				verifyFileDeleted(t, acmeInstance, testFileID)
			})

			// Force the cross-stack path even if instances are on the same server
			t.Run("SuccessfulMoveForcedDifferentStack", func(t *testing.T) {
				// Create destination directory on the target (owner) instance
				eA := httpexpect.Default(t, tsA.URL)
				destDirID := createRootDirectory(t, eA, "Destination Dir Forced", acmeAppToken)

				// mock to perform http call to different stack
				prev := sharings.OnSameStackCheck
				defer func() { sharings.OnSameStackCheck = prev }()
				sharings.OnSameStackCheck = func(_, _ *instance.Instance) bool { return false }

				// mock remote client to route *.cozy.local to the local test server while preserving Host
				prevClient := sharings.NewRemoteClient
				defer func() { sharings.NewRemoteClient = prevClient }()
				uA, _ := url.Parse(tsA.URL)
				sharings.NewRemoteClient = func(u *url.URL, bearer string) *client.Client {
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

				// Perform the upstream move operation (forced cross-stack scenario)
				res := moveFileUpstream(t, eA, sharingID, fileToMoveUpstream,
					"https://"+acmeInstance.Domain, destDirID, acmeAppToken).Status(201)

				// Verify the response contains the new file
				responseObj := res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).Object()
				responseObj.Path("$.data.type").String().IsEqual("io.cozy.files")
				responseObj.Path("$.data.attributes.name").String().IsEqual("file-to-move-upstream.txt")
				responseObj.Path("$.data.attributes.dir_id").String().IsEqual(destDirID)

				// Verify the file was moved to the destination
				movedFileID := responseObj.Path("$.data.id").String().Raw()
				verifyFileMove(t, acmeInstance, movedFileID, "file-to-move-upstream.txt", destDirID, "foo")

				// Verify the original file was deleted from source
				verifyFileDeleted(t, acmeInstance, fileToMoveUpstream)
			})

			t.Run("InvalidFileParameter", func(t *testing.T) {
				res := httpexpect.Default(t, tsA.URL).POST("/sharings/drives/"+sharingID+"//upstream").
					WithQuery("dest-instance", "https://"+bettyInstance.Domain).
					WithQuery("dir-id", productID).
					Expect().Status(400)

				res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
					Object().Path("$.errors[0].detail").String().Contains("missing file-id parameter")
			})

			t.Run("DirectoryInsteadOfFile", func(t *testing.T) {
				// Try to move a directory using the file move endpoint
				_ = moveFileUpstream(t, httpexpect.Default(t, tsA.URL), sharingID, meetingsID,
					"https://"+bettyInstance.Domain, productID, acmeAppToken).Status(404)
			})
		})
	})
}
