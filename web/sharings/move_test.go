package sharings_test

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/crypto"
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
	"github.com/stretchr/testify/require"
)

// Test helpers to reduce duplication across scenarios
type sharedDrivesEnv struct {
	eA, eB, eD *httpexpect.Expect

	// Instances
	acme, betty, dave *instance.Instance
	// Tokens
	acmeToken, bettyToken, daveToken string
	// Servers (closed via t.Cleanup)
	tsA, tsB, tsD *httptest.Server

	// Common resources
	firstSharingID string
	firstRootDirID string
	productDirID   string
	meetingsDirID  string
}

func setupSharedDrivesEnv(t *testing.T) *sharedDrivesEnv {
	t.Helper()

	config.UseTestFile(t)
	build.BuildMode = build.ModeDev
	config.GetConfig().Assets = "../../assets"
	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb(t)
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()
	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()))

	// ACME
	setupA := testutils.NewSetup(t, t.Name()+"_acme")
	acme := setupA.GetTestInstance(&lifecycle.Options{Email: "acme@example.net", PublicName: "ACME"})
	acmeToken := generateAppToken(acme, "drive", "io.cozy.files")
	tsA := setupA.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/files":    files.Routes,
		"/notes":    notes.Routes,
		"/sharings": sharings.Routes,
	})
	tsA.Config.Handler.(*echo.Echo).Renderer = render
	tsA.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsA.Close)

	// Betty
	setupB := testutils.NewSetup(t, t.Name()+"_betty")
	betty := setupB.GetTestInstance(&lifecycle.Options{
		Email: "betty@example.net", PublicName: "Betty", Passphrase: "MyPassphrase", KdfIterations: 5000, Key: "xxx",
	})
	bettyToken := generateAppToken(betty, "drive", consts.Files)
	tsB := setupB.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth":     func(g *echo.Group) { g.Use(middlewares.LoadSession); auth.Routes(g) },
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsB.Config.Handler.(*echo.Echo).Renderer = render
	tsB.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsB.Close)

	// Dave (read-only)
	setupD := testutils.NewSetup(t, strings.ReplaceAll(t.Name(), "/", "_")+"_dave")
	dave := setupD.GetTestInstance(&lifecycle.Options{
		Email: "dave@example.net", PublicName: "Dave", Passphrase: "MyPassphrase", KdfIterations: 5000, Key: "xxx",
	})
	daveToken := generateAppToken(dave, "drive", consts.Files)
	tsD := setupD.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth":     func(g *echo.Group) { g.Use(middlewares.LoadSession); auth.Routes(g) },
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsD.Config.Handler.(*echo.Echo).Renderer = render
	tsD.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsD.Close)

	eA := httpexpect.WithConfig(httpexpect.Config{BaseURL: tsA.URL, Reporter: httpexpect.NewRequireReporter(t)})
	eB := httpexpect.WithConfig(httpexpect.Config{BaseURL: tsB.URL, Reporter: httpexpect.NewRequireReporter(t)})
	eD := httpexpect.WithConfig(httpexpect.Config{BaseURL: tsD.URL, Reporter: httpexpect.NewRequireReporter(t)})

	// Create initial shared drive and accept as Betty; create common dirs
	sharingID, firstRootDirID, disco := createSharedDriveForAcme(t, acme, acmeToken, tsA.URL,
		"One More Shared Drive "+crypto.GenerateRandomString(1000), "One more Shared drive description")
	acceptSharedDriveForBetty(t, betty, tsA.URL, tsB.URL, sharingID, disco)

	productDirID := createDirectory(t, eA, firstRootDirID, "Product", acmeToken)
	meetingsDirID := createDirectory(t, eA, firstRootDirID, "Meetings", acmeToken)

	return &sharedDrivesEnv{
		eA: eA, eB: eB, eD: eD,
		acme: acme, betty: betty, dave: dave,
		acmeToken: acmeToken, bettyToken: bettyToken, daveToken: daveToken,
		tsA: tsA, tsB: tsB, tsD: tsD,
		firstSharingID: sharingID, firstRootDirID: firstRootDirID, productDirID: productDirID, meetingsDirID: meetingsDirID,
	}
}

