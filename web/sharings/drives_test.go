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
	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
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

// DriveCreationMethod represents how a shared drive is created in tests.
type DriveCreationMethod string

const (
	// DriveCreationMethodLegacy uses POST /sharings/ with drive:true and manual rules.
	DriveCreationMethodLegacy DriveCreationMethod = "legacy"
	// DriveCreationMethodFromFolder uses POST /sharings/drives with folder_id.
	DriveCreationMethodFromFolder DriveCreationMethod = "from_folder"
)

// RecipientInfo describes a recipient for a shared drive.
type RecipientInfo struct {
	Name     string
	Email    string
	ReadOnly bool
}

// createDirectory creates a directory with the given name in the specified parent directory
// and returns the directory ID.
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
// and returns the directory ID.
func createRootDirectory(t *testing.T, client *httpexpect.Expect, name, token string) string {
	return createDirectory(t, client, "", name, token)
}

// createFile creates a file with the given name in the specified parent directory
// and returns the file ID.
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

// verifyFileMove verifies that a file was moved successfully by checking its attributes.
func verifyFileMove(t *testing.T, inst *instance.Instance, fileID, expectedName, expectedDirID string, expectedContent string) {
	t.Helper()
	fs := inst.VFS()

	fileDoc, err := fs.FileByID(fileID)
	require.NoError(t, err)
	require.Equal(t, expectedName, fileDoc.DocName)
	require.Equal(t, expectedDirID, fileDoc.DirID)
	require.Equal(t, int64(len(expectedContent)), fileDoc.ByteSize)

	fileHandle, err := fs.OpenFile(fileDoc)
	require.NoError(t, err)
	defer fileHandle.Close()

	content, err := io.ReadAll(fileHandle)
	require.NoError(t, err)
	require.Equal(t, expectedContent, string(content))
}

// verifyFileDeleted verifies that a file was deleted from the source instance.
func verifyFileDeleted(t *testing.T, inst *instance.Instance, fileID string) {
	t.Helper()
	_, err := inst.VFS().FileByID(fileID)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

// verifyNodeDeleted verifies that a node (file or directory) was deleted from the source instance.
func verifyNodeDeleted(t *testing.T, inst *instance.Instance, id string) {
	t.Helper()
	if _, err := inst.VFS().FileByID(id); err == nil {
		require.Fail(t, "expected file to be deleted, but it still exists")
	} else if os.IsNotExist(err) {
		return
	}
	if _, err := inst.VFS().DirByID(id); err == nil {
		require.Fail(t, "expected directory to be deleted, but it still exists")
	} else {
		require.True(t, os.IsNotExist(err))
	}
}

// DefaultRecipients returns the default Betty+Dave recipients for tests.
func DefaultRecipients() []RecipientInfo {
	return []RecipientInfo{
		{Name: "Betty", Email: "betty@example.net", ReadOnly: false},
		{Name: "Dave", Email: "dave@example.net", ReadOnly: true},
	}
}

// createSharedDrive creates a shared drive using the specified method.
// If recipients is nil, it uses DefaultRecipients().
// It returns the created sharing ID and the shared root directory ID.
func createSharedDrive(
	t *testing.T,
	method DriveCreationMethod,
	inst *instance.Instance,
	appToken string,
	tsURL string,
	driveName string,
	description string,
	recipients []RecipientInfo,
) (
	sharingID string,
	productID string,
	discovery string,
) {
	t.Helper()

	e := httpexpect.Default(t, tsURL)

	// Use default recipients if none provided
	if recipients == nil {
		recipients = DefaultRecipients()
	}

	// Create contacts and build relationship arrays
	var rwRecipientRefs, roRecipientRefs []string
	for _, r := range recipients {
		c := createContact(t, inst, r.Name, r.Email)
		require.NotNil(t, c)
		ref := `{"id": "` + c.ID() + `", "type": "` + c.DocType() + `"}`
		if r.ReadOnly {
			roRecipientRefs = append(roRecipientRefs, ref)
		} else {
			rwRecipientRefs = append(rwRecipientRefs, ref)
		}
	}

	// Create the directory that will be shared
	productID = e.POST("/files/").
		WithQuery("Name", driveName).
		WithQuery("Type", "directory").
		WithHeader("Authorization", "Bearer "+appToken).
		Expect().Status(201).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object().Path("$.data.id").String().NotEmpty().Raw()

	var obj *httpexpect.Object

	// Build relationships JSON
	rwRecipientsJSON := "[" + strings.Join(rwRecipientRefs, ",") + "]"
	roRecipientsJSON := "[" + strings.Join(roRecipientRefs, ",") + "]"

	switch method {
	case DriveCreationMethodLegacy:
		// Use POST /sharings/ with drive:true and manual rules
		obj = e.POST("/sharings/").
			WithHeader("Authorization", "Bearer "+appToken).
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
							"data": ` + rwRecipientsJSON + `
						},
						"read_only_recipients": {
							"data": ` + roRecipientsJSON + `
						}
					}
				}
			}`)).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

	case DriveCreationMethodFromFolder:
		// Use POST /sharings/drives with folder_id
		obj = e.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+appToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
				"data": {
					"type": "` + consts.Sharings + `",
					"attributes": {
						"description": "` + description + `",
						"folder_id": "` + productID + `"
					},
					"relationships": {
						"recipients": {
							"data": ` + rwRecipientsJSON + `
						},
						"read_only_recipients": {
							"data": ` + roRecipientsJSON + `
						}
					}
				}
			}`)).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
	}

	sharingID = obj.Value("data").Object().Value("id").String().NotEmpty().Raw()

	// Extract invitation link
	publicName, _ := inst.SettingsPublicName()
	sentDescription, disco := extractInvitationLink(t, inst, publicName, "")
	assert.Equal(t, sentDescription, description)
	assert.Contains(t, disco, "/discovery?state=")
	discovery = disco

	return
}

// acceptSharedDrive performs the acceptance flow on the recipient side.
// It extracts the discovery link for the specific recipient and completes the acceptance flow.
func acceptSharedDrive(
	t *testing.T,
	ownerInstance *instance.Instance,
	recipientInstance *instance.Instance,
	recipientName string,
	tsAURL string,
	tsRecipientURL string,
	sharingID string,
) {
	t.Helper()
	eA := httpexpect.Default(t, tsAURL)
	eR := httpexpect.Default(t, tsRecipientURL)

	// Extract the discovery link for this specific recipient
	ownerPublicName, _ := ownerInstance.SettingsPublicName()
	_, discoveryLink := extractInvitationLink(t, ownerInstance, ownerPublicName, recipientName)

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

	// Recipient goes to the discovery link on owner host
	u, err := url.Parse(discoveryLink)
	assert.NoError(t, err)
	state := u.Query()["state"][0]

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
	authorizeLink := redirectHeader.NotEmpty().Raw()

	// Ensure the owner instance URL is set for this specific sharing
	FakeOwnerInstanceForSharing(t, recipientInstance, tsAURL, sharingID)

	u, err = url.Parse(authorizeLink)
	assert.NoError(t, err)
	st := u.Query()["state"][0]

	// Perform authorize request (POST) without following redirect to drive subdomain.
	// The CSRF token is the same as the login one.
	eR.POST(u.Path).
		WithFormField("sharing_id", sharingID).
		WithFormField("state", st).
		WithFormField("csrf_token", token).
		WithFormField("synchronize", true).
		WithRedirectPolicy(httpexpect.DontFollowRedirects).
		Expect().Status(303).
		Header("Location").
		Contains("/?sharing=" + sharingID)
}

