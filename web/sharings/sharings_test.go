package sharings_test

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/cozy-stack/web/sharings"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const iocozytests = "io.cozy.tests"
const iocozytestswildcard = "io.cozy.tests.*"

// Things that live on Alice's Cozy
var charlieContact, daveContact, edwardContact *contact.Contact
var sharingID, state, aliceAccessToken string

// Bob's browser
var discoveryLink, authorizeLink string

func TestSharings(t *testing.T) {
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

	// Prepare Alice's instance
	setup := testutils.NewSetup(t, t.Name()+"_alice")
	aliceInstance := setup.GetTestInstance(&lifecycle.Options{
		Email:      "alice@example.net",
		PublicName: "Alice",
	})
	aliceAppToken := generateAppToken(aliceInstance, "testapp", iocozytests)
	aliceAppTokenWildcard := generateAppToken(aliceInstance, "testapp2", iocozytestswildcard)
	charlieContact = createContact(t, aliceInstance, "Charlie", "charlie@example.net")
	daveContact = createContact(t, aliceInstance, "Dave", "dave@example.net")
	tsA := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/sharings":    sharings.Routes,
		"/permissions": permissions.Routes,
	})
	tsA.Config.Handler.(*echo.Echo).Renderer = render
	tsA.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsA.Close)

	// Prepare Bob's instance
	bobSetup := testutils.NewSetup(t, t.Name()+"_bob")
	bobInstance := bobSetup.GetTestInstance(&lifecycle.Options{
		Email:         "bob@example.net",
		PublicName:    "Bob",
		Passphrase:    "MyPassphrase",
		KdfIterations: 5000,
		Key:           "xxx",
	})
	bobAppToken := generateAppToken(bobInstance, "testapp", iocozytests)
	tsB := bobSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
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

	t.Run("CreateSharingSuccess", func(t *testing.T) {
		eA := httpexpect.Default(t, tsA.URL)

		bobContact := createBobContact(t, aliceInstance)
		assert.NotEmpty(t, aliceAppToken)
		assert.NotNil(t, bobContact)

		obj := eA.POST("/sharings/").
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "` + consts.Sharings + `",
          "attributes": {
            "description":  "this is a test",
            "open_sharing": true,
            "rules": [{
                "title": "test one",
                "doctype": "` + iocozytests + `",
                "values": ["000000"],
                "add": "sync"
              }]
          },
          "relationships": {
            "recipients": {
              "data": [{"id": "` + bobContact.ID() + `", "type": "` + bobContact.DocType() + `"}]
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

		assertSharingIsCorrectOnSharer(t, obj, sharingID, aliceInstance)
		description := assertInvitationMailWasSent(t, aliceInstance)
		assert.Equal(t, description, "this is a test")
		assert.Contains(t, discoveryLink, "/discovery?state=")
	})

	t.Run("GetSharing", func(t *testing.T) {
		eA := httpexpect.Default(t, tsA.URL)

		obj := eA.GET("/sharings/"+sharingID).
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		assertSharingIsCorrectOnSharer(t, obj, sharingID, aliceInstance)
	})

	t.Run("Discovery", func(t *testing.T) {
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
			Contains("Connect to your Cozy").
			Contains(`<input type="hidden" name="state" value="` + state)

		redirectHeader := eA.POST(u.Path).
			WithFormField("state", state).
			WithFormField("slug", tsB.URL).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(302).Header("Location")

		authorizeLink = redirectHeader.NotEmpty().Raw()
		redirectHeader.Contains(tsB.URL)
		redirectHeader.Contains("/auth/authorize/sharing")

		assertSharingRequestHasBeenCreated(t, aliceInstance, bobInstance, tsB.URL)
	})

	t.Run("AuthorizeSharing", func(t *testing.T) {
		u, err := url.Parse(authorizeLink)
		assert.NoError(t, err)
		sharingID = u.Query()["sharing_id"][0]
		state := u.Query()["state"][0]

		eB := httpexpect.Default(t, tsB.URL)

		// Bob login
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
		// End bob login

		fakeAliceInstance(t, bobInstance, tsA.URL)

		t.Logf("redirect header: %q\n\n", authorizeLink)

		body := eB.GET(u.Path).
			WithQuery("sharing_id", sharingID).
			WithQuery("state", state).
			Expect().Status(200).
			HasContentType("text/html", "utf-8").
			Body()

		body.Contains("and you can collaborate by editing the document")
		body.Contains(`<input type="hidden" name="sharing_id" value="` + sharingID)
		body.Contains(`<input type="hidden" name="state" value="` + state)
		body.Contains(`<span class="filetype-other filetype">`)

		matches := body.Match(`<input type="hidden" name="csrf_token" value="(\w+)"`)
		matches.Length().IsEqual(2)

		eB.POST("/auth/authorize/sharing").
			WithFormField("state", state).
			WithFormField("sharing_id", sharingID).
			WithFormField("csrf_token", token).
			WithFormField("synchronize", true).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Contains("testapp." + bobInstance.Domain)

		assertCredentialsHasBeenExchanged(t, aliceInstance, bobInstance, tsA.URL, tsB.URL)
	})

	t.Run("DelegateAddRecipientByCozyURL", func(t *testing.T) {
		assert.NotEmpty(t, bobAppToken)
		edwardContact = createContact(t, bobInstance, "Edward", "edward@example.net")

		eB := httpexpect.Default(t, tsB.URL)

		obj := eB.POST("/sharings/"+sharingID+"/recipients").
			WithHeader("Authorization", "Bearer "+bobAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "` + consts.Sharings + `",
          "relationships": {
            "recipients": {
              "data": [{"id": "` + edwardContact.ID() + `", "type": "` + edwardContact.DocType() + `"}]
            }
          }
        }
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		attrs := data.Value("attributes").Object()

		members := attrs.Value("members").Array()
		members.Length().IsEqual(4)

		owner := members.Value(0).Object()
		owner.HasValue("status", "owner")
		owner.HasValue("public_name", "Alice")
		owner.HasValue("email", "alice@example.net")

		bob := members.Value(1).Object()
		bob.HasValue("status", "ready")
		bob.HasValue("email", "bob@example.net")

		dave := members.Value(2).Object()
		dave.HasValue("status", "pending")
		dave.HasValue("email", "dave@example.net")
		dave.HasValue("read_only", true)

		edward := members.Value(3).Object()
		edward.HasValue("name", "Edward")
		edward.HasValue("email", "edward@example.net")
	})

	t.Run("CreateSharingWithGroup", func(t *testing.T) {
		eA := httpexpect.Default(t, tsA.URL)
		require.NotEmpty(t, aliceAppToken)

		group := createGroup(t, aliceInstance, "friends")
		require.NotNil(t, group)
		fionaContact := addContactToGroup(t, aliceInstance, group, "Fiona")
		require.NotNil(t, fionaContact)
		gabyContact := addContactToGroup(t, aliceInstance, group, "Gaby")
		require.NotNil(t, gabyContact)

		obj := eA.POST("/sharings/").
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "` + consts.Sharings + `",
          "attributes": {
            "description":  "this is a test with a group",
            "open_sharing": true,
            "rules": [{
                "title": "test group",
                "doctype": "` + iocozytests + `",
                "values": ["000001"],
                "add": "sync"
              }]
          },
          "relationships": {
            "recipients": {
              "data": [{"id": "` + group.ID() + `", "type": "` + consts.Groups + `"}]
            }
          }
        }
      }`)).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		attrs := data.Value("attributes").Object()
		attrs.HasValue("description", "this is a test with a group")
		members := attrs.Value("members").Array()
		members.Length().IsEqual(3)

		owner := members.Value(0).Object()
		owner.HasValue("status", "owner")
		owner.HasValue("public_name", "Alice")
		owner.HasValue("email", "alice@example.net")
		owner.HasValue("instance", "http://"+aliceInstance.Domain)

		recipient := members.Value(1).Object()
		recipient.HasValue("status", "pending")
		recipient.HasValue("name", "Fiona")
		recipient.HasValue("email", "fiona@example.net")
		recipient.NotContainsKey("read_only")

		recipient = members.Value(2).Object()
		recipient.HasValue("status", "pending")
		recipient.HasValue("name", "Gaby")
		recipient.HasValue("email", "gaby@example.net")
		recipient.NotContainsKey("read_only")

		groups := attrs.Value("groups").Array()
		groups.Length().IsEqual(1)
		g := groups.Value(0).Object()
		g.HasValue("id", group.ID())
		g.HasValue("name", "friends")
		g.HasValue("members", []int{1, 2})
		g.HasValue("addedBy", 0)
	})

	t.Run("CreateSharingWithPreview", func(t *testing.T) {
		bobContact := createBobContact(t, aliceInstance)
		require.NotEmpty(t, aliceAppToken)
		require.NotNil(t, bobContact)

		eA := httpexpect.Default(t, tsA.URL)

		obj := eA.POST("/sharings/").
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "` + consts.Sharings + `",
          "attributes": {
            "description":  "this is a test with preview",
            "preview_path": "/preview",
            "rules": [{
                "title": "test two",
                "doctype": "` + iocozytests + `",
                "values": ["foobaz"]
              }]
          },
          "relationships": {
            "recipients": {
              "data": [{"id": "` + bobContact.ID() + `", "type": "` + bobContact.DocType() + `"}]
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

		data := obj.Value("data").Object()
		data.HasValue("type", consts.Sharings)
		sharingID = data.Value("id").String().NotEmpty().Raw()
		data.Value("meta").Object().Value("rev").String().NotEmpty()
		data.Value("links").Object().HasValue("self", "/sharings/"+sharingID)

		attrs := data.Value("attributes").Object()
		attrs.HasValue("description", "this is a test with preview")
		attrs.HasValue("app_slug", "testapp")
		attrs.HasValue("preview_path", "/preview")
		attrs.HasValue("owner", true)
		attrs.Value("created_at").String().AsDateTime(time.RFC3339)
		attrs.Value("updated_at").String().AsDateTime(time.RFC3339)
		attrs.NotContainsKey("credentials")

		members := attrs.Value("members").Array()
		assertSharingByAliceToBobAndDave(t, members, aliceInstance)

		rules := attrs.Value("rules").Array()
		rules.Length().IsEqual(1)
		rule := rules.Value(0).Object()
		rule.HasValue("title", "test two")
		rule.HasValue("doctype", iocozytests)
		rule.HasValue("values", []string{"foobaz"})

		description := assertInvitationMailWasSent(t, aliceInstance)
		assert.Equal(t, description, "this is a test with preview")
		assert.Contains(t, discoveryLink, aliceInstance.Domain)
		assert.Contains(t, discoveryLink, "/preview?sharecode=")
	})

	t.Run("DiscoveryWithPreview", func(t *testing.T) {
		u, err := url.Parse(discoveryLink)
		assert.NoError(t, err)
		sharecode := u.Query()["sharecode"][0]

		eA := httpexpect.Default(t, tsA.URL)

		t.Logf("sharcode: %q\n\n", sharecode)

		obj := eA.POST("/sharings/"+sharingID+"/discovery").
			WithHeader("Accept", "application/json").
			WithFormField("sharecode", sharecode).
			WithFormField("url", tsB.URL).
			Expect().Status(200).
			JSON().Object()

		redirectURI := obj.Value("redirect").String().Contains(tsB.URL).Raw()

		res, err := url.Parse(redirectURI)
		assert.NoError(t, err)
		assert.Equal(t, res.Path, "/auth/authorize/sharing")
		assert.Equal(t, res.Query()["sharing_id"][0], sharingID)
		assert.NotEmpty(t, res.Query()["state"][0])
	})

	t.Run("AddRecipient", func(t *testing.T) {
		require.NotEmpty(t, aliceAppToken)
		require.NotNil(t, charlieContact)

		eA := httpexpect.Default(t, tsA.URL)

		brothers := createGroup(t, aliceInstance, "brothers")
		require.NotNil(t, brothers)
		hugoContact := addContactToGroup(t, aliceInstance, brothers, "Hugo")
		require.NotNil(t, hugoContact)
		idrisContact := addContactToGroup(t, aliceInstance, brothers, "Idris")
		require.NotNil(t, idrisContact)

		obj := eA.POST("/sharings/"+sharingID+"/recipients").
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "` + consts.Sharings + `",
          "relationships": {
            "recipients": {
              "data": [
			    {"id": "` + charlieContact.ID() + `", "type": "` + consts.Contacts + `"},
			    {"id": "` + brothers.ID() + `", "type": "` + consts.Groups + `"}
		      ]
            }
          }
        }
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		attrs := data.Value("attributes").Object()
		members := attrs.Value("members").Array()

		members.Length().IsEqual(6)
		owner := members.Value(0).Object()
		owner.HasValue("status", "owner")
		owner.HasValue("public_name", "Alice")
		owner.HasValue("email", "alice@example.net")
		owner.HasValue("instance", "http://"+aliceInstance.Domain)

		bob := members.Value(1).Object()
		bob.HasValue("status", "pending")
		bob.HasValue("name", "Bob")
		bob.HasValue("email", "bob@example.net")

		dave := members.Value(2).Object()
		dave.HasValue("status", "pending")
		dave.HasValue("name", "Dave")
		dave.HasValue("email", "dave@example.net")
		dave.HasValue("read_only", true)

		charlie := members.Value(3).Object()
		charlie.HasValue("status", "pending")
		charlie.HasValue("name", "Charlie")
		charlie.HasValue("email", "charlie@example.net")

		hugo := members.Value(4).Object()
		hugo.HasValue("status", "pending")
		hugo.HasValue("name", "Hugo")
		hugo.HasValue("email", "hugo@example.net")

		idris := members.Value(5).Object()
		idris.HasValue("status", "pending")
		idris.HasValue("name", "Idris")
		idris.HasValue("email", "idris@example.net")

		groups := attrs.Value("groups").Array()
		groups.Length().IsEqual(1)
		g := groups.Value(0).Object()
		g.HasValue("id", brothers.ID())
		g.HasValue("name", "brothers")
		g.HasValue("members", []int{4, 5})
		g.HasValue("addedBy", 0)
	})

	t.Run("RevokedSharingWithPreview", func(t *testing.T) {
		sharecode := strings.Split(discoveryLink, "=")[1]

		eA := httpexpect.Default(t, tsA.URL)

		obj := eA.GET("/permissions/self").
			WithHeader("Authorization", "Bearer "+sharecode).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		sourceID := obj.Value("data").Object().
			Value("attributes").Object().
			Value("source_id").String().NotEmpty().Raw()
		sharingID = strings.Split(sourceID, "/")[1]

		// Adding a new member to the sharing
		newMemberMail := "foo@bar.com"
		sharingDoc, err := sharing.FindSharing(aliceInstance, sharingID)
		require.NoError(t, err)

		_, err = sharingDoc.AddDelegatedContact(aliceInstance, newMemberMail, "", true)
		require.NoError(t, err)
		perms, err := permission.GetForSharePreview(aliceInstance, sharingID)
		require.NoError(t, err)
		fooShareCode, err := aliceInstance.CreateShareCode(newMemberMail)
		require.NoError(t, err)

		// Adding its sharecode
		codes := perms.Codes
		codes[newMemberMail] = fooShareCode
		perms.PatchCodes(codes)
		assert.NoError(t, couchdb.UpdateDoc(aliceInstance, perms))
		assert.NoError(t, couchdb.UpdateDoc(aliceInstance, sharingDoc))

		// Assert he has access to the sharing preview
		eA.GET("/permissions/self").
			WithHeader("Authorization", "Bearer "+fooShareCode).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(200)

		// Check the member status has been updated to "seen"
		sharingDoc, err = sharing.FindSharing(aliceInstance, sharingID)
		assert.NoError(t, err)
		member, err := sharingDoc.FindMemberBySharecode(aliceInstance, fooShareCode)
		assert.NoError(t, err)
		assert.Equal(t, "seen", member.Status)

		// Now, revoking the fresh user from the sharing
		member, err = sharingDoc.FindMemberBySharecode(aliceInstance, fooShareCode)
		assert.NoError(t, err)
		index := 0
		for i := range sharingDoc.Members {
			if member == &sharingDoc.Members[i] {
				index = i
				break
			}
		}
		err = sharingDoc.RevokeMember(aliceInstance, index)
		assert.NoError(t, err)
		assert.Equal(t, "revoked", member.Status)

		// Try to get permissions/self, we should get a 400
		eA.GET("/permissions/self").
			WithHeader("Authorization", "Bearer "+fooShareCode).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(400).
			Body().Contains("Invalid JWT")
	})

	t.Run("CheckPermissions", func(t *testing.T) {
		bobContact := createBobContact(t, aliceInstance)
		assert.NotNil(t, bobContact)

		eA := httpexpect.Default(t, tsA.URL)

		eA.POST("/sharings").
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "` + consts.Sharings + `",
          "attributes": {
            "description":  "this is a test",
            "preview_path": "/preview",
            "rules": [
              {
                "title": "test one",
                "doctype": "` + iocozytests + `",
                "values": ["000000"],
                "add": "sync"
              },{
                "title": "test two",
                "doctype": "` + consts.Contacts + `",
                "values": ["000000"],
                "add": "sync"
              }]
          },
          "relationships": {
            "recipients": {
              "data": [{"id": "` + bobContact.ID() + `", "type": "` + bobContact.DocType() + `"}]
            }
          }
        }
      }`)).
			Expect().Status(403)

		other := &sharing.Sharing{
			Description: "Another sharing",
			Rules: []sharing.Rule{
				{
					Title:   "a directory",
					DocType: consts.Files,
					Values:  []string{"6836cc06-33e9-11e8-8157-dfc1aca099b6"},
				},
			},
		}
		assert.NoError(t, other.BeOwner(aliceInstance, "home"))
		assert.NoError(t, other.AddContact(aliceInstance, bobContact.ID(), false))
		_, err := other.Create(aliceInstance)
		assert.NoError(t, err)

		eA.GET("/sharings/"+other.ID()).
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(403)
	})

	t.Run("CheckSharingInfoByDocType", func(t *testing.T) {
		sharedDocs1 := []string{"fakeid1", "fakeid2", "fakeid3"}
		sharedDocs2 := []string{"fakeid4", "fakeid5"}
		s1 := createSharing(t, aliceInstance, sharedDocs1, tsB.URL)
		s2 := createSharing(t, aliceInstance, sharedDocs2, tsB.URL)

		for _, id := range sharedDocs1 {
			sid := iocozytests + "/" + id
			sd, errs := createSharedDoc(aliceInstance, sid, s1.ID())
			assert.NoError(t, errs)
			assert.NotNil(t, sd)
		}
		for _, id := range sharedDocs2 {
			sid := iocozytests + "/" + id
			sd, errs := createSharedDoc(aliceInstance, sid, s2.ID())
			assert.NoError(t, errs)
			assert.NotNil(t, sd)
		}

		eA := httpexpect.Default(t, tsA.URL)

		obj := eA.GET("/sharings/doctype/"+iocozytests).
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		s1Found := false
		s2Found := false

		for _, itm := range obj.Value("data").Array().Iter() {
			data := itm.Object()
			data.HasValue("type", consts.Sharings)
			sharingID = data.Value("id").String().NotEmpty().Raw()
			rel := data.Value("relationships").Object()
			sharedDocs := rel.Value("shared_docs").Object()

			if sharingID == s1.ID() {
				sharedDocsData := sharedDocs.Value("data").Array()
				sharedDocsData.Value(0).Object().Value("id").IsEqual("fakeid1")
				sharedDocsData.Value(1).Object().Value("id").IsEqual("fakeid2")
				sharedDocsData.Value(2).Object().Value("id").IsEqual("fakeid3")
				s1Found = true
			}

			if sharingID == s2.ID() {
				sharedDocsData := sharedDocs.Value("data").Array()
				sharedDocsData.Value(0).Object().Value("id").IsEqual("fakeid4")
				sharedDocsData.Value(1).Object().Value("id").IsEqual("fakeid5")
				s2Found = true
			}
		}

		assert.Equal(t, true, s1Found)
		assert.Equal(t, true, s2Found)

		eA.GET("/sharings/doctype/io.cozy.tests.notyet").
			WithHeader("Authorization", "Bearer "+aliceAppTokenWildcard).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(200)

		eA.GET("/sharings/doctype/"+iocozytests).
			WithHeader("Authorization", "Bearer "+aliceAppTokenWildcard).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(200)

		eA.GET("/sharings/doctype/io.cozy.things").
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(401)
	})

	t.Run("RevokeSharing", func(t *testing.T) {
		sharedDocs := []string{"mygreatid1", "mygreatid2"}
		sharedRefs := []*sharing.SharedRef{}
		s := createSharing(t, aliceInstance, sharedDocs, tsB.URL)

		for _, id := range sharedDocs {
			sid := iocozytests + "/" + id
			sd, errs := createSharedDoc(aliceInstance, sid, s.SID)
			sharedRefs = append(sharedRefs, sd)
			assert.NoError(t, errs)
			assert.NotNil(t, sd)
		}

		cli, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[1])
		assert.NoError(t, err)
		s.Credentials[0].Client = sharing.ConvertOAuthClient(cli)
		token, err := sharing.CreateAccessToken(aliceInstance, cli, s.SID, permission.ALL)
		assert.NoError(t, err)
		s.Credentials[0].AccessToken = token
		s.Members[1].Status = sharing.MemberStatusReady

		err = couchdb.UpdateDoc(aliceInstance, s)
		assert.NoError(t, err)

		err = s.AddTrackTriggers(aliceInstance)
		assert.NoError(t, err)
		err = s.AddReplicateTrigger(aliceInstance)
		assert.NoError(t, err)

		eA := httpexpect.Default(t, tsA.URL)

		eA.DELETE("/sharings/"+s.ID()+"/recipients").
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(204)

		var sRevoke sharing.Sharing
		err = couchdb.GetDoc(aliceInstance, s.DocType(), s.SID, &sRevoke)
		assert.NoError(t, err)

		assert.Equal(t, "", sRevoke.Triggers.TrackID)
		assert.Empty(t, sRevoke.Triggers.TrackIDs)
		assert.Equal(t, "", sRevoke.Triggers.ReplicateID)
		assert.Equal(t, "", sRevoke.Triggers.UploadID)
		assert.Equal(t, false, sRevoke.Active)

		var sdoc sharing.SharedRef
		err = couchdb.GetDoc(aliceInstance, sharedRefs[0].DocType(), sharedRefs[0].ID(), &sdoc)
		assert.EqualError(t, err, "CouchDB(not_found): deleted")
		err = couchdb.GetDoc(aliceInstance, sharedRefs[1].DocType(), sharedRefs[1].ID(), &sdoc)
		assert.EqualError(t, err, "CouchDB(not_found): deleted")
	})

	t.Run("RevokeRecipient", func(t *testing.T) {
		sharedDocs := []string{"mygreatid3", "mygreatid4"}
		sharedRefs := []*sharing.SharedRef{}
		s := createSharing(t, aliceInstance, sharedDocs, tsB.URL)

		for _, id := range sharedDocs {
			sid := iocozytests + "/" + id
			sd, errs := createSharedDoc(aliceInstance, sid, s.SID)
			sharedRefs = append(sharedRefs, sd)
			assert.NoError(t, errs)
			assert.NotNil(t, sd)
		}

		cli, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[1])
		assert.NoError(t, err)
		s.Credentials[0].Client = sharing.ConvertOAuthClient(cli)
		token, err := sharing.CreateAccessToken(aliceInstance, cli, s.SID, permission.ALL)
		assert.NoError(t, err)
		s.Credentials[0].AccessToken = token
		s.Members[1].Status = sharing.MemberStatusReady

		s.Members = append(s.Members, sharing.Member{
			Status:   sharing.MemberStatusReady,
			Name:     "Charlie",
			Email:    "charlie@cozy.local",
			Instance: tsB.URL,
		})

		clientC, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[2])
		assert.NoError(t, err)
		tokenC, err := sharing.CreateAccessToken(aliceInstance, clientC, s.SID, permission.ALL)
		assert.NoError(t, err)
		s.Credentials = append(s.Credentials, sharing.Credentials{
			Client:      sharing.ConvertOAuthClient(clientC),
			AccessToken: tokenC,
		})

		err = couchdb.UpdateDoc(aliceInstance, s)
		assert.NoError(t, err)

		err = s.AddTrackTriggers(aliceInstance)
		assert.NoError(t, err)
		err = s.AddReplicateTrigger(aliceInstance)
		assert.NoError(t, err)

		eA := httpexpect.Default(t, tsA.URL)

		eA.DELETE("/sharings/"+s.ID()+"/recipients/1").
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(204)

		assertOneRecipientIsRevoked(t, s, aliceInstance)

		eA.DELETE("/sharings/"+s.ID()+"/recipients/2").
			WithHeader("Authorization", "Bearer "+aliceAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(204)

		assertLastRecipientIsRevoked(t, s, sharedRefs, aliceInstance)
	})

	t.Run("RevocationFromRecipient", func(t *testing.T) {
		sharedDocs := []string{"mygreatid5", "mygreatid6"}
		sharedRefs := []*sharing.SharedRef{}
		s := createSharing(t, aliceInstance, sharedDocs, tsB.URL)
		for _, id := range sharedDocs {
			sid := iocozytests + "/" + id
			sd, errs := createSharedDoc(aliceInstance, sid, s.SID)
			sharedRefs = append(sharedRefs, sd)
			assert.NoError(t, errs)
			assert.NotNil(t, sd)
		}

		cli, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[1])
		assert.NoError(t, err)
		s.Credentials[0].InboundClientID = cli.ClientID
		s.Credentials[0].Client = sharing.ConvertOAuthClient(cli)
		token, err := sharing.CreateAccessToken(aliceInstance, cli, s.SID, permission.ALL)
		assert.NoError(t, err)
		s.Credentials[0].AccessToken = token
		s.Members[1].Status = sharing.MemberStatusReady

		s.Members = append(s.Members, sharing.Member{
			Status:   sharing.MemberStatusReady,
			Name:     "Charlie",
			Email:    "charlie@cozy.local",
			Instance: tsB.URL,
		})
		clientC, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[2])
		assert.NoError(t, err)
		tokenC, err := sharing.CreateAccessToken(aliceInstance, clientC, s.SID, permission.ALL)
		assert.NoError(t, err)
		s.Credentials = append(s.Credentials, sharing.Credentials{
			Client:          sharing.ConvertOAuthClient(clientC),
			AccessToken:     tokenC,
			InboundClientID: clientC.ClientID,
		})

		err = couchdb.UpdateDoc(aliceInstance, s)
		assert.NoError(t, err)

		err = s.AddTrackTriggers(aliceInstance)
		assert.NoError(t, err)
		err = s.AddReplicateTrigger(aliceInstance)
		assert.NoError(t, err)

		eA := httpexpect.Default(t, tsA.URL)

		eA.DELETE("/sharings/"+s.ID()+"/answer").
			WithHeader("Authorization", "Bearer "+s.Credentials[0].AccessToken.AccessToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(204)

		assertOneRecipientIsRevoked(t, s, aliceInstance)

		eA.DELETE("/sharings/"+s.ID()+"/answer").
			WithHeader("Authorization", "Bearer "+s.Credentials[1].AccessToken.AccessToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(204)

		assertLastRecipientIsRevoked(t, s, sharedRefs, aliceInstance)
	})

	t.Run("ClearAppInURL", func(t *testing.T) {
		host := sharings.ClearAppInURL("https://example.mycozy.cloud/")
		assert.Equal(t, "https://example.mycozy.cloud/", host)
		host = sharings.ClearAppInURL("https://example-drive.mycozy.cloud/")
		assert.Equal(t, "https://example.mycozy.cloud/", host)
		host = sharings.ClearAppInURL("https://my-cozy.example.net/")
		assert.Equal(t, "https://my-cozy.example.net/", host)
	})
}

