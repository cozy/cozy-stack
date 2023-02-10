package files

import (
	"net/url"
	"strconv"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	require.NoError(t, loadLocale(), "Could not load default locale translations")

	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())

	config.GetConfig().Fs.URL = &url.URL{
		Scheme: "file",
		Host:   "localhost",
		Path:   t.TempDir(),
	}

	_, token := setup.GetTestClient(consts.Files + " " + consts.CertifiedCarbonCopy + " " + consts.CertifiedElectronicSafe)
	ts := setup.GetTestServer("/files", Routes, func(r *echo.Echo) *echo.Echo {
		secure := middlewares.Secure(&middlewares.SecureConfig{
			CSPDefaultSrc:     []middlewares.CSPSource{middlewares.CSPSrcSelf},
			CSPFrameAncestors: []middlewares.CSPSource{middlewares.CSPSrcNone},
		})
		r.Use(secure)
		return r
	})
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	t.Run("TrashIsSkipped", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		nb := 15
		for i := 0; i < nb; i++ {
			name := "foo" + strconv.Itoa(i)

			e.POST("/files/").
				WithQuery("Name", name).
				WithQuery("Type", "file").
				WithQuery("CreatedAt", "2016-09-18T10:24:53Z").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Date", "Mon, 19 Sep 2016 12:38:04 GMT").
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
				WithBytes([]byte("foo")).
				Expect().Status(201)
		}

		ids := []string{}

		// Get the first page
		obj := e.GET("/files/io.cozy.files.root-dir").
			WithQuery("page[limit]", "5").
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.data.relationships.contents.data").Array().Length().Equal(5)
		obj.Value("included").Array().Length().Equal(5)

		for i, ref := range obj.Path("$.data.relationships.contents.data").Array().Iter() {
			id := obj.Value("included").Array().Element(i).Object().Value("id").String().Raw()
			ref.Object().Value("id").Equal(id).NotEqual(consts.TrashDirID)

			for _, seen := range ids {
				assert.NotEqual(t, id, seen)
			}

			ids = append(ids, id)
		}

		nextURL, err := url.Parse(obj.Path("$.links.next").String().NotEmpty().Raw())
		require.NoError(t, err)

		// Get the second page
		obj = e.GET(nextURL.Path).
			WithQueryString(nextURL.RawQuery).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.data.relationships.contents.data").Array().Length().Equal(5)
		obj.Value("included").Array().Length().Equal(5)

		for i, ref := range obj.Path("$.data.relationships.contents.data").Array().Iter() {
			id := obj.Value("included").Array().Element(i).Object().Value("id").String().Raw()
			ref.Object().Value("id").Equal(id).NotEqual(consts.TrashDirID)

			for _, seen := range ids {
				assert.NotEqual(t, id, seen)
			}

			ids = append(ids, id)
		}

		nextURL, err = url.Parse(obj.Path("$.links.next").String().NotEmpty().Raw())
		require.NoError(t, err)

		// Get the third page and skip 10 elements
		obj = e.GET(nextURL.Path).
			WithQueryString(nextURL.RawQuery).
			WithQuery("page[skip]", "10").
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.data.relationships.contents.data").Array().Length().Equal(5)
		obj.Value("included").Array().Length().Equal(5)

		for i, ref := range obj.Path("$.data.relationships.contents.data").Array().Iter() {
			id := obj.Value("included").Array().Element(i).Object().Value("id").String().Raw()
			ref.Object().Value("id").Equal(id).NotEqual(consts.TrashDirID)

			for _, seen := range ids {
				assert.NotEqual(t, id, seen)
			}

			ids = append(ids, id)
		}
	})

	t.Run("ZeroCountIsPresent", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		parentID := e.POST("/files/").
			WithQuery("Name", "emptydirectory").
			WithQuery("Type", "directory").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		obj := e.GET("/files/"+parentID).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.data.relationships.contents.meta.count").Equal(0)
	})

	t.Run("ListDirPaginated", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		parentID := e.POST("/files/").
			WithQuery("Name", "paginationcontainer").
			WithQuery("Type", "directory").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		nb := 15
		for i := 0; i < nb; i++ {
			name := "file" + strconv.Itoa(i)

			e.POST("/files/"+parentID).
				WithQuery("Name", name).
				WithQuery("Type", "file").
				WithQuery("CreatedAt", "2016-09-18T10:24:53Z").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Date", "Mon, 19 Sep 2016 12:38:04 GMT").
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
				WithBytes([]byte("foo")).
				Expect().Status(201)
		}

		obj := e.GET("/files/"+parentID).
			WithQuery("page[limit]", "7").
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data1 := obj.Path("$.data.relationships.contents.data").Array()
		data1.Length().Equal(7)
		obj.Value("included").Array().Length().Equal(7)

		for i, ref := range obj.Path("$.data.relationships.contents.data").Array().Iter() {
			id := obj.Value("included").Array().Element(i).Object().Value("id").String().Raw()
			ref.Object().Value("id").Equal(id).NotEqual(consts.TrashDirID)
		}

		obj.Path("$.data.relationships.contents.meta.count").Equal(15)

		nextURL, err := url.Parse(obj.Path("$.data.relationships.contents.links.next").String().NotEmpty().Raw())
		require.NoError(t, err)

		// Get the second page
		obj = e.GET(nextURL.Path).
			WithQueryString(nextURL.RawQuery).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data2 := obj.Value("data").Array()
		data2.Length().Equal(7)
		obj.Path("$.meta.count").Equal(15)

		data2.Element(0).Object().Value("id").
			NotEqual(data1.Element(0).Object().Value("id").String().Raw())

		nextURL, err = url.Parse(obj.Path("$.links.next").String().NotEmpty().Raw())
		require.NoError(t, err)

		// Get the third page
		obj = e.GET(nextURL.Path).
			WithQueryString(nextURL.RawQuery).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data3 := obj.Value("data").Array()
		data3.Length().Equal(1)
		obj.Path("$.meta.count").Equal(15)

		data3.Element(0).Object().Value("id").
			NotEqual(data1.Element(0).Object().Value("id").String().Raw())

			// Trash the dir
		e.DELETE("/files/"+parentID).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200)
	})

	t.Run("ListDirPaginatedSkip", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		parentID := e.POST("/files/").
			WithQuery("Name", "paginationcontainerskip").
			WithQuery("Type", "directory").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().Path("$.data.id").String().NotEmpty().Raw()

		nb := 15
		for i := 0; i < nb; i++ {
			name := "file" + strconv.Itoa(i)

			e.POST("/files/"+parentID).
				WithQuery("Name", name).
				WithQuery("Type", "file").
				WithQuery("CreatedAt", "2016-09-18T10:24:53Z").
				WithHeader("Content-Type", "text/plain").
				WithHeader("Date", "Mon, 19 Sep 2016 12:38:04 GMT").
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-MD5", "rL0Y20zC+Fzt72VPzMSk2A==").
				WithBytes([]byte("foo")).
				Expect().Status(201)
		}

		obj := e.GET("/files/"+parentID).
			WithQuery("page[limit]", "7").
			WithQuery("page[skip]", "0").
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data1 := obj.Path("$.data.relationships.contents.data").Array()
		data1.Length().Equal(7)
		obj.Value("included").Array().Length().Equal(7)
		obj.Path("$.data.relationships.contents.meta.count").Equal(15)

		rawNext := obj.Path("$.data.relationships.contents.links.next").String().NotEmpty().Raw()
		assert.Contains(t, rawNext, "skip")

		nextURL, err := url.Parse(rawNext)
		require.NoError(t, err)

		// Get the second page
		obj = e.GET(nextURL.Path).
			WithQueryString(nextURL.RawQuery).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data2 := obj.Value("data").Array()
		data2.Length().Equal(7)
		obj.Path("$.meta.count").Equal(15)

		data2.Element(0).Object().Value("id").
			NotEqual(data1.Element(0).Object().Value("id").String().Raw())

		nextURL, err = url.Parse(obj.Path("$.links.next").String().NotEmpty().Raw())
		require.NoError(t, err)

		// Get the third page
		obj = e.GET(nextURL.Path).
			WithQueryString(nextURL.RawQuery).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data3 := obj.Value("data").Array()
		data3.Length().Equal(1)
		obj.Path("$.meta.count").Equal(15)

		data2.Element(0).Object().Value("id").
			NotEqual(data1.Element(0).Object().Value("id").String().Raw())

			// Trash the dir
		e.DELETE("/files/"+parentID).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200)
	})
}