// acceptSharedDriveForBetty is kept for convenience and delegates to acceptSharedDrive.
func acceptSharedDriveForBetty(
	t *testing.T,
	ownerInstance *instance.Instance,
	bettyInstance *instance.Instance,
	tsAURL string,
	tsBURL string,
	sharingID string,
) {
	acceptSharedDrive(t, ownerInstance, bettyInstance, "Betty", tsAURL, tsBURL, sharingID)
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

// runCoreSharedDrivesTests runs core shared drive tests with a specific creation method.
func runCoreSharedDrivesTests(t *testing.T, method DriveCreationMethod) {
	var sharingID, productID, meetingsID, checklistID string

	config.UseTestFile(t)
	build.BuildMode = build.ModeDev
	cfg := config.GetConfig()
	cfg.Assets = "../../assets"
	cfg.Contexts = map[string]interface{}{
		config.DefaultInstanceContext: map[string]interface{}{
			"sharing": map[string]interface{}{
				"auto_accept_trusted_contacts": true,
				"auto_accept_trusted":          true,
				"trusted_domains":              []interface{}{"cozy.local", "example.com"},
			},
		},
	}
	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb(t)
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	// Prepare the owner instance - replace "/" with "_" for valid domain names
	testName := strings.ReplaceAll(t.Name(), "/", "_")
	ownerSetup := testutils.NewSetup(t, testName+"_owner")
	ownerInstance := ownerSetup.GetTestInstance(&lifecycle.Options{
		Email:      "owner@example.net",
		PublicName: "Owner",
	})
	ownerAppToken := generateAppToken(ownerInstance, "drive", "io.cozy.files")
	tsOwner := ownerSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/files":    files.Routes,
		"/notes":    notes.Routes,
		"/sharings": sharings.Routes,
	})
	tsOwner.Config.Handler.(*echo.Echo).Renderer = render
	tsOwner.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsOwner.Close)

	// Prepare Betty's instance (recipient)
	bettySetup := testutils.NewSetup(t, testName+"_betty")
	bettyInstance := bettySetup.GetTestInstance(&lifecycle.Options{
		Email:         "betty@example.net",
		PublicName:    "Betty",
		Passphrase:    "MyPassphrase",
		KdfIterations: 5000,
		Key:           "xxx",
	})
	bettyAppToken := generateAppToken(bettyInstance, "drive", consts.Files)
	tsBetty := bettySetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth": func(g *echo.Group) {
			g.Use(middlewares.LoadSession)
			auth.Routes(g)
		},
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsBetty.Config.Handler.(*echo.Echo).Renderer = render
	tsBetty.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsBetty.Close)

	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()), "Could not init dynamic FS")

	eOwner := httpexpect.Default(t, tsOwner.URL)
	eBetty := httpexpect.Default(t, tsBetty.URL)

	t.Run("CreateSharedDrive", func(t *testing.T) {
		sid, dirID, _ := createSharedDrive(t, method, ownerInstance, ownerAppToken, tsOwner.URL, "Product team", "Drive for the product team", nil)
		sharingID, productID = sid, dirID

		// Create additional files for subsequent tests
		meetingsID = createDirectory(t, eOwner, productID, "Meetings", ownerAppToken)
		checklistID = createFile(t, eOwner, meetingsID, "Checklist.txt", ownerAppToken)
	})

	t.Run("ListSharedDrives", func(t *testing.T) {
		obj := eOwner.GET("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
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
		owner.Value("public_name").IsEqual("Owner")

		rules := attrs.Value("rules").Array()
		rules.Length().IsEqual(1)
		rule := rules.Value(0).Object()
		rule.Value("title").IsEqual("Product team")
		rule.Value("doctype").IsEqual("io.cozy.files")
		rule.Value("values").Array().Value(0).IsEqual(productID)
	})

	t.Run("AcceptSharedDrive", func(t *testing.T) {
		acceptSharedDrive(t, ownerInstance, bettyInstance, "Betty", tsOwner.URL, tsBetty.URL, sharingID)

		// Verify that owner's contact was created and marked as trusted on Betty's side
		var s sharing.Sharing
		require.NoError(t, couchdb.GetDoc(bettyInstance, consts.Sharings, sharingID, &s))
		require.NotEmpty(t, s.Members[0].Email, "Owner should have an email")

		ownerContact, err := contact.FindByEmail(bettyInstance, s.Members[0].Email)
		require.NoError(t, err, "Owner's contact should exist on Betty's instance after accepting")
		require.True(t, ownerContact.IsTrusted(), "Owner's contact should be marked as trusted on Betty's side after she accepted the sharing")
	})

	t.Run("OwnerSeesRecipientReadyStatus", func(t *testing.T) {
		recipientObj := eBetty.GET("/sharings/"+sharingID).
			WithHeader("Authorization", "Bearer "+bettyAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		recipientAttrs := recipientObj.Value("data").Object().Value("attributes").Object()
		recipientMembers := recipientAttrs.Value("members").Array()
		recipientMembers.Length().IsEqual(3)
		recipientMembers.Value(1).Object().Value("email").IsEqual("betty@example.net")
		recipientMembers.Value(1).Object().Value("status").IsEqual("ready")

		ownerObj := eOwner.GET("/sharings/"+sharingID).
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		ownerAttrs := ownerObj.Value("data").Object().Value("attributes").Object()
		ownerMembers := ownerAttrs.Value("members").Array()
		ownerMembers.Length().IsEqual(3)
		ownerMembers.Value(1).Object().Value("email").IsEqual("betty@example.net")
		ownerMembers.Value(1).Object().Value("status").IsEqual("ready")
	})

	t.Run("AccessDirOrFile", func(t *testing.T) {
		t.Run("HEAD", func(t *testing.T) {
			// HEAD request on non-existing file should return 404
			eBetty.HEAD("/sharings/drives/"+sharingID+"/nonexistent").
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(404)

			// HEAD request on directory should return 200
			eBetty.HEAD("/sharings/drives/"+sharingID+"/"+productID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(200)

			// HEAD request on file should return 200
			eBetty.HEAD("/sharings/drives/"+sharingID+"/"+meetingsID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(200)

			// HEAD request without authentication should fail
			eBetty.HEAD("/sharings/drives/" + sharingID + "/" + checklistID).
				Expect().Status(401)

			// HEAD request with wrong sharing ID should fail
			eBetty.HEAD("/sharings/drives/wrong-id/"+checklistID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(404)
		})

		t.Run("GET", func(t *testing.T) {
			// GET request on non-existing file should return 404
			eBetty.GET("/sharings/drives/"+sharingID+"/nonexistent").
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(404)

			// GET request on directory should return 200 and directory data
			obj := eBetty.GET("/sharings/drives/"+sharingID+"/"+meetingsID).
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

			// GET request on file should return 200 and file data
			obj = eBetty.GET("/sharings/drives/"+sharingID+"/"+checklistID).
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
			eBetty.GET("/sharings/drives/" + sharingID + "/" + meetingsID).
				Expect().Status(401)

			// GET request with wrong sharing ID should fail
			eBetty.GET("/sharings/drives/wrong-id/"+meetingsID).
				WithHeader("Authorization", "Bearer "+bettyAppToken).
				Expect().Status(404)
		})
	})
}

// TestCoreSharedDrivesWithBothMethods runs core shared drive tests with both
// the legacy API (POST /sharings/ with drive:true) and the new API
// (POST /sharings/drives with folder_id).
func TestCoreSharedDrivesWithBothMethods(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	methods := []DriveCreationMethod{
		DriveCreationMethodLegacy,
		DriveCreationMethodFromFolder,
	}

	for _, method := range methods {
		t.Run(string(method), func(t *testing.T) {
			runCoreSharedDrivesTests(t, method)
		})
	}
}

func TestCreateDriveFromFolder(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	build.BuildMode = build.ModeDev
	config.GetConfig().Assets = "../../assets"
	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb(t)
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	// Setup test instance
	setup := testutils.NewSetup(t, t.Name()+"_owner")
	ownerInstance := setup.GetTestInstance(&lifecycle.Options{
		Email:      "owner@example.net",
		PublicName: "Owner",
	})
	ownerAppToken := generateAppToken(ownerInstance, "drive", consts.Files)
	tsOwner := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsOwner.Config.Handler.(*echo.Echo).Renderer = render
	tsOwner.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsOwner.Close)

	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()), "Could not init dynamic FS")

	eOwner := httpexpect.Default(t, tsOwner.URL)

	t.Run("CreateDriveFromUnsharedFolder", func(t *testing.T) {
		// Create a directory
		dirID := createRootDirectory(t, eOwner, "ProjectDocs", ownerAppToken)

		// Create a contact for the recipient
		recipientContact := createContact(t, ownerInstance, "Alice", "alice@example.net")

		// Create shared drive from that folder using the new endpoint
		obj := eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"description": "Project Documents",
						"folder_id": "%s"
					},
					"relationships": {
						"recipients": {
							"data": [{"id": "%s", "type": "%s"}]
						}
					}
				}
			}`, consts.Sharings, dirID, recipientContact.ID(), recipientContact.DocType()))).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		// Verify sharing was created with correct attributes
		data := obj.Value("data").Object()
		data.Value("id").String().NotEmpty()
		attrs := data.Value("attributes").Object()
		attrs.Value("drive").Boolean().IsTrue()
		attrs.Value("description").String().IsEqual("Project Documents")

		// Verify the rule was created correctly
		rules := attrs.Value("rules").Array()
		rules.Length().IsEqual(1)
		rule := rules.Value(0).Object()
		rule.Value("title").String().IsEqual("ProjectDocs")
		rule.Value("doctype").String().IsEqual(consts.Files)
		rule.Value("values").Array().Value(0).String().IsEqual(dirID)
	})

	t.Run("CreateDriveWithoutRecipients", func(t *testing.T) {
		// Create a directory
		dirID := createRootDirectory(t, eOwner, "NoRecipientsFolder", ownerAppToken)

		// Create shared drive without recipients (should succeed)
		obj := eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"description": "Drive without recipients",
						"folder_id": "%s"
					}
				}
			}`, consts.Sharings, dirID))).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		// Verify sharing was created
		data := obj.Value("data").Object()
		data.Value("id").String().NotEmpty()
		attrs := data.Value("attributes").Object()
		attrs.Value("drive").Boolean().IsTrue()
	})

	t.Run("FailOnMissingFolderID", func(t *testing.T) {
		eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"description": "No folder ID"
					}
				}
			}`, consts.Sharings))).
			Expect().Status(422)
	})

	t.Run("FailOnNonexistentFolder", func(t *testing.T) {
		eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "nonexistent-folder-id"
					}
				}
			}`, consts.Sharings))).
			Expect().Status(404)
	})

	t.Run("FailOnFile", func(t *testing.T) {
		// Create a file instead of directory
		fileID := createFile(t, eOwner, "", "test.txt", ownerAppToken)

		eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "%s"
					}
				}
			}`, consts.Sharings, fileID))).
			Expect().Status(422)
	})

	t.Run("FailOnSystemFolder", func(t *testing.T) {
		// Try to share the root directory
		resp := eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "%s"
					}
				}
			}`, consts.Sharings, consts.RootDirID))).
			Expect().Status(422)

		// Verify error message
		resp.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.errors[0].detail").String().
			Contains("Cannot share system folder")

		// Try to share the trash directory
		resp = eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "%s"
					}
				}
			}`, consts.Sharings, consts.TrashDirID))).
			Expect().Status(422)

		// Verify error message
		resp.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.errors[0].detail").String().
			Contains("Cannot share system folder")

		// Try to share the shared-drives directory
		resp = eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "%s"
					}
				}
			}`, consts.Sharings, consts.SharedDrivesDirID))).
			Expect().Status(422)

		// Verify error message
		resp.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.errors[0].detail").String().
			Contains("Cannot share system folder")
	})

	t.Run("FailOnAlreadySharedFolder", func(t *testing.T) {
		// Create a directory and share it first
		dirID := createRootDirectory(t, eOwner, "AlreadyShared", ownerAppToken)

		recipientContact := createContact(t, ownerInstance, "Bob", "bob@example.net")

		// Create first sharing
		eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "%s"
					},
					"relationships": {
						"recipients": {
							"data": [{"id": "%s", "type": "%s"}]
						}
					}
				}
			}`, consts.Sharings, dirID, recipientContact.ID(), recipientContact.DocType()))).
			Expect().Status(201)

		// Try to create another drive from the same folder
		resp := eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "%s"
					}
				}
			}`, consts.Sharings, dirID))).
			Expect().Status(409)

		// Verify error message
		resp.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.errors[0].detail").String().
			Contains("already has an existing sharing")
	})

	t.Run("FailOnFolderInsideSharing", func(t *testing.T) {
		// Create a parent directory and share it
		parentID := createRootDirectory(t, eOwner, "ParentShared", ownerAppToken)

		// Create a subdirectory inside
		childID := createDirectory(t, eOwner, parentID, "ChildFolder", ownerAppToken)

		recipientContact := createContact(t, ownerInstance, "Carol", "carol@example.net")

		// Share the parent
		eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "%s"
					},
					"relationships": {
						"recipients": {
							"data": [{"id": "%s", "type": "%s"}]
						}
					}
				}
			}`, consts.Sharings, parentID, recipientContact.ID(), recipientContact.DocType()))).
			Expect().Status(201)

		// Try to share the child folder (should fail because it's inside a shared folder)
		resp := eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "%s"
					}
				}
			}`, consts.Sharings, childID))).
			Expect().Status(409)

		// Verify error message
		resp.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.errors[0].detail").String().
			Contains("already has an existing sharing")
	})

	t.Run("FailOnFolderContainingSharedChild", func(t *testing.T) {
		// Create a parent directory
		parentID := createRootDirectory(t, eOwner, "ParentWithSharedChild", ownerAppToken)

		// Create a subdirectory inside
		childID := createDirectory(t, eOwner, parentID, "SharedChildFolder", ownerAppToken)

		recipientContact := createContact(t, ownerInstance, "Dan", "dan@example.net")

		// Share the child folder first
		eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "%s"
					},
					"relationships": {
						"recipients": {
							"data": [{"id": "%s", "type": "%s"}]
						}
					}
				}
			}`, consts.Sharings, childID, recipientContact.ID(), recipientContact.DocType()))).
			Expect().Status(201)

		// Try to share the parent folder (should fail because it contains a shared folder)
		resp := eOwner.POST("/sharings/drives").
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
				"data": {
					"type": "%s",
					"attributes": {
						"folder_id": "%s"
					}
				}
			}`, consts.Sharings, parentID))).
			Expect().Status(409)

		// Verify error message
		resp.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.errors[0].detail").String().
			Contains("already has an existing sharing")
	})
}