func assertSharingByAliceToBobAndDave(t *testing.T, obj *httpexpect.Array, instance *instance.Instance) {
	t.Helper()

	obj.Length().IsEqual(3)

	owner := obj.Value(0).Object()
	owner.HasValue("status", "owner")
	owner.HasValue("public_name", "Alice")
	owner.HasValue("email", "alice@example.net")
	owner.HasValue("instance", "http://"+instance.Domain)

	recipient := obj.Value(1).Object()
	recipient.HasValue("status", "pending")
	recipient.HasValue("name", "Bob")
	recipient.HasValue("email", "bob@example.net")
	recipient.NotContainsKey("read_only")

	recipient2 := obj.Value(2).Object()
	recipient2.HasValue("status", "pending")
	recipient2.HasValue("name", "Dave")
	recipient2.HasValue("email", "dave@example.net")
	recipient2.HasValue("read_only", true)
}

func assertSharingIsCorrectOnSharer(t *testing.T, obj *httpexpect.Object, sharingID string, instance *instance.Instance) {
	t.Helper()

	data := obj.Value("data").Object()

	data.HasValue("type", consts.Sharings)
	data.Value("meta").Object().Value("rev").String().NotEmpty()
	data.Value("links").Object().HasValue("self", "/sharings/"+sharingID)

	attrs := data.Value("attributes").Object()
	attrs.HasValue("description", "this is a test")
	attrs.HasValue("app_slug", "testapp")
	attrs.HasValue("owner", true)
	attrs.Value("created_at").String().AsDateTime(time.RFC3339)
	attrs.Value("updated_at").String().AsDateTime(time.RFC3339)
	attrs.NotContainsKey("credentials")

	assertSharingByAliceToBobAndDave(t, attrs.Value("members").Array(), instance)

	rules := attrs.Value("rules").Array()
	rules.Length().IsEqual(1)
	rule := rules.Value(0).Object()
	rule.HasValue("title", "test one")
	rule.HasValue("doctype", iocozytests)
	rule.HasValue("values", []interface{}{"000000"})
}

