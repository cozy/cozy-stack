package shortcuts

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/gavv/httpexpect/v2"
)

const targetURL = "https://alice-photos.cozy.example/#/photos/629fb233be550a21174ac8e19f0043af"

func TestShortcuts(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var shortcutID string

	config.UseTestFile()
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	// _ = setup.GetTestInstance()
	_, token := setup.GetTestClient(consts.Files)

	ts := setup.GetTestServer("/shortcuts", Routes)
	t.Cleanup(ts.Close)

	// ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = weberrors.ErrorHandler

	t.Run("CreateShortcut", func(t *testing.T) {
		e := httpexpect.Default(t, ts.URL)

		obj := e.POST("/shortcuts").
			WithHeader("Content-Type", "application/vnd.api+json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(`{
        "data": {
          "type": "io.cozy.files.shortcuts",
          "attributes": {
            "name": "sunset.jpg.url",
            "url": "` + targetURL + `",
            "metadata": {
              "target": {
                "cozyMetadata": {
                  "instance": "https://alice.cozy.example/"
                },
                "app": "photos",
                "_type": "io.cozy.files",
                "mime": "image/jpg"
              }
            }
          }
        }
      }`)).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()

		data.ValueEqual("type", "io.cozy.files")
		shortcutID = data.Value("id").String().NotEmpty().Raw()

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("type", "file")
		attrs.ValueEqual("name", "sunset.jpg.url")
		attrs.ValueEqual("mime", "application/internet-shortcut")

		cozyMeta := attrs.Value("cozyMetadata").Object()
		cozyMeta.Value("createdAt").String().DateTime(time.RFC3339)
		cozyMeta.Value("createdOn").String().Contains("https://testshortcuts_")

		target := attrs.Value("metadata").Object().Value("target").Object()
		target.ValueEqual("app", "photos")
		target.ValueEqual("_type", "io.cozy.files")
		target.ValueEqual("mime", "image/jpg")
	})

	t.Run("GetShortcut", func(t *testing.T) {
		e := httpexpect.Default(t, ts.URL)

		obj := e.GET("/shortcuts/"+shortcutID).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.files.shortcuts")
		data.ValueEqual("id", shortcutID)

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("name", "sunset.jpg.url")
		attrs.ValueEqual("dir_id", "io.cozy.files.root-dir")
		attrs.ValueEqual("url", targetURL)

		target := attrs.Value("metadata").Object().Value("target").Object()
		target.ValueEqual("app", "photos")
		target.ValueEqual("_type", "io.cozy.files")
		target.ValueEqual("mime", "image/jpg")

		e.GET("/shortcuts/"+shortcutID).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "text/html").
			Expect().Status(303).
			Header("Location").Equal(targetURL)
	})
}