func TestDriveAutoAcceptTrusted(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	build.BuildMode = build.ModeDev
	config.GetConfig().Assets = "../../assets"
	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb(t)
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	// Owner setup
	ownerSetup := testutils.NewSetup(t, t.Name()+"_owner")
	ownerInstance := ownerSetup.GetTestInstance(&lifecycle.Options{
		Email:      "owner@example.com",
		PublicName: "Owner",
	})
	ownerAppToken := generateAppToken(ownerInstance, "drive", consts.Files)
	tsOwner := ownerSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsOwner.Config.Handler.(*echo.Echo).Renderer = render
	tsOwner.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsOwner.Close)

	// Recipient setup with trusted domain configuration
	recipientSetup := testutils.NewSetup(t, t.Name()+"_recipient")
	recipientInstance := recipientSetup.GetTestInstance(&lifecycle.Options{
		Email:      "recipient@example.com",
		PublicName: "Recipient",
		Passphrase: "MyPassphrase",
	})
	recipientAppToken := generateAppToken(recipientInstance, "drive", consts.Files)
	tsRecipient := recipientSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth": func(g *echo.Group) {
			g.Use(middlewares.LoadSession)
			auth.Routes(g)
		},
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsRecipient.Config.Handler.(*echo.Echo).Renderer = render
	tsRecipient.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsRecipient.Close)

	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()), "Could not init dynamic FS")

	// Configure auto-accept for trusted domains
	cfg := config.GetConfig()
	prevContexts := cfg.Contexts
	cfg.Contexts = map[string]interface{}{
		config.DefaultInstanceContext: map[string]interface{}{
			"sharing": map[string]interface{}{
				"auto_accept_trusted": true,
				"trusted_domains":     []interface{}{"cozy.local", "example.com"},
			},
		},
	}
	t.Cleanup(func() {
		cfg.Contexts = prevContexts
	})

	// Create the Drive sharing using existing helper patterns
	eOwner := httpexpect.Default(t, tsOwner.URL)

	// Create a directory for the Drive sharing
	sharedDirID := createRootDirectory(t, eOwner, "Shared Drive", ownerAppToken)

	// Create a contact for the recipient
	recipientContact := createContact(t, ownerInstance, "Recipient", "recipient@example.com")
	recipientContact.M["cozy"] = []interface{}{
		map[string]interface{}{"url": tsRecipient.URL, "primary": true},
	}
	require.NoError(t, couchdb.UpdateDoc(ownerInstance, recipientContact))

	// Create the Drive sharing
	payload := fmt.Sprintf(`{
		"data": {
			"type": "%s",
			"attributes": {
				"description": "Auto-accept test drive",
				"drive": true,
				"rules": [{
					"title": "Test Drive",
					"doctype": "%s",
					"values": ["%s"]
				}]
			},
			"relationships": {
				"recipients": {
					"data": [{
						"id": "%s",
						"type": "%s"
					}]
				}
			}
		}
	}`, consts.Sharings, consts.Files, sharedDirID, recipientContact.ID(), recipientContact.DocType())

	resp := eOwner.POST("/sharings/").
		WithHeader("Authorization", "Bearer "+ownerAppToken).
		WithHeader("Content-Type", "application/vnd.api+json").
		WithBytes([]byte(payload)).
		Expect().
		Status(http.StatusCreated).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object()

	sharingID := resp.Value("data").Object().Value("id").String().NotEmpty().Raw()

	// Make the recipient aware of the owner's test server URL
	FakeOwnerInstanceForSharing(t, recipientInstance, tsOwner.URL, sharingID)

	// Verify the sharing was auto-accepted on the owner's side
	eRecipient := httpexpect.Default(t, tsRecipient.URL)

	require.Eventually(t, func() bool {
		// Check on owner side
		ownerSharing := eOwner.GET("/sharings/"+sharingID).
			WithHeader("Authorization", "Bearer "+ownerAppToken).
			Expect().
			Status(http.StatusOK).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		attrs := ownerSharing.Value("data").Object().Value("attributes").Object()
		members := attrs.Value("members").Array()

		if members.Length().Raw() < 2 {
			return false
		}

		// Check if recipient (member[1]) is ready
		status := members.Value(1).Object().Value("status").String().Raw()
		active := attrs.Value("active").Boolean().Raw()

		return status == sharing.MemberStatusReady && active
	}, 10*time.Second, 500*time.Millisecond, "Drive sharing was not auto-accepted")

	// Verify the sharing is active on recipient side too
	require.Eventually(t, func() bool {
		recipientSharing := eRecipient.GET("/sharings/"+sharingID).
			WithHeader("Authorization", "Bearer "+recipientAppToken).
			Expect().
			Status(http.StatusOK).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		attrs := recipientSharing.Value("data").Object().Value("attributes").Object()
		return attrs.Value("active").Boolean().Raw()
	}, 10*time.Second, 500*time.Millisecond, "Drive sharing not active on recipient side")
}

func TestSharedDriveAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	t.Run("RecipientSeesReadyStatus", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		obj := eB.GET("/sharings/"+env.firstSharingID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		attrs := obj.Value("data").Object().Value("attributes").Object()
		members := attrs.Value("members").Array()
		members.Length().Ge(2)
		members.Value(1).Object().Value("status").IsEqual("ready")
	})

	t.Run("OwnerSeesRecipientReadyStatus", func(t *testing.T) {
		eA, _, _ := env.createClients(t)

		obj := eA.GET("/sharings/"+env.firstSharingID).
			WithHeader("Authorization", "Bearer "+env.acmeToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		attrs := obj.Value("data").Object().Value("attributes").Object()
		members := attrs.Value("members").Array()
		members.Length().Ge(2)
		members.Value(1).Object().Value("status").IsEqual("ready")
	})

	t.Run("AcceptNewDriveViaPOST", func(t *testing.T) {
		// Create a new drive that Dave hasn't accepted yet
		newSharingID, _, _ := createSharedDrive(
			t,
			DriveCreationMethodLegacy,
			env.acme,
			env.acmeToken,
			env.tsA.URL,
			"Drive for Dave POST acceptance",
			"Testing POST acceptance flow",
			nil,
		)

		// Dave accepts the sharing
		acceptSharedDrive(t, env.acme, env.dave, "Dave", env.tsA.URL, env.tsD.URL, newSharingID)

		// Verify Dave can now access the sharing
		_, _, eD := env.createClients(t)
		eD.GET("/sharings/"+newSharingID).
			WithHeader("Authorization", "Bearer "+env.daveToken).
			Expect().Status(200)
	})
}

func TestSharedDriveAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	t.Run("HEAD", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		t.Run("NonExistentFile", func(t *testing.T) {
			eB.HEAD("/sharings/drives/"+env.firstSharingID+"/nonexistent").
				WithHeader("Authorization", "Bearer "+env.bettyToken).
				Expect().Status(404)
		})

		t.Run("Directory", func(t *testing.T) {
			eB.HEAD("/sharings/drives/"+env.firstSharingID+"/"+env.firstRootDirID).
				WithHeader("Authorization", "Bearer "+env.bettyToken).
				Expect().Status(200)
		})

		t.Run("Subdirectory", func(t *testing.T) {
			eB.HEAD("/sharings/drives/"+env.firstSharingID+"/"+env.meetingsDirID).
				WithHeader("Authorization", "Bearer "+env.bettyToken).
				Expect().Status(200)
		})

		t.Run("File", func(t *testing.T) {
			eB.HEAD("/sharings/drives/"+env.firstSharingID+"/"+env.checklistID).
				WithHeader("Authorization", "Bearer "+env.bettyToken).
				Expect().Status(200)
		})

		t.Run("WithoutAuth", func(t *testing.T) {
			eB.HEAD("/sharings/drives/" + env.firstSharingID + "/" + env.checklistID).
				Expect().Status(401)
		})

		t.Run("WrongSharingID", func(t *testing.T) {
			eB.HEAD("/sharings/drives/wrong-id/"+env.checklistID).
				WithHeader("Authorization", "Bearer "+env.bettyToken).
				Expect().Status(404)
		})
	})

	t.Run("GET", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		t.Run("NonExistentFile", func(t *testing.T) {
			eB.GET("/sharings/drives/"+env.firstSharingID+"/nonexistent").
				WithHeader("Authorization", "Bearer "+env.bettyToken).
				Expect().Status(404)
		})

		t.Run("Directory", func(t *testing.T) {
			obj := eB.GET("/sharings/drives/"+env.firstSharingID+"/"+env.meetingsDirID).
				WithHeader("Authorization", "Bearer "+env.bettyToken).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()

			data := obj.Value("data").Object()
			data.Value("type").String().IsEqual("io.cozy.files")
			data.Value("id").String().IsEqual(env.meetingsDirID)
			attrs := data.Value("attributes").Object()
			attrs.Value("type").String().IsEqual("directory")
			attrs.Value("name").String().IsEqual("Meetings")
			attrs.Value("driveId").String().IsEqual(env.firstSharingID)
		})

		t.Run("File", func(t *testing.T) {
			obj := eB.GET("/sharings/drives/"+env.firstSharingID+"/"+env.checklistID).
				WithHeader("Authorization", "Bearer "+env.bettyToken).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()

			data := obj.Value("data").Object()
			data.Value("type").String().IsEqual("io.cozy.files")
			data.Value("id").String().IsEqual(env.checklistID)
			attrs := data.Value("attributes").Object()
			attrs.Value("type").String().IsEqual("file")
			attrs.Value("name").String().IsEqual("Checklist.txt")
			attrs.Value("mime").String().IsEqual("text/plain")
			attrs.Value("size").String().IsEqual("3")
		})

		t.Run("WithoutAuth", func(t *testing.T) {
			eB.GET("/sharings/drives/" + env.firstSharingID + "/" + env.meetingsDirID).
				Expect().Status(401)
		})

		t.Run("WrongSharingID", func(t *testing.T) {
			eB.GET("/sharings/drives/wrong-id/"+env.meetingsDirID).
				WithHeader("Authorization", "Bearer "+env.bettyToken).
				Expect().Status(404)
		})
	})
}

func TestSharedDriveChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	t.Run("GetChangesFeed", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		obj := eB.GET("/sharings/drives/"+env.firstSharingID+"/_changes").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/json"}).
			Object()

		// Should have results array
		results := obj.Value("results").Array()
		results.NotEmpty()

		// Should have last_seq
		obj.Value("last_seq").String().NotEmpty()
	})

	t.Run("ChangesIncludeSharedFiles", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		obj := eB.GET("/sharings/drives/"+env.firstSharingID+"/_changes").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/json"}).
			Object()

		results := obj.Value("results").Array()

		// Find the checklist file in the results
		found := false
		for i := 0; i < int(results.Length().Raw()); i++ {
			change := results.Value(i).Object()
			if change.Value("id").String().Raw() == env.checklistID {
				found = true
				// Verify driveId is set
				change.Path("$.doc.driveId").String().IsEqual(env.firstSharingID)
				break
			}
		}
		require.True(t, found, "Checklist file should be in changes feed")
	})

	t.Run("ChangesExcludeUnsharedFiles", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		obj := eB.GET("/sharings/drives/"+env.firstSharingID+"/_changes").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/json"}).
			Object()

		results := obj.Value("results").Array()

		// The outsideOfShareID should either not be present or be marked as deleted
		for i := 0; i < int(results.Length().Raw()); i++ {
			change := results.Value(i).Object()
			if change.Value("id").String().Raw() == env.outsideOfShareID {
				// If present, should be marked as deleted
				change.Value("deleted").Boolean().IsTrue()
			}
		}
	})

	t.Run("ChangesWithoutAuth", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		eB.GET("/sharings/drives/" + env.firstSharingID + "/_changes").
			Expect().Status(401)
	})

	t.Run("ChangesWrongSharingID", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		eB.GET("/sharings/drives/wrong-id/_changes").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(404)
	})
}

func TestSharedDriveCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	t.Run("ListSharedDrives", func(t *testing.T) {
		eA, _, _ := env.createClients(t)

		obj := eA.GET("/sharings/drives").
			WithHeader("Authorization", "Bearer "+env.acmeToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Array()
		data.Length().Ge(1) // At least one drive from setup

		// Verify first drive has expected structure
		sharingObj := data.Value(0).Object()
		sharingObj.Value("type").IsEqual("io.cozy.sharings")
		sharingObj.Value("id").String().NotEmpty()

		attrs := sharingObj.Value("attributes").Object()
		attrs.Value("app_slug").IsEqual("drive")
		attrs.Value("owner").IsEqual(true)
		attrs.Value("drive").IsEqual(true)

		members := attrs.Value("members").Array()
		members.Length().Ge(2) // At least owner + Betty

		owner := members.Value(0).Object()
		owner.Value("status").IsEqual("owner")
	})

	t.Run("CreateAdditionalDrive", func(t *testing.T) {
		// Create a new shared drive
		sharingID, dirID, _ := createSharedDrive(
			t,
			DriveCreationMethodLegacy,
			env.acme,
			env.acmeToken,
			env.tsA.URL,
			"Additional Test Drive",
			"Drive created in TestSharedDriveCreation",
			nil,
		)

		require.NotEmpty(t, sharingID, "Sharing ID should not be empty")
		require.NotEmpty(t, dirID, "Directory ID should not be empty")

		// Verify the drive appears in the list
		eA, _, _ := env.createClients(t)
		obj := eA.GET("/sharings/drives").
			WithHeader("Authorization", "Bearer "+env.acmeToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Array()
		data.Length().Ge(2) // Original + new drive
	})

	t.Run("CreateDriveFromFolder", func(t *testing.T) {
		sharingID, dirID, _ := createSharedDrive(
			t,
			DriveCreationMethodFromFolder,
			env.acme,
			env.acmeToken,
			env.tsA.URL,
			"Drive From Folder Test",
			"Drive created from existing folder",
			nil,
		)

		require.NotEmpty(t, sharingID, "Sharing ID should not be empty")
		require.NotEmpty(t, dirID, "Directory ID should not be empty")
	})
}

func TestSharedDriveDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	t.Run("DirectDownload", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		res := eB.GET("/sharings/drives/"+env.firstSharingID+"/download/"+env.checklistID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200)

		res.Header("Content-Disposition").HasPrefix("inline")
		res.Header("Content-Disposition").Contains("Checklist.txt")
		res.Header("Content-Type").IsEqual("text/plain")
		res.Header("Etag").NotEmpty()
		res.Header("Content-Length").IsEqual("3")
		res.Body().IsEqual("foo")
	})

	t.Run("TwoStepDownload", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		// Create download link
		related := eB.POST("/sharings/drives/"+env.firstSharingID+"/downloads").
			WithQuery("Id", env.checklistID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.links.related").String().NotEmpty().Raw()

		// Download inline
		res := eB.GET(related).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200)
		res.Header("Content-Disposition").IsEqual(`inline; filename="Checklist.txt"`)

		// Download as attachment
		res = eB.GET(related).
			WithQuery("Dl", "1").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200)
		res.Header("Content-Disposition").IsEqual(`attachment; filename="Checklist.txt"`)
	})

	t.Run("DownloadNonExistent", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		eB.GET("/sharings/drives/"+env.firstSharingID+"/download/nonexistent").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(404)
	})

	t.Run("DownloadWithoutAuth", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		eB.GET("/sharings/drives/" + env.firstSharingID + "/download/" + env.checklistID).
			Expect().Status(401)
	})
}