func assertInvitationMailWasSent(t *testing.T, instance *instance.Instance) string {
	var jobs []job.Job
	couchReq := &couchdb.FindRequest{
		UseIndex: "by-worker-and-state",
		Selector: mango.And(
			mango.Equal("worker", "sendmail"),
			mango.Exists("state"),
		),
		Sort: mango.SortBy{
			mango.SortByField{Field: "worker", Direction: "desc"},
		},
		Limit: 2,
	}
	err := couchdb.FindDocs(instance, consts.Jobs, couchReq, &jobs)
	assert.NoError(t, err)
	assert.Len(t, jobs, 2)
	var msg map[string]interface{}
	// Ignore the mail sent to Dave
	err = json.Unmarshal(jobs[0].Message, &msg)
	assert.NoError(t, err)
	if msg["recipient_name"] == "Dave" {
		err = json.Unmarshal(jobs[1].Message, &msg)
		assert.NoError(t, err)
	}
	assert.Equal(t, msg["mode"], "from")
	assert.Equal(t, msg["template_name"], "sharing_request")
	values := msg["template_values"].(map[string]interface{})
	assert.Equal(t, values["SharerPublicName"], "Alice")
	discoveryLink = values["SharingLink"].(string)
	return values["Description"].(string)
}

func assertSharingRequestHasBeenCreated(t *testing.T, instanceA, instanceB *instance.Instance, serverURL string) {
	var results []*sharing.Sharing
	req := couchdb.AllDocsRequest{}
	err := couchdb.GetAllDocs(instanceB, consts.Sharings, &req, &results)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	s := results[0]
	assert.Equal(t, s.SID, sharingID)
	assert.False(t, s.Active)
	assert.False(t, s.Owner)
	assert.Equal(t, s.Description, "this is a test")
	assert.Equal(t, s.AppSlug, "testapp")

	assert.Len(t, s.Members, 3)
	owner := s.Members[0]
	assert.Equal(t, owner.Status, "owner")
	assert.Equal(t, owner.PublicName, "Alice")
	assert.Equal(t, owner.Email, "alice@example.net")
	assert.Equal(t, owner.Instance, "http://"+instanceA.Domain)
	recipient := s.Members[1]
	assert.Equal(t, recipient.Status, "pending")
	assert.Equal(t, recipient.Email, "bob@example.net")
	assert.Equal(t, recipient.Instance, serverURL)
	recipient = s.Members[2]
	assert.Equal(t, recipient.Status, "pending")
	assert.Equal(t, recipient.Email, "dave@example.net")
	assert.Equal(t, recipient.ReadOnly, true)

	assert.Len(t, s.Rules, 1)
	rule := s.Rules[0]
	assert.Equal(t, rule.Title, "test one")
	assert.Equal(t, rule.DocType, iocozytests)
	assert.NotEmpty(t, rule.Values)
}

