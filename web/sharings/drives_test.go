package sharings_test

import (
	"net/url"
	"testing"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/sharings"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedDrives(t *testing.T) {
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

	// Prepare the Cozy instance for the ACME organization
	setup := testutils.NewSetup(t, t.Name()+"_acme")
	acmeInstance := setup.GetTestInstance(&lifecycle.Options{
		Email:      "acme@example.net",
		PublicName: "ACME",
	})
	acmeAppToken := generateAppToken(acmeInstance, "drive", "io.cozy.files")
	tsA := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
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
	// bettyAppToken := generateAppToken(bettyInstance, "drive", consts.Files)
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

	t.Run("CreateSharedDrive", func(t *testing.T) {
		eA := httpexpect.Default(t, tsA.URL)

		// Create a shared drive
		contact := createContact(t, acmeInstance, "Betty", "betty@example.net")
		daveContact := createContact(t, acmeInstance, "Dave", "dave@example.net")
		require.NotNil(t, contact)
		obj := eA.POST("/sharings/").
			WithHeader("Authorization", "Bearer "+acmeAppToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "` + consts.Sharings + `",
          "attributes": {
            "description":  "Drive for the product team",
            "drive": true,
            "rules": [{
                "title": "Product team",
                "doctype": "` + consts.Files + `",
                "values": ["612acf1c-1d72-11e8-b043-ef239d3074dd"]
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
		rule.Value("values").Array().Value(0).IsEqual("612acf1c-1d72-11e8-b043-ef239d3074dd")
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
			Contains("Connect to your Cozy").
			Contains(`<input type="hidden" name="state" value="` + state)

		redirectHeader := eA.POST(u.Path).
			WithFormField("state", state).
			WithFormField("slug", tsB.URL).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(302).Header("Location")

		redirectHeader.Contains(tsB.URL)
		redirectHeader.Contains("/auth/authorize/sharing")
		authorizeLink = redirectHeader.NotEmpty().Raw()
		t.Logf("redirect header: %q\n\n", authorizeLink)

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
		driveLink := redirectHeader.NotEmpty().Raw()
		t.Logf("redirect header: %q\n\n", driveLink)
	})
}
