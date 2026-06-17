package excalidraw

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestExcalidraw(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance()
	_, token := setup.GetTestClient(consts.Files)

	fileID := createExcalidrawFile(t, inst)

	ts := setup.GetTestServer("/excalidraw", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	t.Run("OpenLocal", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/excalidraw/"+fileID+"/open").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", consts.Files)
		data.ValueEqual("id", fileID)

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("instance", inst.Domain)
		attrs.Value("protocol").String().Contains("http")
		attrs.Value("subdomain").String().NotEmpty()
		attrs.Value("public_name").String().NotEmpty()
	})

	t.Run("OpenNonExcalidrawFile", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		notDrawID := createPlainFile(t, inst)
		e.GET("/excalidraw/"+notDrawID+"/open").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(404)
	})
}

func createExcalidrawFile(t *testing.T, inst *instance.Instance) string {
	filedoc, err := vfs.NewFileDoc("drawing.excalidraw", consts.RootDirID, -1, nil,
		"application/json", "application", time.Now(), false, false, false, nil)
	require.NoError(t, err)

	f, err := inst.VFS().CreateFile(filedoc, nil)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	return filedoc.ID()
}

func createPlainFile(t *testing.T, inst *instance.Instance) string {
	filedoc, err := vfs.NewFileDoc("notes.txt", consts.RootDirID, -1, nil,
		"text/plain", "text", time.Now(), false, false, false, nil)
	require.NoError(t, err)

	f, err := inst.VFS().CreateFile(filedoc, nil)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	return filedoc.ID()
}