func fakeAliceInstance(t *testing.T, instance *instance.Instance, serverURL string) {
	var results []*sharing.Sharing
	req := couchdb.AllDocsRequest{}
	err := couchdb.GetAllDocs(instance, consts.Sharings, &req, &results)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	s := results[0]
	assert.Len(t, s.Members, 3)
	s.Members[0].Instance = serverURL
	err = couchdb.UpdateDoc(instance, s)
	assert.NoError(t, err)
}

func assertCredentialsHasBeenExchanged(t *testing.T, instanceA, instanceB *instance.Instance, urlA, urlB string) {
	var resultsA []map[string]interface{}
	req := couchdb.AllDocsRequest{}
	err := couchdb.GetAllDocs(instanceB, consts.OAuthClients, &req, &resultsA)
	assert.NoError(t, err)
	assert.True(t, len(resultsA) > 0)
	clientA := resultsA[len(resultsA)-1]
	assert.Equal(t, clientA["client_kind"], "sharing")
	assert.Equal(t, clientA["client_uri"], urlA+"/")
	assert.Equal(t, clientA["client_name"], "Sharing Alice")

	var resultsB []map[string]interface{}
	err = couchdb.GetAllDocs(instanceA, consts.OAuthClients, &req, &resultsB)
	assert.NoError(t, err)
	assert.True(t, len(resultsB) > 0)
	clientB := resultsB[len(resultsB)-1]
	assert.Equal(t, clientB["client_kind"], "sharing")
	assert.Equal(t, clientB["client_uri"], urlB+"/")
	assert.Equal(t, clientB["client_name"], "Sharing Bob")

	var sharingsA []*sharing.Sharing
	err = couchdb.GetAllDocs(instanceA, consts.Sharings, &req, &sharingsA)
	assert.NoError(t, err)
	assert.True(t, len(sharingsA) > 0)
	assert.Len(t, sharingsA[0].Credentials, 2)
	credentials := sharingsA[0].Credentials[0]
	if assert.NotNil(t, credentials.Client) {
		assert.Equal(t, credentials.Client.ClientID, clientA["_id"])
	}
	if assert.NotNil(t, credentials.AccessToken) {
		assert.NotEmpty(t, credentials.AccessToken.AccessToken)
		assert.NotEmpty(t, credentials.AccessToken.RefreshToken)
		aliceAccessToken = credentials.AccessToken.AccessToken
	}
	assert.Equal(t, sharingsA[0].Members[1].Status, "ready")
	assert.Equal(t, sharingsA[0].Members[2].Status, "pending")

	var sharingsB []*sharing.Sharing
	err = couchdb.GetAllDocs(instanceB, consts.Sharings, &req, &sharingsB)
	assert.NoError(t, err)
	assert.True(t, len(sharingsB) > 0)
	assert.Len(t, sharingsB[0].Credentials, 1)
	credentials = sharingsB[0].Credentials[0]
	if assert.NotNil(t, credentials.Client) {
		assert.Equal(t, credentials.Client.ClientID, clientB["_id"])
	}
	if assert.NotNil(t, credentials.AccessToken) {
		assert.NotEmpty(t, credentials.AccessToken.AccessToken)
		assert.NotEmpty(t, credentials.AccessToken.RefreshToken)
	}
}