func TestSharedDriveFileCopy(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	t.Run("CanCopySharedFile", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		// Copy without name
		attrs := eB.POST("/sharings/drives/"+env.firstSharingID+"/"+env.checklistID+"/copy").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			WithBytes([]byte("")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().
			Path("$.data.attributes").Object()

		attrs.Value("name").String().IsEqual("Checklist (copy).txt")
		attrs.Value("driveId").String().IsEqual(env.firstSharingID)
	})

	t.Run("CopyWithSameName", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		eB.POST("/sharings/drives/"+env.firstSharingID+"/"+env.checklistID+"/copy").
			WithQuery("Name", "Checklist.txt").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			WithBytes([]byte("")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().
			Path("$.data.attributes.name").String().Contains("(")
	})

	t.Run("CopyWithCustomName", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		eB.POST("/sharings/drives/"+env.firstSharingID+"/"+env.checklistID+"/copy").
			WithQuery("Name", "MyCustomCopy.txt").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			WithBytes([]byte("")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().
			Path("$.data.attributes.name").String().IsEqual("MyCustomCopy.txt")
	})

	t.Run("CannotCopyUnsharedFile", func(t *testing.T) {
		eA, eB, _ := env.createClients(t)

		// Create a file outside the shared drive
		nonSharedID := createFile(t, eA, env.outsideOfShareID, "NotShared.txt", env.acmeToken)

		eB.POST("/sharings/drives/"+env.firstSharingID+"/"+nonSharedID+"/copy").
			WithHeader("Content-Type", "text/plain").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			WithBytes([]byte("")).
			Expect().Status(403)
	})
}

func TestSharedDriveMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	t.Run("GetMetadataByPath", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		// GET metadata request with file path (path includes root dir name)
		filePath := "/" + env.rootDirName + "/Meetings/Checklist.txt"
		obj := eB.GET("/sharings/drives/"+env.firstSharingID+"/metadata").
			WithQuery("Path", filePath).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.Value("type").String().IsEqual("io.cozy.files")
		data.Value("id").String().IsEqual(env.checklistID)
		attrs := data.Value("attributes").Object()
		attrs.Value("type").String().IsEqual("file")
		attrs.Value("name").String().IsEqual("Checklist.txt")
	})

	t.Run("GetMetadataByDirectoryPath", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		// Path includes root dir name
		dirPath := "/" + env.rootDirName + "/Meetings"
		obj := eB.GET("/sharings/drives/"+env.firstSharingID+"/metadata").
			WithQuery("Path", dirPath).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.Value("type").String().IsEqual("io.cozy.files")
		data.Value("id").String().IsEqual(env.meetingsDirID)
		attrs := data.Value("attributes").Object()
		attrs.Value("type").String().IsEqual("directory")
		attrs.Value("name").String().IsEqual("Meetings")
	})

	t.Run("GetMetadataNonExistent", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		nonExistentPath := "/" + env.rootDirName + "/NonExistent"
		eB.GET("/sharings/drives/"+env.firstSharingID+"/metadata").
			WithQuery("Path", nonExistentPath).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(404)
	})

	t.Run("GetMetadataWithoutPath", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		eB.GET("/sharings/drives/"+env.firstSharingID+"/metadata").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(400)
	})

	t.Run("GetMetadataWithoutAuth", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		filePath := "/" + env.rootDirName + "/Meetings/Checklist.txt"
		eB.GET("/sharings/drives/"+env.firstSharingID+"/metadata").
			WithQuery("Path", filePath).
			Expect().Status(401)
	})

	t.Run("GetDirSize", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		obj := eB.GET("/sharings/drives/"+env.firstSharingID+"/"+env.firstRootDirID+"/size").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.Value("id").IsEqual(env.firstRootDirID)
		data.Value("type").IsEqual("io.cozy.files.sizes")
		// Size should be at least 3 bytes (from Checklist.txt)
		data.Value("attributes").Object().Value("size").NotNull()
	})

	t.Run("MoveFileWithinDrive", func(t *testing.T) {
		eA, eB, _ := env.createClients(t)

		// Create a file to move
		fileID := createFile(t, eA, env.meetingsDirID, "FileToMove.txt", env.acmeToken)

		// Move the file to product directory
		movedFile := eB.PATCH("/sharings/drives/"+env.firstSharingID+"/"+fileID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
				"data": {
					"type": "io.cozy.files",
					"id": "` + fileID + `",
					"attributes": {
						"dir_id": "` + env.productDirID + `"
					}
				}
			}`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		movedFile.Path("$.data.attributes.dir_id").String().IsEqual(env.productDirID)
	})

	t.Run("CannotMoveFileOutsideDrive", func(t *testing.T) {
		eA, eB, _ := env.createClients(t)

		// Create a file to attempt moving outside
		fileID := createFile(t, eA, env.meetingsDirID, "CannotMoveOut.txt", env.acmeToken)

		// Attempt to move outside the shared drive - should fail
		eB.PATCH("/sharings/drives/"+env.firstSharingID+"/"+fileID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
				"data": {
					"type": "io.cozy.files",
					"id": "` + fileID + `",
					"attributes": {
						"dir_id": "` + env.outsideOfShareID + `"
					}
				}
			}`)).
			Expect().Status(403)
	})

	t.Run("RenameFile", func(t *testing.T) {
		eA, eB, _ := env.createClients(t)

		// Create a file to rename
		fileID := createFile(t, eA, env.meetingsDirID, "OriginalName.txt", env.acmeToken)

		// Rename the file
		renamed := eB.PATCH("/sharings/drives/"+env.firstSharingID+"/"+fileID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
				"data": {
					"type": "io.cozy.files",
					"id": "` + fileID + `",
					"attributes": {
						"name": "RenamedFile.txt"
					}
				}
			}`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		renamed.Path("$.data.attributes.name").String().IsEqual("RenamedFile.txt")
	})
}

func TestSharedDriveMultipleAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	// Create a second shared drive
	secondSharingID, secondRootDirID, _ := createSharedDrive(
		t,
		DriveCreationMethodLegacy,
		env.acme,
		env.acmeToken,
		env.tsA.URL,
		"Second Shared Drive",
		"Another drive for testing",
		nil,
	)
	acceptSharedDriveForBetty(t, env.acme, env.betty, env.tsA.URL, env.tsB.URL, secondSharingID)

	// Create a file in the second drive
	eA, _, _ := env.createClients(t)
	fileID := createFile(t, eA, secondRootDirID, "SecondDriveFile.txt", env.acmeToken)

	t.Run("BettyCanAccessBothDrives", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		// Access first drive
		eB.GET("/sharings/drives/"+env.firstSharingID+"/"+env.checklistID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200)

		// Access second drive
		eB.GET("/sharings/drives/"+secondSharingID+"/"+fileID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200)
	})

	t.Run("ListShowsBothDrives", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		obj := eB.GET("/sharings/drives").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Array()
		data.Length().Ge(2) // At least two drives
	})

	t.Run("CannotAccessWrongDrive", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		// Try to access file from first drive using second drive's ID
		eB.GET("/sharings/drives/"+secondSharingID+"/"+env.checklistID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(403)
	})
}

func TestSharedDriveNotes(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	t.Run("CreateNoteInSharedDrive", func(t *testing.T) {
		eA, _, _ := env.createClients(t)

		obj := eA.POST("/sharings/drives/"+env.firstSharingID+"/notes").
			WithHeader("Authorization", "Bearer "+env.acmeToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
				"data": {
					"type": "io.cozy.notes.documents",
					"attributes": {
						"title": "Test Note",
						"dir_id": "` + env.meetingsDirID + `",
						"schema": {
							"nodes": [
								["doc", { "content": "block+" }],
								["paragraph", { "content": "inline*", "group": "block" }],
								["text", { "group": "inline" }]
							],
							"marks": [["em", {}], ["strong", {}]],
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
		data.Value("id").String().NotEmpty()

		attrs := data.Value("attributes").Object()
		attrs.HasValue("type", "file")
		attrs.Value("name").String().Contains(".cozy-note")
		attrs.HasValue("mime", "text/vnd.cozy.note+markdown")
	})

	t.Run("OpenNoteFromSharedDrive", func(t *testing.T) {
		eA, eB, _ := env.createClients(t)

		// First create a note
		noteObj := eA.POST("/sharings/drives/"+env.firstSharingID+"/notes").
			WithHeader("Authorization", "Bearer "+env.acmeToken).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
				"data": {
					"type": "io.cozy.notes.documents",
					"attributes": {
						"title": "Note to Open",
						"dir_id": "` + env.meetingsDirID + `",
						"schema": {
							"nodes": [
								["doc", { "content": "block+" }],
								["paragraph", { "content": "inline*", "group": "block" }],
								["text", { "group": "inline" }]
							],
							"marks": [],
							"topNode": "doc"
						}
					}
				}
			}`)).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		noteID := noteObj.Value("data").Object().Value("id").String().Raw()

		// Open the note as Betty (who has write access, ReadOnly: false)
		obj := eB.GET("/sharings/drives/"+env.firstSharingID+"/notes/"+noteID+"/open").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.HasValue("type", consts.NotesURL)
		data.HasValue("id", noteID)

		attrs := data.Value("attributes").Object()
		attrs.HasValue("note_id", noteID)
		sharecode := attrs.Value("sharecode").String().NotEmpty().Raw()

		permObj := eA.GET("/permissions/self").
			WithHeader("Authorization", "Bearer "+sharecode).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		// The permissions should include write verbs (POST, PUT, PATCH, DELETE), not just GET
		permAttrs := permObj.Value("data").Object().Value("attributes").Object()
		perms := permAttrs.Value("permissions").Object()
		for _, rule := range perms.Iter() {
			verbs := rule.Object().Value("verbs").Array()
			verbs.Length().Gt(1)
			break
		}
	})

	t.Run("CreateNoteWithoutAuth", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		eB.POST("/sharings/drives/"+env.firstSharingID+"/notes").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
				"data": {
					"type": "io.cozy.notes.documents",
					"attributes": {
						"title": "Unauthorized",
						"dir_id": "` + env.meetingsDirID + `"
					}
				}
			}`)).
			Expect().Status(401)
	})
}

func TestSharedDriveRealtime(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	t.Run("ReceiveCreatedEvent", func(t *testing.T) {
		eA, eB, _ := env.createClients(t)

		ws := eB.GET("/sharings/drives/" + env.firstSharingID + "/realtime").
			WithWebsocketUpgrade().
			Expect().Status(http.StatusSwitchingProtocols).
			Websocket()
		defer ws.Disconnect()

		ws.WriteText(fmt.Sprintf(`{"method": "AUTH", "payload": "%s"}`, env.bettyToken))
		time.Sleep(50 * time.Millisecond)

		// Create a file as owner
		newFileID := createFile(t, eA, env.meetingsDirID, "RealtimeTest.txt", env.acmeToken)

		// Verify Betty receives the CREATED event
		obj := ws.Expect().TextMessage().JSON().Object()
		obj.HasValue("event", "CREATED")
		payload := obj.Value("payload").Object()
		payload.HasValue("type", consts.Files)
		payload.HasValue("id", newFileID)
	})
}

func TestSharedDriveRevocation(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)

	// Create a new shared drive specifically for revocation testing
	// so we don't affect other tests
	sharingID, rootDirID, _ := createSharedDrive(
		t,
		DriveCreationMethodLegacy,
		env.acme,
		env.acmeToken,
		env.tsA.URL,
		"Revocation Test Drive",
		"Drive for testing revocation",
		nil,
	)
	acceptSharedDriveForBetty(t, env.acme, env.betty, env.tsA.URL, env.tsB.URL, sharingID)

	// Create a test file in the drive
	eA, _, _ := env.createClients(t)
	fileID := createFile(t, eA, rootDirID, "RevocationTestFile.txt", env.acmeToken)

	t.Run("RecipientCanInitiallyAccess", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		eB.GET("/sharings/drives/"+sharingID+"/"+fileID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(200)
	})

	t.Run("RemoveRecipient", func(t *testing.T) {
		eA, _, _ := env.createClients(t)

		eA.DELETE("/sharings/"+sharingID+"/recipients/1").
			WithHeader("Authorization", "Bearer "+env.acmeToken).
			Expect().Status(204)
	})

	t.Run("RevokedRecipientCannotAccess", func(t *testing.T) {
		_, eB, _ := env.createClients(t)

		eB.GET("/sharings/drives/"+sharingID+"/"+fileID).
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(403)

		eB.GET("/sharings/drives/"+sharingID+"/metadata").
			WithQuery("Path", "/RevocationTestFile.txt").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(403)

		eB.GET("/sharings/drives/"+sharingID+"/_changes").
			WithHeader("Authorization", "Bearer "+env.bettyToken).
			Expect().Status(403)
	})
}

// TestSharedDriveGroupDynamicMembership tests that when a contact is added to a group
// that is part of a shared drive, the new member automatically gets access to the drive
// without needing to accept a specific invitation.
func TestSharedDriveGroupDynamicMembership(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	build.BuildMode = build.ModeDev
	cfg := config.GetConfig()
	cfg.Assets = "../../assets"
	cfg.Contexts = map[string]interface{}{
		config.DefaultInstanceContext: map[string]interface{}{
			"sharing": map[string]interface{}{
				"auto_accept_trusted_contacts": true,
				"auto_accept_trusted":          true,
				"trusted_domains":              []interface{}{"cozy.local", "example.com"},
			},
		},
	}
	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb(t)
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()
	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()))

	// Create owner instance (ACME)
	ownerSetup := testutils.NewSetup(t, t.Name()+"_owner")
	ownerInstance := ownerSetup.GetTestInstance(&lifecycle.Options{
		Email:      "owner@example.com",
		PublicName: "Owner",
	})
	ownerAppToken := generateAppToken(ownerInstance, "drive", consts.Files)
	tsOwner := ownerSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsOwner.Config.Handler.(*echo.Echo).Renderer = render
	tsOwner.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsOwner.Close)

	// Create first group member instance (Alice - already in the group when drive is created)
	aliceSetup := testutils.NewSetup(t, t.Name()+"_alice")
	aliceInstance := aliceSetup.GetTestInstance(&lifecycle.Options{
		Email:         "alice@example.com",
		PublicName:    "Alice",
		Passphrase:    "MyPassphrase",
		KdfIterations: 5000,
		Key:           "xxx",
	})
	aliceAppToken := generateAppToken(aliceInstance, "drive", consts.Files)
	tsAlice := aliceSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth": func(g *echo.Group) {
			g.Use(middlewares.LoadSession)
			auth.Routes(g)
		},
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsAlice.Config.Handler.(*echo.Echo).Renderer = render
	tsAlice.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsAlice.Close)

	// Create new member instance (Bob - will be added to the group later)
	bobSetup := testutils.NewSetup(t, t.Name()+"_bob")
	bobInstance := bobSetup.GetTestInstance(&lifecycle.Options{
		Email:         "bob@example.com",
		PublicName:    "Bob",
		Passphrase:    "MyPassphrase",
		KdfIterations: 5000,
		Key:           "xxx",
	})
	bobAppToken := generateAppToken(bobInstance, "drive", consts.Files)
	tsBob := bobSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth": func(g *echo.Group) {
			g.Use(middlewares.LoadSession)
			auth.Routes(g)
		},
		"/files":    files.Routes,
		"/sharings": sharings.Routes,
	})
	tsBob.Config.Handler.(*echo.Echo).Renderer = render
	tsBob.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsBob.Close)

	eOwner := httpexpect.Default(t, tsOwner.URL)

	// Step 1: Create a group on owner's instance
	teamGroup := createGroupOnInstance(t, ownerInstance, "Team")

	// Step 2: Create Alice as a contact in that group on owner's instance (with Cozy URL for auto-accept)
	aliceContact := createContactInGroupWithCozy(t, ownerInstance, teamGroup, "Alice", "alice@example.com", tsAlice.URL)
	require.NotNil(t, aliceContact)

	// Step 3: Create a directory for the shared drive
	sharedDirID := createRootDirectory(t, eOwner, "Team Drive", ownerAppToken)

	// Step 4: Create a file in the directory before sharing
	fileID := createFile(t, eOwner, sharedDirID, "TeamDocument.txt", ownerAppToken)

	// Step 5: Create a shared drive with the group as recipient
	obj := eOwner.POST("/sharings/drives").
		WithHeader("Authorization", "Bearer "+ownerAppToken).
		WithHeader("Content-Type", "application/vnd.api+json").
		WithBytes([]byte(`{
			"data": {
				"type": "` + consts.Sharings + `",
				"attributes": {
					"description": "Team Drive for dynamic membership test",
					"folder_id": "` + sharedDirID + `"
				},
				"relationships": {
					"recipients": {
						"data": [{"id": "` + teamGroup.ID() + `", "type": "` + consts.Groups + `"}]
					}
				}
			}
		}`)).
		Expect().Status(201).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object()

	sharingID := obj.Value("data").Object().Value("id").String().NotEmpty().Raw()

	// Verify group and members are set correctly
	attrs := obj.Value("data").Object().Value("attributes").Object()
	groups := attrs.Value("groups").Array()
	groups.Length().IsEqual(1)
	groups.Value(0).Object().Value("id").IsEqual(teamGroup.ID())
	groups.Value(0).Object().Value("name").IsEqual("Team")

	members := attrs.Value("members").Array()
	members.Length().IsEqual(2) // Owner + Alice
	members.Value(1).Object().Value("name").IsEqual("Alice")
	members.Value(1).Object().Value("only_in_groups").IsEqual(true)
	members.Value(1).Object().Value("groups").Array().Value(0).IsEqual(0)

	// Step 6: Set up Alice's instance to know about the owner's URL for the sharing (for auto-accept)
	FakeOwnerInstanceForSharing(t, aliceInstance, tsOwner.URL, sharingID)

	// Step 7: Wait for Alice's sharing to be auto-accepted
	eAlice := httpexpect.Default(t, tsAlice.URL)
	require.Eventually(t, func() bool {
		resp := eAlice.GET("/sharings/"+sharingID).
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			Expect()
		if resp.Raw().StatusCode != 200 {
			return false
		}
		obj := resp.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).Object()
		active := obj.Value("data").Object().Value("attributes").Object().Value("active").Boolean().Raw()
		return active
	}, 10*time.Second, 500*time.Millisecond, "Alice's sharing should be auto-accepted and active")

	// Step 8: Verify Alice can access the file
	eAlice.GET("/sharings/drives/"+sharingID+"/"+fileID).
		WithHeader("Authorization", "Bearer "+aliceAppToken).
		Expect().Status(200)

	// Step 9: Create Bob as a contact WITHOUT the group initially (with Cozy URL for auto-accept)
	bobContact := createContactWithCozy(t, ownerInstance, "Bob", "bob@example.com", tsBob.URL)

	// Step 10: Add Bob to the group by updating the contact
	addContactToExistingGroup(t, ownerInstance, bobContact, teamGroup)

	// Step 11: Call UpdateGroups to simulate the trigger that fires when contact group membership changes
	msg := job.ShareGroupMessage{
		ContactID:   bobContact.ID(),
		GroupsAdded: []string{teamGroup.ID()},
	}
	require.NoError(t, sharing.UpdateGroups(ownerInstance, msg))

	// Step 12: Reload the sharing to verify Bob was added
	var s sharing.Sharing
	require.NoError(t, couchdb.GetDoc(ownerInstance, consts.Sharings, sharingID, &s))
	require.Len(t, s.Members, 3, "Should now have 3 members: Owner, Alice, Bob")
	require.Equal(t, "Bob", s.Members[2].Name)
	require.True(t, s.Members[2].OnlyInGroups)
	require.Equal(t, []int{0}, s.Members[2].Groups)

	// Step 13: Set up Bob's instance to know about the owner's URL for the sharing
	FakeOwnerInstanceForSharing(t, bobInstance, tsOwner.URL, sharingID)

	// Step 14: Verify Bob can access the shared drive after auto-accept
	// Wait for the sharing to be auto-accepted on Bob's side
	eBob := httpexpect.Default(t, tsBob.URL)
	require.Eventually(t, func() bool {
		// Check if Bob can get the sharing
		resp := eBob.GET("/sharings/"+sharingID).
			WithHeader("Authorization", "Bearer "+bobAppToken).
			Expect()
		if resp.Raw().StatusCode != 200 {
			return false
		}
		obj := resp.JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).Object()
		active := obj.Value("data").Object().Value("attributes").Object().Value("active").Boolean().Raw()
		return active
	}, 10*time.Second, 500*time.Millisecond, "Bob's sharing should be auto-accepted and active")

	// Step 15: Verify Bob can access the file in the shared drive
	eBob.GET("/sharings/drives/"+sharingID+"/"+fileID).
		WithHeader("Authorization", "Bearer "+bobAppToken).
		Expect().Status(200)

	// Step 16: Verify Bob can see the file content via download
	res := eBob.GET("/sharings/drives/"+sharingID+"/download/"+fileID).
		WithHeader("Authorization", "Bearer "+bobAppToken).
		Expect().Status(200)
	res.Body().IsEqual("foo")
}

// createGroupOnInstance creates a contact group on the specified instance.
func createGroupOnInstance(t *testing.T, inst *instance.Instance, name string) *contact.Group {
	t.Helper()
	g := contact.NewGroup()
	g.M["name"] = name
	require.NoError(t, couchdb.CreateDoc(inst, g))
	return g
}

// createContactInGroupWithCozy creates a contact in a group with a Cozy URL.
func createContactInGroupWithCozy(t *testing.T, inst *instance.Instance, g *contact.Group, name, email, cozyURL string) *contact.Contact {
	t.Helper()
	mail := map[string]interface{}{"address": email}
	c := contact.New()
	c.M["fullname"] = name
	c.M["email"] = []interface{}{mail}
	c.M["cozy"] = []interface{}{
		map[string]interface{}{"url": cozyURL, "primary": true},
	}
	c.M["relationships"] = map[string]interface{}{
		"groups": map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{
					"_id":   g.ID(),
					"_type": consts.Groups,
				},
			},
		},
	}
	require.NoError(t, couchdb.CreateDoc(inst, c))
	return c
}

// createContactWithCozy creates a contact with a Cozy URL but NOT in any group.
func createContactWithCozy(t *testing.T, inst *instance.Instance, name, email, cozyURL string) *contact.Contact {
	t.Helper()
	mail := map[string]interface{}{"address": email}
	c := contact.New()
	c.M["fullname"] = name
	c.M["email"] = []interface{}{mail}
	c.M["cozy"] = []interface{}{
		map[string]interface{}{"url": cozyURL, "primary": true},
	}
	require.NoError(t, couchdb.CreateDoc(inst, c))
	return c
}

// addContactToExistingGroup adds an existing contact to a group by updating the contact.
func addContactToExistingGroup(t *testing.T, inst *instance.Instance, c *contact.Contact, g *contact.Group) {
	t.Helper()

	// Get existing groups or create empty slice
	var existingGroups []interface{}
	if rels, ok := c.M["relationships"].(map[string]interface{}); ok {
		if groups, ok := rels["groups"].(map[string]interface{}); ok {
			if data, ok := groups["data"].([]interface{}); ok {
				existingGroups = data
			}
		}
	}

	// Add the new group
	existingGroups = append(existingGroups, map[string]interface{}{
		"_id":   g.ID(),
		"_type": consts.Groups,
	})

	// Update the contact
	c.M["relationships"] = map[string]interface{}{
		"groups": map[string]interface{}{
			"data": existingGroups,
		},
	}
	require.NoError(t, couchdb.UpdateDoc(inst, c))
}