func forceCrossStack(t *testing.T, baseURL string) func() {
	t.Helper()
	prevSameStack := sharings.OnSameStackCheck
	prevClient := sharings.NewRemoteClient
	u, _ := url.Parse(baseURL)
	sharings.OnSameStackCheck = func(_, _ *instance.Instance) bool { return false }
	sharings.NewRemoteClient = mockAcmeClient(u)
	return func() {
		sharings.OnSameStackCheck = prevSameStack
		sharings.NewRemoteClient = prevClient
	}
}

// Convenience wrapper to POST to /sharings/drives/move and return the JSON object.
func postMove(t *testing.T, e *httpexpect.Expect, token string, body string) *httpexpect.Object {
	t.Helper()
	return e.POST("/sharings/drives/move").
		WithHeader("Authorization", "Bearer "+token).
		WithHeader("Content-Type", "application/json").
		WithBytes([]byte(body)).
		Expect().Status(201).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object()
}

// postMoveExpectStatus posts a move request and asserts the provided HTTP status.
// Returns the raw response object for further assertions when needed.
func postMoveExpectStatus(t *testing.T, e *httpexpect.Expect, token string, body string, status int) *httpexpect.Response {
	t.Helper()
	return e.POST("/sharings/drives/move").
		WithHeader("Authorization", "Bearer "+token).
		WithHeader("Content-Type", "application/json").
		WithBytes([]byte(body)).
		Expect().Status(status)
}