func assertOneRecipientIsRevoked(t *testing.T, s *sharing.Sharing, instance *instance.Instance) {
	var sRevoked sharing.Sharing
	err := couchdb.GetDoc(instance, s.DocType(), s.SID, &sRevoked)
	assert.NoError(t, err)

	assert.Equal(t, sharing.MemberStatusRevoked, sRevoked.Members[1].Status)
	assert.Equal(t, sharing.MemberStatusReady, sRevoked.Members[2].Status)
	assert.NotEmpty(t, sRevoked.Triggers.TrackIDs)
	assert.NotEmpty(t, sRevoked.Triggers.ReplicateID)
	assert.True(t, sRevoked.Active)
}

func assertLastRecipientIsRevoked(t *testing.T, s *sharing.Sharing, refs []*sharing.SharedRef, instance *instance.Instance) {
	var sRevoked sharing.Sharing
	err := couchdb.GetDoc(instance, s.DocType(), s.SID, &sRevoked)
	assert.NoError(t, err)

	assert.Equal(t, sharing.MemberStatusRevoked, sRevoked.Members[1].Status)
	assert.Equal(t, sharing.MemberStatusRevoked, sRevoked.Members[2].Status)
	assert.Empty(t, sRevoked.Triggers.TrackIDs)
	assert.Empty(t, sRevoked.Triggers.ReplicateID)
	assert.False(t, sRevoked.Active)

	var sdoc sharing.SharedRef
	err = couchdb.GetDoc(instance, refs[0].DocType(), refs[0].ID(), &sdoc)
	assert.EqualError(t, err, "CouchDB(not_found): deleted")
	err = couchdb.GetDoc(instance, refs[1].DocType(), refs[1].ID(), &sdoc)
	assert.EqualError(t, err, "CouchDB(not_found): deleted")
}

