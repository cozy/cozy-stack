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

// createClients creates httpexpect clients for the current test with proper error reporting
func (env *sharedDrivesEnv) createClients(t *testing.T) (*httpexpect.Expect, *httpexpect.Expect, *httpexpect.Expect) {
	t.Helper()
	eA := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  env.tsA.URL,
		Reporter: httpexpect.NewRequireReporter(t),
		Printers: []httpexpect.Printer{httpexpect.NewCompactPrinter(t)},
	})
	eB := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  env.tsB.URL,
		Reporter: httpexpect.NewRequireReporter(t),
		Printers: []httpexpect.Printer{httpexpect.NewCompactPrinter(t)},
	})
	eD := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  env.tsD.URL,
		Reporter: httpexpect.NewRequireReporter(t),
		Printers: []httpexpect.Printer{httpexpect.NewCompactPrinter(t)},
	})
	return eA, eB, eD
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

	// Create initial shared drive and accept as Betty; create common dirs
	sharingID, firstRootDirID, disco := createSharedDriveForAcme(t, acme, acmeToken, tsA.URL,
		"One More Shared Drive "+crypto.GenerateRandomString(1000), "One more Shared drive description")
	acceptSharedDriveForBetty(t, betty, tsA.URL, tsB.URL, sharingID, disco)

	// Create temporary clients for initial setup
	eA := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  tsA.URL,
		Reporter: httpexpect.NewRequireReporter(t),
		Printers: []httpexpect.Printer{httpexpect.NewCompactPrinter(t)},
	})

	productDirID := createDirectory(t, eA, firstRootDirID, "Product", acmeToken)
	meetingsDirID := createDirectory(t, eA, firstRootDirID, "Meetings", acmeToken)

	return &sharedDrivesEnv{
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
	firstSharingID := env.firstSharingID
	productDirID := env.productDirID
	meetingsDirID := env.meetingsDirID

	t.Run("SuccessfulMove_ToSharedDrive_SameStack", func(t *testing.T) {
		_, eB, _ := env.createClients(t)
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
		eA, eB, _ := env.createClients(t)
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
		eA, eB, _ := env.createClients(t)
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
		eA, eB, _ := env.createClients(t)
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
		eA, eB, _ := env.createClients(t)
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
		eA, eB, _ := env.createClients(t)
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
		eA, eB, _ := env.createClients(t)
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

	// File move with conflict resolution: moving a file between shared drives (same stack)
	// when a file with the same name already exists at destination should auto-rename.
	t.Run("AutoRename_MoveFile_BetweenSharedDrives_SameStack_NameExists", func(t *testing.T) {
		eA, eB, _ := env.createClients(t)

		// Prepare: create another shared drive and accept it as Betty (source drive)
		secondSharingID, secondRootDirID, disco := createSharedDriveForAcme(t, acmeInstance, acmeAppToken, tsA.URL,
			"AutoRename Target Drive", "Drive used as source for auto-rename move")
		acceptSharedDriveForBetty(t, bettyInstance, tsA.URL, tsB.URL, secondSharingID, disco)

		// Create destination file with the conflicting name in the first shared drive (target)
		conflictName := "conflict-name.txt"
		_ = createFile(t, eA, meetingsDirID, conflictName, acmeAppToken)

		// Create source file with the same name in the second shared drive (source)
		sourceFileID := createFile(t, eA, secondRootDirID, conflictName, acmeAppToken)

		// Attempt to move → should succeed with auto-renaming
		responseObj := eB.POST("/sharings/drives/move").
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
			  "source": {
			    "instance": "https://` + acmeInstance.Domain + `",
			    "sharing_id": "` + secondSharingID + `",
			    "file_id": "` + sourceFileID + `"
			  },
			  "dest": {
			    "instance": "https://` + acmeInstance.Domain + `",
			    "sharing_id": "` + firstSharingID + `",
			    "dir_id": "` + meetingsDirID + `"
			  }
			}`)).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		// Verify the file was moved with a renamed name (conflict resolution)
		movedFileName := responseObj.Path("$.data.attributes.name").String().Raw()
		require.NotEqual(t, conflictName, movedFileName)
		require.Contains(t, movedFileName, "conflict-name")
		require.Contains(t, movedFileName, " (2)")

		// Verify both files exist in destination (original + renamed)
		// Get the actual path of the meetings directory
		meetingsDir, err := acmeInstance.VFS().DirByID(meetingsDirID)
		require.NoError(t, err)

		_, err = acmeInstance.VFS().FileByPath(meetingsDir.Fullpath + "/" + conflictName)
		require.NoError(t, err)
		_, err = acmeInstance.VFS().FileByPath(meetingsDir.Fullpath + "/" + movedFileName)
		require.NoError(t, err)

		// Verify source file was deleted
		verifyFileDeleted(t, acmeInstance, sourceFileID)
	})

	// Move directory from Betty's local drive to a shared drive
	t.Run("MoveDirectoryWithFilesAndChild_LocalToSharedDrive_SameStack", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		// Create a directory with files and subdirectories in Betty's local drive
		localDirID := createDirectory(t, eB, "", "LocalTestDir", bettyAppToken)
		_ = createFile(t, eB, localDirID, "local-file1.txt", bettyAppToken)
		_ = createFile(t, eB, localDirID, "local-file2.md", bettyAppToken)
		subDirID := createDirectory(t, eB, localDirID, "LocalSubDir", bettyAppToken)
		_ = createFile(t, eB, subDirID, "local-file3.bin", bettyAppToken)

		// Move the directory to the shared drive
		responseObj := eB.POST("/sharings/drives/move").
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
			  "source": {
			    "dir_id": "` + localDirID + `"
			  },
			  "dest": {
			    "instance": "https://` + acmeInstance.Domain + `",
			    "sharing_id": "` + firstSharingID + `",
			    "dir_id": "` + meetingsDirID + `"
			  }
			}`)).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		// Verify the directory was moved successfully
		// Note: The response type is "io.cozy.files" for both files and directories
		require.Equal(t, "io.cozy.files", responseObj.Path("$.data.type").String().Raw())
		require.Equal(t, "LocalTestDir", responseObj.Path("$.data.attributes.name").String().Raw())
		require.Equal(t, "directory", responseObj.Path("$.data.attributes.type").String().Raw())

		// Get the actual path of the meetings directory
		meetingsDir, err := acmeInstance.VFS().DirByID(meetingsDirID)
		require.NoError(t, err)

		// Verify the moved directory exists in the shared drive
		movedDir, err := acmeInstance.VFS().DirByPath(meetingsDir.Fullpath + "/LocalTestDir")
		require.NoError(t, err)
		require.Equal(t, "LocalTestDir", movedDir.DocName)

		// Verify all files were moved to the shared drive
		_, err = acmeInstance.VFS().FileByPath(meetingsDir.Fullpath + "/LocalTestDir/local-file1.txt")
		require.NoError(t, err)
		_, err = acmeInstance.VFS().FileByPath(meetingsDir.Fullpath + "/LocalTestDir/local-file2.md")
		require.NoError(t, err)

		// Verify subdirectory and its file were moved
		_, err = acmeInstance.VFS().DirByPath(meetingsDir.Fullpath + "/LocalTestDir/LocalSubDir")
		require.NoError(t, err)
		_, err = acmeInstance.VFS().FileByPath(meetingsDir.Fullpath + "/LocalTestDir/LocalSubDir/local-file3.bin")
		require.NoError(t, err)

		// Verify the original directory was deleted from Betty's local drive
		verifyFileDeleted(t, bettyInstance, localDirID)
	})

	// Move directory from a shared drive to Betty's local drive
	t.Run("MoveDirectoryWithFilesAndChild_SharedDriveToLocal_SameStack", func(t *testing.T) {
		eA, eB, _ := env.createClients(t)

		// Create a directory with files and subdirectories in the shared drive
		sharedDirID := createDirectory(t, eA, meetingsDirID, "SharedTestDir", acmeAppToken)
		_ = createFile(t, eA, sharedDirID, "shared-file1.txt", acmeAppToken)
		_ = createFile(t, eA, sharedDirID, "shared-file2.md", acmeAppToken)
		subDirID := createDirectory(t, eA, sharedDirID, "SharedSubDir", acmeAppToken)
		_ = createFile(t, eA, subDirID, "shared-file3.bin", acmeAppToken)

		// Create a destination directory in Betty's local drive
		destDirID := createRootDirectory(t, eB, "Betty Dest Dir", bettyAppToken)

		// Move the directory to Betty's local drive
		responseObj := eB.POST("/sharings/drives/move").
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
			  "source": {
			    "instance": "https://` + acmeInstance.Domain + `",
			    "sharing_id": "` + firstSharingID + `",
			    "dir_id": "` + sharedDirID + `"
			  },
			  "dest": {
			    "dir_id": "` + destDirID + `"
			  }
			}`)).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		// Verify the directory was moved successfully
		// Note: The response type is "io.cozy.files" for both files and directories
		require.Equal(t, "io.cozy.files", responseObj.Path("$.data.type").String().Raw())
		require.Equal(t, "SharedTestDir", responseObj.Path("$.data.attributes.name").String().Raw())
		require.Equal(t, "directory", responseObj.Path("$.data.attributes.type").String().Raw())

		// Get the actual path of the destination directory
		destDir, err := bettyInstance.VFS().DirByID(destDirID)
		require.NoError(t, err)

		// Verify the moved directory exists in Betty's local drive
		movedDir, err := bettyInstance.VFS().DirByPath(destDir.Fullpath + "/SharedTestDir")
		require.NoError(t, err)
		require.Equal(t, "SharedTestDir", movedDir.DocName)

		// Verify all files were moved to Betty's local drive
		_, err = bettyInstance.VFS().FileByPath(destDir.Fullpath + "/SharedTestDir/shared-file1.txt")
		require.NoError(t, err)
		_, err = bettyInstance.VFS().FileByPath(destDir.Fullpath + "/SharedTestDir/shared-file2.md")
		require.NoError(t, err)

		// Verify subdirectory and its file were moved
		_, err = bettyInstance.VFS().DirByPath(destDir.Fullpath + "/SharedTestDir/SharedSubDir")
		require.NoError(t, err)
		_, err = bettyInstance.VFS().FileByPath(destDir.Fullpath + "/SharedTestDir/SharedSubDir/shared-file3.bin")
		require.NoError(t, err)

		// Verify the original directory was deleted from the shared drive
		verifyFileDeleted(t, acmeInstance, sharedDirID)
	})

	t.Run("MoveDirectoryWithFilesAndChild_BetweenSharedDrives_SameStack", func(t *testing.T) {
		eA, eB, _ := env.createClients(t)
		// Prepare: create a second shared drive and accept it as Betty
		secondSharingID, secondRootDirID, disco := createSharedDriveForAcme(t, acmeInstance, acmeAppToken, tsA.URL,
			"NestedDir Move Target Drive", "Drive used as destination for nested dir move")
		acceptSharedDriveForBetty(t, bettyInstance, tsA.URL, tsB.URL, secondSharingID, disco)

		// Prepare: create a directory with files and a child directory with files in the first drive
		srcDirID := createDirectory(t, eA, productDirID, "FolderToMove", acmeAppToken)
		_ = createFile(t, eA, srcDirID, "A1.txt", acmeAppToken)
		_ = createFile(t, eA, srcDirID, "B1.md", acmeAppToken)
		_ = createFile(t, eA, srcDirID, "C1.bin", acmeAppToken)
		childDirID := createDirectory(t, eA, srcDirID, "SubFolder", acmeAppToken)
		_ = createFile(t, eA, childDirID, "A2.txt", acmeAppToken)
		// add deeper hierarchy
		deepDirID := createDirectory(t, eA, childDirID, "Deep", acmeAppToken)
		_ = createFile(t, eA, deepDirID, "D1.txt", acmeAppToken)

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

		// Verify using VFS on owner instance (IDs are not preserved; check by names and paths)
		// 1) Destination now contains a directory named like the source root
		destRoot, err := acmeInstance.VFS().DirByID(destDirID)
		require.NoError(t, err)
		movedRoot, err := acmeInstance.VFS().DirByPath(destRoot.Fullpath + "/" + "FolderToMove")
		require.NoError(t, err)
		require.Equal(t, "FolderToMove", movedRoot.DocName)

		// 2) Moved root contains multiple files and subdir SubFolder
		_, err = acmeInstance.VFS().FileByPath(movedRoot.Fullpath + "/A1.txt")
		require.NoError(t, err)
		_, err = acmeInstance.VFS().FileByPath(movedRoot.Fullpath + "/B1.md")
		require.NoError(t, err)
		_, err = acmeInstance.VFS().FileByPath(movedRoot.Fullpath + "/C1.bin")
		require.NoError(t, err)
		childDir, err := acmeInstance.VFS().DirByPath(movedRoot.Fullpath + "/SubFolder")
		require.NoError(t, err)

		// 3) Child dir contains A2.txt and nested Deep/D1.txt
		_, err = acmeInstance.VFS().FileByPath(childDir.Fullpath + "/A2.txt")
		require.NoError(t, err)
		deepDir, err := acmeInstance.VFS().DirByPath(childDir.Fullpath + "/Deep")
		require.NoError(t, err)
		_, err = acmeInstance.VFS().FileByPath(deepDir.Fullpath + "/D1.txt")
		require.NoError(t, err)

		// 4) Original source path no longer exists
		productRoot, err := acmeInstance.VFS().DirByID(productDirID)
		require.NoError(t, err)
		_, err = acmeInstance.VFS().DirByPath(productRoot.Fullpath + "/FolderToMove")
		require.Error(t, err)
	})

	// Validation errors for Move endpoint
	t.Run("BadRequest_MissingSourceFileID", func(t *testing.T) {
		_, eB, _ := env.createClients(t)
		// missing source.file_id
		_ = postMoveExpectStatus(t, eB, bettyAppToken, `{
			  "source": {},
			  "dest": {"dir_id": "`+productDirID+`"}
		}`, 400)
	})

	t.Run("BadRequest_MissingDestDirID", func(t *testing.T) {
		_, eB, _ := env.createClients(t)
		fileID := createFile(t, eB, "", "file-missing-dest.txt", bettyAppToken)
		_ = postMoveExpectStatus(t, eB, bettyAppToken, `{
			  "source": {"file_id": "`+fileID+`"},
			  "dest": {}
		}`, 400)
	})

	t.Run("BadRequest_SourceInstanceWithoutSharingID", func(t *testing.T) {
		_, eB, _ := env.createClients(t)
		fileID := createFile(t, eB, "", "file-src-no-share.txt", bettyAppToken)
		_ = postMoveExpectStatus(t, eB, bettyAppToken, `{
			  "source": {"instance": "https://`+acmeInstance.Domain+`", "file_id": "`+fileID+`"},
			  "dest": {"dir_id": "`+productDirID+`"}
		}`, 400)
	})

	t.Run("BadRequest_DestInstanceWithoutSharingID", func(t *testing.T) {
		_, eB, _ := env.createClients(t)
		fileID := createFile(t, eB, "", "file-dest-no-share.txt", bettyAppToken)
		_ = postMoveExpectStatus(t, eB, bettyAppToken, `{
			  "source": {"file_id": "`+fileID+`"},
			  "dest": {"instance": "https://`+acmeInstance.Domain+`", "dir_id": "`+productDirID+`"}
		}`, 400)
	})

	t.Run("BadRequest_NoSharingIDsProvided", func(t *testing.T) {
		_, eB, _ := env.createClients(t)
		fileID := createFile(t, eB, "", "file-no-shares.txt", bettyAppToken)
		_ = postMoveExpectStatus(t, eB, bettyAppToken, `{
			  "source": {"file_id": "`+fileID+`"},
			  "dest": {"dir_id": "`+productDirID+`"}
		}`, 400)
	})

	// Dave is a read-only member; he must not be able to move files to a shared drive
	t.Run("PermissionDeniedWithoutShare_ToSharedDrive", func(t *testing.T) {
		_, _, eD := env.createClients(t)
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
		eA, _, eD := env.createClients(t)
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