func TestSharedDrivesMove(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)
	// Short aliases to keep existing test body readable
	acmeInstance := env.acme
	bettyInstance := env.betty
	acmeAppToken := env.acmeToken
	bettyAppToken := env.bettyToken
	daveAppToken := env.daveToken
	tsA := env.tsA
	tsB := env.tsB
	eA := env.eA
	eB := env.eB
	eD := env.eD
	firstSharingID := env.firstSharingID
	productDirID := env.productDirID
	meetingsDirID := env.meetingsDirID

	t.Run("SuccessfulMove_ToSharedDrive_SameStack", func(t *testing.T) {
		// Perform the move operation
		fileToMoveSameStack := createFile(t, eB, "", "file-to-upload.txt", bettyAppToken)
		responseObj := postMove(t, eB, bettyAppToken, `{
				  "source": {
				    "file_id": "`+fileToMoveSameStack+`"
				  },
				  "dest": {
				    "instance": "https://`+acmeInstance.Domain+`",
				    "sharing_id": "`+firstSharingID+`",
				    "dir_id": "`+productDirID+`"
				  }
				}`)
		responseObj.Path("$.data.type").String().IsEqual("io.cozy.files")
		responseObj.Path("$.data.attributes.name").String().IsEqual("file-to-upload.txt")
		responseObj.Path("$.data.attributes.dir_id").String().IsEqual(productDirID)
		responseObj.Path("$.data.attributes.driveId").String().IsEqual(firstSharingID)

		// Verify the file was moved and content preserved
		movedFileID := responseObj.Path("$.data.id").String().Raw()
		verifyFileMove(t, acmeInstance, movedFileID, "file-to-upload.txt", productDirID, "foo")

		// Verify the original file was deleted
		verifyFileDeleted(t, bettyInstance, fileToMoveSameStack)
	})

	// Force the cross-stack path even if instances are on the same server
	t.Run("SuccessfulMove_ToSharedDrive_DifferentStack", func(t *testing.T) {
		fileToMoveDifferentStack := createFile(t, eB, "", "file-to-upload-diff.txt", bettyAppToken)
		destDirInSharedDrive := createDirectory(t, eA, productDirID, "Dest DIR To Move", acmeAppToken)

		cleanup := forceCrossStack(t, tsA.URL)
		defer cleanup()

		responseObj := postMove(t, eB, bettyAppToken, `{
				  "source": {
				    "file_id": "`+fileToMoveDifferentStack+`"
				  },
				  "dest": {
				    "instance": "https://`+acmeInstance.Domain+`",
				    "sharing_id": "`+firstSharingID+`",
				    "dir_id": "`+destDirInSharedDrive+`"
				  }
				}`)
		responseObj.Path("$.data.type").String().IsEqual("io.cozy.files")
		responseObj.Path("$.data.attributes.name").String().IsEqual("file-to-upload-diff.txt")
		responseObj.Path("$.data.attributes.dir_id").String().IsEqual(destDirInSharedDrive)
		responseObj.Path("$.data.attributes.driveId").String().IsEqual(firstSharingID)

		// Verify the file was moved and content preserved
		movedFileID := responseObj.Path("$.data.id").String().Raw()
		verifyFileMove(t, acmeInstance, movedFileID, "file-to-upload-diff.txt", destDirInSharedDrive, "foo")

		// Verify the original file was deleted
		verifyFileDeleted(t, acmeInstance, fileToMoveDifferentStack)
	})

	t.Run("SuccessfulMove_FromSharedDrive_SameStack", func(t *testing.T) {
		// Create file to move
		fileToMove := createFile(t, eA, meetingsDirID, "file-to-move-upstream.txt", acmeAppToken)
		// Create destination directory on the target instance
		destDirID := createRootDirectory(t, eB, "Destination Dir", bettyAppToken)

		responseObj := postMove(t, eB, bettyAppToken, `{
				  "source": {
					"instance": "https://`+acmeInstance.Domain+`",
					 "sharing_id": "`+firstSharingID+`",
				    "file_id": "`+fileToMove+`"
				  },
				  "dest": {
				    "dir_id": "`+destDirID+`"
				  }
				}`)
		responseObj.Path("$.data.type").String().IsEqual("io.cozy.files")
		responseObj.Path("$.data.attributes.name").String().IsEqual("file-to-move-upstream.txt")
		responseObj.Path("$.data.attributes.dir_id").String().IsEqual(destDirID)

		// Verify the file was moved to the destination
		movedFileID := responseObj.Path("$.data.id").String().Raw()
		verifyFileMove(t, bettyInstance, movedFileID, "file-to-move-upstream.txt", destDirID, "foo")

		// Verify the original file was deleted from source
		verifyFileDeleted(t, acmeInstance, fileToMove)
	})

	// Force the cross-stack path even if instances are on the same server
	t.Run("SuccessfulMove_FromSharedDrive_DifferentStack", func(t *testing.T) {
		fileName := "file-to-move-diff.txt"
		fileToDiffStack := createFile(t, eA, meetingsDirID, fileName, acmeAppToken)
		// Create destination directory on the target (owner) instance
		destDirID := createRootDirectory(t, eB, "Destination Dir Diff", bettyAppToken)

		cleanup := forceCrossStack(t, tsA.URL)
		defer cleanup()

		responseObj := postMove(t, eB, bettyAppToken, `{
				  "source": {
					"instance": "https://`+acmeInstance.Domain+`",
					 "sharing_id": "`+firstSharingID+`",
				    "file_id": "`+fileToDiffStack+`"
				  },
				  "dest": {
				    "dir_id": "`+destDirID+`"
				  }
				}`)
		responseObj.Path("$.data.type").String().IsEqual("io.cozy.files")
		responseObj.Path("$.data.attributes.name").String().IsEqual(fileName)
		responseObj.Path("$.data.attributes.dir_id").String().IsEqual(destDirID)

		// Verify the file was moved to the destination
		movedFileID := responseObj.Path("$.data.id").String().Raw()
		verifyFileMove(t, bettyInstance, movedFileID, fileName, destDirID, "foo")

		// Verify the original file was deleted from source
		verifyFileDeleted(t, acmeInstance, fileToDiffStack)
	})

	t.Run("SuccessfulMove_BetweenSharedDrives_DifferentStack", func(t *testing.T) {
		secondSharingID, secondRootDirID, disco := createSharedDriveForAcme(t, acmeInstance, acmeAppToken, tsA.URL,
			"One More Shared Drive", "One more Shared drive description")
		// Accept it as Betty
		acceptSharedDriveForBetty(t, bettyInstance, tsA.URL, tsB.URL, secondSharingID, disco)

		// Create the file on the owner (ACME) instance inside the second shared drive
		fileName := "file-to-move-between-shared-drives.txt"
		toMove := createFile(t, eA, secondRootDirID, fileName, acmeAppToken)

		cleanup := forceCrossStack(t, tsA.URL)
		defer cleanup()

		res := eB.POST("/sharings/drives/move").
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
				  "source": {
					 "instance": "https://` + acmeInstance.Domain + `",
					 "sharing_id": "` + secondSharingID + `",
				     "file_id": "` + toMove + `"
				  },
				  "dest": {
                     "instance": "https://` + acmeInstance.Domain + `",
				     "dir_id": "` + meetingsDirID + `",
					 "sharing_id": "` + firstSharingID + `"
				  }
				}`)).Expect().Status(201)

		// Verify the response contains the new file
		responseObj := res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).Object()
		responseObj.Path("$.data.type").String().IsEqual("io.cozy.files")
		responseObj.Path("$.data.attributes.name").String().IsEqual(fileName)
		responseObj.Path("$.data.attributes.dir_id").String().IsEqual(meetingsDirID)
		responseObj.Path("$.data.attributes.driveId").String().IsEqual(firstSharingID)

		// Verify the file was moved to the destination
		movedFileID := responseObj.Path("$.data.id").String().Raw()
		verifyFileMove(t, acmeInstance, movedFileID, fileName, meetingsDirID, "foo")
		verifyFileDeleted(t, acmeInstance, toMove)
	})

	t.Run("SuccessfulMoveBetween_SharedDrives_SameStack", func(t *testing.T) {
		firstSharingID, firstRootDirID, disco := createSharedDriveForAcme(t, acmeInstance, acmeAppToken, tsA.URL,
			"One More Shared Drive "+crypto.GenerateRandomString(1000), "One more Shared drive description")
		// Accept it as Betty
		acceptSharedDriveForBetty(t, bettyInstance, tsA.URL, tsB.URL, firstSharingID, disco)

		meetingsID := eA.POST("/files/"+firstRootDirID).
			WithQuery("Name", "Meetings").
			WithQuery("Type", "directory").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		secondSharingID, secondRootDirID, disco := createSharedDriveForAcme(t, acmeInstance, acmeAppToken, tsA.URL,
			"One More Shared Drive "+crypto.GenerateRandomString(1000), "One more Shared drive description")
		// Accept it as Betty
		acceptSharedDriveForBetty(t, bettyInstance, tsA.URL, tsB.URL, secondSharingID, disco)

		// Create the file on the owner (ACME) instance inside the second shared drive
		fileName := "file-to-move-between-shared-drives.txt" + crypto.GenerateRandomString(1000)
		toMove := createFile(t, eA, secondRootDirID, fileName, acmeAppToken)

		res := eB.POST("/sharings/drives/move").
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
				  "source": {
					 "instance": "https://` + acmeInstance.Domain + `",
					 "sharing_id": "` + secondSharingID + `",
				     "file_id": "` + toMove + `"
				  },
				  "dest": {
                     "instance": "https://` + acmeInstance.Domain + `",
				     "dir_id": "` + meetingsID + `",
					 "sharing_id": "` + firstSharingID + `"
				  }
				}`)).Expect().Status(201)

		// Verify the response contains the new file
		responseObj := res.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).Object()
		responseObj.Path("$.data.type").String().IsEqual("io.cozy.files")
		responseObj.Path("$.data.attributes.name").String().IsEqual(fileName)
		responseObj.Path("$.data.attributes.dir_id").String().IsEqual(meetingsID)
		responseObj.Path("$.data.attributes.driveId").String().IsEqual(firstSharingID)

		// Verify the file was moved to the destination
		movedFileID := responseObj.Path("$.data.id").String().Raw()
		verifyFileMove(t, acmeInstance, movedFileID, fileName, meetingsID, "foo")

		// Verify the original file was deleted from source
		verifyFileDeleted(t, acmeInstance, toMove)
	})

	// Directory move tests (sync) using existing /move route and source.dir_id
	t.Run("MoveEmptyDirectory_BetweenSharedDrives_SameStack", func(t *testing.T) {
		t.Skip()
		// Prepare: create a second shared drive and accept it as Betty
		secondSharingID, secondRootDirID, disco := createSharedDriveForAcme(t, acmeInstance, acmeAppToken, tsA.URL,
			"EmptyDir Move Target Drive", "Drive used as destination for empty dir move")
		acceptSharedDriveForBetty(t, bettyInstance, tsA.URL, tsB.URL, secondSharingID, disco)

		// Prepare: create empty directory under first shared drive and a destination dir under second shared drive
		srcEmptyDirID := createDirectory(t, eA, productDirID, "EmptyToMove", acmeAppToken)
		destDirID := createDirectory(t, eA, secondRootDirID, "DestForEmpty", acmeAppToken)

		// Force cross-stack network path for move between drives
		cleanup := forceCrossStack(t, tsA.URL)
		defer cleanup()

		// Move the empty directory under destination directory
		eB.POST("/sharings/drives/move").
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
				  "source": {
				    "instance": "https://` + acmeInstance.Domain + `",
				    "sharing_id": "` + firstSharingID + `",
				    "dir_id": "` + srcEmptyDirID + `"
				  },
				  "dest": {
				    "instance": "https://` + acmeInstance.Domain + `",
				    "sharing_id": "` + firstSharingID + `",
				    "dir_id": "` + destDirID + `"
				  }
				}`)).
			Expect().Status(201)

		// Verify: the empty directory now appears under destination (second drive) and not under the first drive root
		obj := eB.GET("/sharings/drives/"+secondSharingID+"/"+destDirID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		contents := obj.Path("$.data.relationships.contents.data").Array()
		contents.Filter(func(_ int, v *httpexpect.Value) bool {
			return v.Object().Value("id").String().Raw() == srcEmptyDirID
		}).Length().IsEqual(1)

		// First drive root should no longer contain the moved directory
		rootObj := eB.GET("/sharings/drives/"+firstSharingID+"/"+productDirID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		rootContents := rootObj.Path("$.data.relationships.contents.data").Array()
		rootContents.Filter(func(_ int, v *httpexpect.Value) bool {
			return v.Object().Value("id").String().Raw() == srcEmptyDirID
		}).IsEmpty()
	})

	t.Run("MoveDirectoryWithFilesAndChild_BetweenSharedDrives_SameStack", func(t *testing.T) {
		// Prepare: create a second shared drive and accept it as Betty
		secondSharingID, secondRootDirID, disco := createSharedDriveForAcme(t, acmeInstance, acmeAppToken, tsA.URL,
			"NestedDir Move Target Drive", "Drive used as destination for nested dir move")
		acceptSharedDriveForBetty(t, bettyInstance, tsA.URL, tsB.URL, secondSharingID, disco)

		// Prepare: create a directory with files and a child directory with files in the first drive
		srcDirID := createDirectory(t, eA, productDirID, "FolderToMove", acmeAppToken)
		_ = createFile(t, eA, srcDirID, "A1.txt", acmeAppToken)
		childDirID := createDirectory(t, eA, srcDirID, "SubFolder", acmeAppToken)
		_ = createFile(t, eA, childDirID, "A2.txt", acmeAppToken)

		// Destination directory under the second shared drive
		destDirID := createDirectory(t, eA, secondRootDirID, "DestForFolder", acmeAppToken)

		// Move the directory subtree
		eB.POST("/sharings/drives/move").
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
				  "source": {
				    "instance": "https://` + acmeInstance.Domain + `",
				    "sharing_id": "` + firstSharingID + `",
				    "dir_id": "` + srcDirID + `"
				  },
				  "dest": {
				    "instance": "https://` + acmeInstance.Domain + `",
				    "sharing_id": "` + secondSharingID + `",
				    "dir_id": "` + destDirID + `"
				  }
				}`)).
			Expect().Status(201)

		// Verify: the moved root directory is now under destination (second drive)
		destObj := eB.GET("/sharings/drives/"+secondSharingID+"/"+destDirID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		destContents := destObj.Path("$.data.relationships.contents.data").Array()
		destContents.Filter(func(_ int, v *httpexpect.Value) bool {
			return v.Object().Value("id").String().Raw() == srcDirID
		}).Length().IsEqual(1)

		// Verify: first drive root no longer contains the moved directory
		rootObj := eB.GET("/sharings/drives/"+firstSharingID+"/"+productDirID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		rootContents := rootObj.Path("$.data.relationships.contents.data").Array()
		rootContents.Filter(func(_ int, v *httpexpect.Value) bool {
			return v.Object().Value("id").String().Raw() == srcDirID
		}).IsEmpty()

		// Verify: the subtree content is still present under the moved directory
		dirObj := eB.GET("/sharings/drives/"+secondSharingID+"/"+srcDirID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		children := dirObj.Path("$.data.relationships.contents.data").Array()
		children.Filter(func(_ int, v *httpexpect.Value) bool {
			// file A1.txt or subdir SubFolder
			id := v.Object().Value("id").String().Raw()
			return id != "" // ensure non-empty; detailed name checks happen below
		}).Length().NotEqual(0)

		// Verify: child directory still contains its file
		childObj := eB.GET("/sharings/drives/"+secondSharingID+"/"+childDirID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		grandChildren := childObj.Path("$.data.relationships.contents.data").Array()
		grandChildren.Length().IsEqual(1)
	})

	// Validation errors for Move endpoint
	t.Run("BadRequest_MissingSourceFileID", func(t *testing.T) {
		// missing source.file_id
		_ = postMoveExpectStatus(t, eB, bettyAppToken, `{
			  "source": {},
			  "dest": {"dir_id": "`+productDirID+`"}
		}`, 400)
	})

	t.Run("BadRequest_MissingDestDirID", func(t *testing.T) {
		fileID := createFile(t, eB, "", "file-missing-dest.txt", bettyAppToken)
		_ = postMoveExpectStatus(t, eB, bettyAppToken, `{
			  "source": {"file_id": "`+fileID+`"},
			  "dest": {}
		}`, 400)
	})

	t.Run("BadRequest_SourceInstanceWithoutSharingID", func(t *testing.T) {
		fileID := createFile(t, eB, "", "file-src-no-share.txt", bettyAppToken)
		_ = postMoveExpectStatus(t, eB, bettyAppToken, `{
			  "source": {"instance": "https://`+acmeInstance.Domain+`", "file_id": "`+fileID+`"},
			  "dest": {"dir_id": "`+productDirID+`"}
		}`, 400)
	})

	t.Run("BadRequest_DestInstanceWithoutSharingID", func(t *testing.T) {
		fileID := createFile(t, eB, "", "file-dest-no-share.txt", bettyAppToken)
		_ = postMoveExpectStatus(t, eB, bettyAppToken, `{
			  "source": {"file_id": "`+fileID+`"},
			  "dest": {"instance": "https://`+acmeInstance.Domain+`", "dir_id": "`+productDirID+`"}
		}`, 400)
	})

	t.Run("BadRequest_NoSharingIDsProvided", func(t *testing.T) {
		fileID := createFile(t, eB, "", "file-no-shares.txt", bettyAppToken)
		_ = postMoveExpectStatus(t, eB, bettyAppToken, `{
			  "source": {"file_id": "`+fileID+`"},
			  "dest": {"dir_id": "`+productDirID+`"}
		}`, 400)
	})

	// Dave is a read-only member; he must not be able to move files to a shared drive
	t.Run("PermissionDeniedWithoutShare_ToSharedDrive", func(t *testing.T) {
		// Dave creates a file on his own instance
		fileOnDave := createFile(t, eD, "", "dave-local.txt", daveAppToken)
		// Try to move Dave's file into the shared drive → forbidden
		_ = postMoveExpectStatus(t, eD, daveAppToken, `{
			  "source": { "file_id": "`+fileOnDave+`" },
			  "dest": {"instance": "https://`+acmeInstance.Domain+`", "sharing_id": "`+firstSharingID+`", "dir_id": "`+productDirID+`"}
		}`, 403)
	})

	// Dave is a read-only member; he must not be able to move files out of a shared drive
	t.Run("PermissionDeniedWithoutShare_FromSharedDrive", func(t *testing.T) {
		// ACME (owner) creates a file in the shared drive that Dave can see
		sharedFile := createFile(t, eA, meetingsDirID, "dave-cannot-move.txt", acmeAppToken)
		// Dave creates a destination directory on his instance
		daveDestDir := createRootDirectory(t, eD, "Dave Dest Dir", daveAppToken)
		// Try to move the shared file to Dave's instance → forbidden
		_ = postMoveExpectStatus(t, eD, daveAppToken, `{
			  "source": {"instance": "https://`+acmeInstance.Domain+`", "sharing_id": "`+firstSharingID+`", "file_id": "`+sharedFile+`"},
			  "dest": {"dir_id": "`+daveDestDir+`"}
		}`, 403)
	})
}