func createBobContact(t *testing.T, instance *instance.Instance) *contact.Contact {
	return createContact(t, instance, "Bob", "bob@example.net")
}

func createContact(t *testing.T, inst *instance.Instance, name, email string) *contact.Contact {
	t.Helper()
	mail := map[string]interface{}{"address": email}
	c := contact.New()
	c.M["fullname"] = name
	c.M["email"] = []interface{}{mail}
	require.NoError(t, couchdb.CreateDoc(inst, c))
	return c
}

func createGroup(t *testing.T, inst *instance.Instance, name string) *contact.Group {
	t.Helper()
	g := contact.NewGroup()
	g.M["name"] = name
	require.NoError(t, couchdb.CreateDoc(inst, g))
	return g
}

func addContactToGroup(t *testing.T, inst *instance.Instance, g *contact.Group, contactName string) *contact.Contact {
	t.Helper()
	email := strings.ToLower(contactName) + "@example.net"
	mail := map[string]interface{}{"address": email}
	c := contact.New()
	c.M["fullname"] = contactName
	c.M["email"] = []interface{}{mail}
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

func createSharing(t *testing.T, inst *instance.Instance, values []string, serverURL string) *sharing.Sharing {
	bobContact := createBobContact(t, inst)
	assert.NotNil(t, bobContact)

	r := sharing.Rule{
		Title:   "test",
		DocType: iocozytests,
		Values:  values,
		Add:     sharing.ActionRuleSync,
	}
	mail, err := bobContact.ToMailAddress()
	assert.NoError(t, err)
	m := sharing.Member{
		Name:     bobContact.Get("fullname").(string),
		Email:    mail.Email,
		Instance: serverURL,
	}
	s := &sharing.Sharing{
		Owner: true,
		Rules: []sharing.Rule{r},
	}
	s.Credentials = append(s.Credentials, sharing.Credentials{})
	err = s.BeOwner(inst, "")
	assert.NoError(t, err)
	s.Members = append(s.Members, m)

	err = couchdb.CreateDoc(inst, s)
	assert.NoError(t, err)
	assert.NotNil(t, s)
	return s
}

func createSharedDoc(inst *instance.Instance, id, sharingID string) (*sharing.SharedRef, error) {
	ref := &sharing.SharedRef{
		SID:       id,
		Revisions: &sharing.RevsTree{Rev: "1-aaa"},
		Infos: map[string]sharing.SharedInfo{
			sharingID: {Rule: 0},
		},
	}
	err := couchdb.CreateNamedDocWithDB(inst, ref)
	if err != nil {
		return nil, err
	}
	return ref, nil
}

func generateAppToken(inst *instance.Instance, slug, doctype string) string {
	rules := permission.Set{
		permission.Rule{
			Type:  doctype,
			Verbs: permission.ALL,
		},
	}
	permReq := permission.Permission{
		Permissions: rules,
		Type:        permission.TypeWebapp,
		SourceID:    consts.Apps + "/" + slug,
	}
	err := couchdb.CreateDoc(inst, &permReq)
	if err != nil {
		return ""
	}
	manifest := &couchdb.JSONDoc{
		Type: consts.Apps,
		M: map[string]interface{}{
			"_id":         consts.Apps + "/" + slug,
			"slug":        slug,
			"permissions": rules,
		},
	}
	err = couchdb.CreateNamedDocWithDB(inst, manifest)
	if err != nil {
		return ""
	}
	return inst.BuildAppToken(slug, "")
}
