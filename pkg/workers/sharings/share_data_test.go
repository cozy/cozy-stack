package sharings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"net/url"

	"reflect"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

var testDocType = "io.cozy.tests"
var testDocID = "aydiayda"

var in *instance.Instance
var testInstance *instance.Instance
var domainSharer = "domain.sharer"
var setup *testutils.TestSetup
var ts *httptest.Server

func createInstance(domain, publicName string) (*instance.Instance, error) {
	var settings couchdb.JSONDoc
	settings.M = make(map[string]interface{})
	settings.M["public_name"] = publicName
	opts := &instance.Options{
		Domain:   domain,
		Settings: settings,
	}
	return instance.Create(opts)
}

func createFile(t *testing.T, fs vfs.VFS, name, content string, refs []couchdb.DocReference) *vfs.FileDoc {
	doc, err := vfs.NewFileDoc(name, "", -1, nil, "foo/bar", "foo", time.Now(), false, false, []string{"this", "is", "spartest"})
	assert.NoError(t, err)

	doc.ReferencedBy = refs

	body := bytes.NewReader([]byte(content))

	file, err := fs.CreateFile(doc, nil)
	assert.NoError(t, err)

	n, err := io.Copy(file, body)
	assert.NoError(t, err)
	assert.Equal(t, len(content), int(n))

	err = file.Close()
	assert.NoError(t, err)

	return doc
}

func createDir(t *testing.T, fs vfs.VFS, name string) *vfs.DirDoc {
	dirDoc, err := vfs.NewDirDoc(fs, name, "", []string{"It's", "me", "again"})
	assert.NoError(t, err)
	dirDoc.CreatedAt = time.Now()
	dirDoc.UpdatedAt = time.Now()
	err = fs.CreateDir(dirDoc)
	assert.NoError(t, err)

	return dirDoc
}

func TestSendDataMissingDocType(t *testing.T) {
	docType := "fakedoctype"
	err := couchdb.ResetDB(in, docType)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	msg, err := jobs.NewMessage(jobs.JSONEncoding, SendOptions{
		DocID:      "fakeid",
		DocType:    docType,
		Recipients: []*sharings.RecipientInfo{},
	})
	assert.NoError(t, err)

	err = SendData(jobs.NewWorkerContext(domainSharer, "123"), msg)
	assert.Error(t, err)
	assert.Equal(t, "CouchDB(not_found): missing", err.Error())
}

func TestSendDataBadID(t *testing.T) {

	doc := &couchdb.JSONDoc{
		Type: testDocType,
		M:    map[string]interface{}{"test": "tests"},
	}
	doc.SetID(testDocID)
	err := couchdb.CreateNamedDocWithDB(in, doc)
	assert.NoError(t, err)
	defer func() {
		couchdb.DeleteDoc(in, doc)
	}()

	msg, err := jobs.NewMessage(jobs.JSONEncoding, SendOptions{
		DocID:      "fakeid",
		DocType:    testDocType,
		Recipients: []*sharings.RecipientInfo{},
	})
	assert.NoError(t, err)

	err = SendData(jobs.NewWorkerContext(domainSharer, "123"), msg)
	assert.Error(t, err)
	assert.Equal(t, "CouchDB(not_found): missing", err.Error())
}

func TestSendDataBadRecipient(t *testing.T) {

	doc := &couchdb.JSONDoc{
		Type: testDocType,
		M:    map[string]interface{}{"test": "tests"},
	}
	doc.SetID(testDocID)
	err := couchdb.CreateNamedDocWithDB(in, doc)
	assert.NoError(t, err)
	defer func() {
		couchdb.DeleteDoc(in, doc)
	}()

	rec := &sharings.RecipientInfo{
		URL:         "nowhere",
		AccessToken: auth.AccessToken{AccessToken: "inthesky"},
	}

	msg, err := jobs.NewMessage(jobs.JSONEncoding, SendOptions{
		DocID:      testDocID,
		DocType:    testDocType,
		Recipients: []*sharings.RecipientInfo{rec},
	})
	assert.NoError(t, err)

	err = SendData(jobs.NewWorkerContext(domainSharer, "123"), msg)
	assert.NoError(t, err)
}

func TestDeleteDoc(t *testing.T) {
	randomrev := "randomrev"

	mpr := map[string]func(*echo.Group){
		"/sharings": func(router *echo.Group) {
			router.DELETE("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, randomrev, c.QueryParam(consts.QueryParamRev))
				assert.Equal(t, testDocID, c.Param("docid"))
				assert.Equal(t, testDocType, c.Param("doctype"))
				return c.JSON(http.StatusOK, nil)
			})
		},
		"/data": func(router *echo.Group) {
			router.GET("/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, testDocID, c.Param("docid"))
				assert.Equal(t, testDocType, c.Param("doctype"))
				doc := &couchdb.JSONDoc{
					Type: testDocType,
					M: map[string]interface{}{
						"_rev": randomrev,
					},
				}
				return c.JSON(http.StatusOK, doc.ToMapWithType())
			})
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)

	tsURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	opts := &SendOptions{
		DocID:   testDocID,
		DocType: testDocType,
		Path:    fmt.Sprintf("/sharings/doc/%s/%s", testDocType, testDocID),
		Recipients: []*sharings.RecipientInfo{
			&sharings.RecipientInfo{
				URL:         tsURL.Host,
				AccessToken: auth.AccessToken{AccessToken: "inthesky"},
			},
		},
	}

	err = DeleteDoc(in, opts)
	assert.NoError(t, err)
}

func TestSendFile(t *testing.T) {
	fs := testInstance.VFS()
	fileDoc := createFile(t, fs, "filetestsend", "Hello, it's me again.",
		[]couchdb.DocReference{})

	mpr := map[string]func(*echo.Group){
		"/files": func(router *echo.Group) {
			router.HEAD("/download/:file-id", func(c echo.Context) error {
				assert.Equal(t, fileDoc.ID(), c.Param("file-id"))
				return c.JSON(http.StatusForbidden, nil)
			})
		},
		"/sharings": func(router *echo.Group) {
			router.POST("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, fileDoc.DocType(), c.Param("doctype"))
				assert.Equal(t, fileDoc.ID(), c.Param("docid"))
				assert.Equal(t, consts.FileType, c.QueryParam("Type"))
				assert.Equal(t, fileDoc.DocName, c.QueryParam("Name"))
				sentFileDoc, err := files.FileDocFromReq(c, fileDoc.DocName,
					"", nil)
				assert.NoError(t, err)
				assert.Equal(t, fileDoc.MD5Sum, sentFileDoc.MD5Sum)
				return c.JSON(http.StatusOK, nil)
			})
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)
	tsURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	recipient := &sharings.RecipientInfo{
		URL:         tsURL.Host,
		AccessToken: auth.AccessToken{AccessToken: "inthesky"},
	}
	recipients := []*sharings.RecipientInfo{recipient}

	sendFileOpts := &SendOptions{
		DocID:   fileDoc.ID(),
		DocType: fileDoc.DocType(),
		Type:    consts.FileType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
			fileDoc.ID()),
		Recipients: recipients,
	}

	err = SendFile(testInstance, sendFileOpts, fileDoc)
	assert.NoError(t, err)
}

func TestSendFileAbort(t *testing.T) {
	fs := testInstance.VFS()
	fileDoc := createFile(t, fs, "filetestsendabort", "Hello, it's me again.",
		[]couchdb.DocReference{})

	mpr := map[string]func(*echo.Group){
		"/files": func(router *echo.Group) {
			router.HEAD("/:file-id", func(c echo.Context) error {
				assert.Equal(t, fileDoc.ID(), c.Param("file-id"))
				return c.JSON(http.StatusOK, nil)
			})
		},
		"/sharings": func(router *echo.Group) {
			router.POST("/doc/:doctype/:docid", func(c echo.Context) error {
				// As we are testing the fact that the function aborts if the
				// file already exists we should never reach this part of the
				// code.
				t.FailNow()
				return c.JSON(http.StatusOK, nil)
			})
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)
	tsURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	recipient := &sharings.RecipientInfo{
		URL:         tsURL.Host,
		AccessToken: auth.AccessToken{AccessToken: "inthesky"},
	}
	recipients := []*sharings.RecipientInfo{recipient}

	sendFileOpts := &SendOptions{
		DocID:   fileDoc.ID(),
		DocType: fileDoc.DocType(),
		Type:    consts.FileType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
			fileDoc.ID()),
		Recipients: recipients,
	}

	err = SendFile(testInstance, sendFileOpts, fileDoc)
	assert.NoError(t, err)
}

func TestSendFileThroughUpdateOrPatchFile(t *testing.T) {
	// A file can be sent if a GET on the recipient side results in a
	// "Not Found" error. This is what this test is about.

	fs := testInstance.VFS()
	fileDoc := createFile(t, fs, "filetestsendthroughupdateorpatchfile",
		"Hello, it's me again.", []couchdb.DocReference{})

	mpr := map[string]func(*echo.Group){
		"/files": func(router *echo.Group) {
			router.GET("/:file-id", func(c echo.Context) error {
				assert.Equal(t, fileDoc.ID(), c.Param("file-id"))
				return c.JSON(http.StatusNotFound, map[string]string{
					"status": "404",
					"title":  "Not Found",
					"detail": "sorry don't want to look",
				})
			})
		},
		"/sharings": func(router *echo.Group) {
			router.POST("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, fileDoc.DocType(), c.Param("doctype"))
				assert.Equal(t, fileDoc.ID(), c.Param("docid"))
				assert.Equal(t, consts.FileType, c.QueryParam("Type"))
				assert.Equal(t, fileDoc.DocName, c.QueryParam("Name"))
				sentFileDoc, err := files.FileDocFromReq(c, fileDoc.DocName,
					"", nil)
				assert.NoError(t, err)
				assert.Equal(t, fileDoc.MD5Sum, sentFileDoc.MD5Sum)
				return c.JSON(http.StatusOK, nil)
			})
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)
	tsURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	recipient := &sharings.RecipientInfo{
		URL:         tsURL.Host,
		AccessToken: auth.AccessToken{AccessToken: "inthesky"},
	}
	recipients := []*sharings.RecipientInfo{recipient}

	updateFileOpts := &SendOptions{
		DocID:   fileDoc.ID(),
		DocType: fileDoc.DocType(),
		Type:    consts.FileType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
			fileDoc.ID()),
		Recipients: recipients,
	}
	err = UpdateOrPatchFile(testInstance, updateFileOpts, fileDoc, true)
	assert.NoError(t, err)
}

func TestSendDir(t *testing.T) {
	fs := testInstance.VFS()
	dirDoc := createDir(t, fs, "testsenddir")
	dirDoc.ReferencedBy = []couchdb.DocReference{
		couchdb.DocReference{ID: "123", Type: "first"},
	}

	mpr := map[string]func(*echo.Group){
		"/sharings": func(router *echo.Group) {
			router.POST("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, dirDoc.DocType(), c.Param("doctype"))
				assert.Equal(t, dirDoc.ID(), c.Param("docid"))
				assert.Equal(t, consts.DirType, c.QueryParam("Type"))
				assert.Equal(t, dirDoc.DocName, c.QueryParam("Name"))
				dirTags := strings.Join(dirDoc.Tags, files.TagSeparator)
				assert.Equal(t, dirTags, c.QueryParam("Tags"))
				assert.Equal(t, dirDoc.CreatedAt.Format(time.RFC1123),
					c.QueryParam("Created_at"))
				assert.Equal(t, dirDoc.UpdatedAt.Format(time.RFC1123),
					c.QueryParam("Updated_at"))
				return c.JSON(http.StatusOK, nil)
			})
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)
	tsURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	recipient := &sharings.RecipientInfo{
		URL:         tsURL.Host,
		AccessToken: auth.AccessToken{AccessToken: "inthesky"},
	}
	recipients := []*sharings.RecipientInfo{recipient}

	sendDirOpts := &SendOptions{
		DocID:    dirDoc.ID(),
		DocType:  dirDoc.DocType(),
		Type:     consts.FileType,
		Selector: consts.SelectorReferencedBy,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", dirDoc.DocType(),
			dirDoc.ID()),
		Recipients: recipients,
	}

	err = SendDir(testInstance, sendDirOpts, dirDoc)
	assert.NoError(t, err)
}

func TestUpdateOrPatchFile(t *testing.T) {
	fs := testInstance.VFS()
	fileDoc := createFile(t, fs, "original", "original file",
		[]couchdb.DocReference{})
	fileDoc.ReferencedBy = []couchdb.DocReference{
		couchdb.DocReference{Type: "random", ID: "123"},
	}
	patchedFileDoc := &vfs.FileDoc{
		DocName:   "patchedName",
		DirID:     fileDoc.DirID,
		Tags:      fileDoc.Tags,
		UpdatedAt: fileDoc.UpdatedAt,
		DocID:     fileDoc.ID(),
		DocRev:    fileDoc.Rev(),
		MD5Sum:    fileDoc.MD5Sum,
	}
	referenceUpdateFileDoc := &vfs.FileDoc{
		DocName:   fileDoc.DocName,
		DirID:     fileDoc.DirID,
		Tags:      fileDoc.Tags,
		UpdatedAt: fileDoc.UpdatedAt,
		DocID:     fileDoc.ID(),
		DocRev:    fileDoc.Rev(),
		MD5Sum:    fileDoc.MD5Sum,
	}
	newReference := couchdb.DocReference{Type: "new", ID: "reference"}
	referenceUpdateFileDoc.ReferencedBy = []couchdb.DocReference{newReference}
	updatedFileDoc := createFile(t, fs, "update", "this is an update",
		[]couchdb.DocReference{})
	updatedFileDoc.ReferencedBy = []couchdb.DocReference{newReference}

	mpr := map[string]func(*echo.Group){
		"/files": func(router *echo.Group) {
			router.GET("/:file-id", func(c echo.Context) error {
				assert.NotEmpty(t, c.Param("file-id"))
				return jsonapi.Data(c, http.StatusOK, &file{fileDoc}, nil)
			})
			router.POST("/:file-id/relationships/referenced_by",
				func(c echo.Context) error {
					assert.Equal(t, fileDoc.ID(), c.Param("file-id"))
					refsReq, err := jsonapi.BindRelations(c.Request())
					assert.NoError(t, err)
					assert.True(t, reflect.DeepEqual(refsReq,
						referenceUpdateFileDoc.ReferencedBy))
					return c.JSON(http.StatusOK, nil)
				},
			)
		},
		"/sharings": func(router *echo.Group) {
			router.PATCH("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, fileDoc.ID(), c.Param("docid"))
				assert.Equal(t, fileDoc.Rev(),
					c.QueryParam(consts.QueryParamRev))
				assert.Equal(t, consts.FileType, c.QueryParam("Type"))

				var patch vfs.DocPatch
				_, err := jsonapi.Bind(c.Request(), &patch)
				assert.NoError(t, err)
				assert.Equal(t, patchedFileDoc.DocName, *patch.Name)
				assert.Equal(t, "", *patch.DirID)
				assert.Equal(t, fileDoc.Tags, *patch.Tags)
				assert.Equal(t, fileDoc.UpdatedAt.Unix(),
					(*patch.UpdatedAt).Unix())
				return c.JSON(http.StatusOK, nil)
			})
			router.PUT("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, updatedFileDoc.ID(), c.Param("docid"))
				assert.Equal(t, updatedFileDoc.DocType(), c.Param("doctype"))
				// We parametered the route to always return the rev of fileDoc
				assert.Equal(t, fileDoc.Rev(), c.QueryParam("rev"))
				assert.Equal(t, consts.FileType, c.QueryParam("Type"))
				assert.Equal(t, updatedFileDoc.DocName, c.QueryParam("Name"))
				assert.Equal(t, "false", c.QueryParam("Executable"))
				assert.Equal(t, updatedFileDoc.UpdatedAt.Format(time.RFC1123),
					c.QueryParam("Updated_at"))
				sentFileDoc, err := files.FileDocFromReq(c,
					updatedFileDoc.DocName, "", nil)
				assert.NoError(t, err)
				assert.Equal(t, updatedFileDoc.MD5Sum, sentFileDoc.MD5Sum)
				return c.JSON(http.StatusOK, nil)
			})
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)
	tsURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	testRecipient := &sharings.RecipientInfo{
		URL:         tsURL.Host,
		AccessToken: auth.AccessToken{AccessToken: "inthesky"},
	}
	recipients := []*sharings.RecipientInfo{testRecipient}

	// Patch test: we trigger a patch by providing a different DocName through
	// patchedFileDoc.
	patchSendOptions := &SendOptions{
		DocID:   fileDoc.ID(),
		DocType: fileDoc.DocType(),
		Type:    consts.FileType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
			fileDoc.ID()),
		Recipients: recipients,
	}
	err = UpdateOrPatchFile(testInstance, patchSendOptions, patchedFileDoc,
		true)
	assert.NoError(t, err)

	// Reference test: we trigger an update of references by providing a
	// "missing" reference through the selector/values in the SendOptions and in
	// the ReferencedBy field of referenceUpdateFileDoc.
	referenceSendOptions := &SendOptions{
		DocID:   fileDoc.ID(),
		DocType: fileDoc.DocType(),
		Type:    consts.FileType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
			fileDoc.ID()),
		Recipients: recipients,
		Selector:   consts.SelectorReferencedBy,
		Values:     []string{"new/reference"},
	}
	err = UpdateOrPatchFile(testInstance, referenceSendOptions,
		referenceUpdateFileDoc, true)
	assert.NoError(t, err)

	// Update test: we trigger a content update by providing a file with a
	// different MD5Sum, updatedFileDoc.
	// We still set Selector/Values so as to test the sharedRefs part in the
	// method `fillDetailsAndOpenFile`.
	updateSendOptions := &SendOptions{
		DocID:   updatedFileDoc.ID(),
		DocType: updatedFileDoc.DocType(),
		Type:    consts.FileType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", updatedFileDoc.DocType(),
			updatedFileDoc.ID()),
		Recipients: recipients,
		Selector:   consts.SelectorReferencedBy,
		Values:     []string{"new/reference"},
	}
	err = UpdateOrPatchFile(testInstance, updateSendOptions, updatedFileDoc,
		true)
	assert.NoError(t, err)
}

func TestPatchDir(t *testing.T) {
	fs := testInstance.VFS()
	dirDoc := createDir(t, fs, "testpatchdir")

	mpr := map[string]func(*echo.Group){
		"/files": func(router *echo.Group) {
			router.GET("/:file-id", func(c echo.Context) error {
				assert.Equal(t, dirDoc.ID(), c.Param("file-id"))
				return jsonapi.Data(c, http.StatusOK, &dir{dirDoc}, nil)
			})
		},
		"/sharings": func(router *echo.Group) {
			router.PATCH("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, dirDoc.ID(), c.Param("docid"))
				assert.Equal(t, dirDoc.Rev(),
					c.QueryParam(consts.QueryParamRev))
				assert.Equal(t, consts.DirType, c.QueryParam("Type"))

				var patch vfs.DocPatch
				_, err := jsonapi.Bind(c.Request(), &patch)
				assert.NoError(t, err)
				assert.Equal(t, dirDoc.DocName, *patch.Name)
				assert.Equal(t, dirDoc.DirID, *patch.DirID)
				assert.Equal(t, dirDoc.Tags, *patch.Tags)
				assert.Equal(t, dirDoc.UpdatedAt.Unix(),
					(*patch.UpdatedAt).Unix())
				return c.JSON(http.StatusOK, nil)
			})
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)
	tsURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	testRecipient := &sharings.RecipientInfo{
		URL:         tsURL.Host,
		AccessToken: auth.AccessToken{AccessToken: "inthesky"},
	}
	recipients := []*sharings.RecipientInfo{testRecipient}

	patchSendOptions := &SendOptions{
		DocID:   dirDoc.ID(),
		DocType: dirDoc.DocType(),
		Type:    consts.DirType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", dirDoc.DocType(),
			dirDoc.ID()),
		Recipients: recipients,
	}

	err = PatchDir(in, patchSendOptions, dirDoc)
	assert.NoError(t, err)
}

func TestRemoveDirOrFileFromSharing(t *testing.T) {
	fileDoc := createFile(t, testInstance.VFS(), "removeFileFromSharing",
		"removeFileFromSharingContent", []couchdb.DocReference{
			couchdb.DocReference{Type: "third", ID: "789"},
		})
	dirDoc := createDir(t, testInstance.VFS(), "removeDirFromSharing")
	dirToKeep := createDir(t, testInstance.VFS(),
		"removeDirFromSharingButKeepIt")

	rule := permissions.Rule{
		Selector: consts.SelectorReferencedBy,
		Type:     consts.Files,
		Values:   []string{"third/789"},
	}
	createSharing(t, consts.MasterMasterSharing, false, []*sharings.Recipient{}, rule)

	refs := []couchdb.DocReference{
		couchdb.DocReference{
			Type: "first",
			ID:   "123",
		},
		couchdb.DocReference{
			Type: "second",
			ID:   "456",
		},
	}

	mpr := map[string]func(*echo.Group){
		"/sharings": func(router *echo.Group) {
			router.DELETE("/files/:file-id/referenced_by",
				func(c echo.Context) error {
					fileid := c.Param("file-id")
					assert.True(t, fileid == fileDoc.ID() ||
						fileid == dirDoc.ID() || fileid == dirToKeep.ID())
					reqRefs, err := jsonapi.BindRelations(c.Request())
					assert.NoError(t, err)
					assert.True(t, reflect.DeepEqual(refs, reqRefs))
					return c.JSON(http.StatusOK, nil)
				},
			)
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)
	tsURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	testRecipient := &sharings.RecipientInfo{
		URL:         tsURL.Host,
		AccessToken: auth.AccessToken{AccessToken: "inthesky"},
	}
	recipients := []*sharings.RecipientInfo{testRecipient}

	// Test: we simulate a recipient asking the sharer to remove a file from
	// a sharing while the file is still shared in the sharing we created. It
	// should not be removed.
	optsFile := SendOptions{
		Selector:   consts.SelectorReferencedBy,
		Values:     []string{"first/123", "second/456"},
		DocID:      fileDoc.ID(),
		DocType:    consts.Files,
		Recipients: recipients,
	}

	err = RemoveDirOrFileFromSharing(testInstance, &optsFile, true)
	assert.NoError(t, err)
	fileDoc, err = testInstance.VFS().FileByID(fileDoc.ID())
	assert.NoError(t, err)

	optsDir := SendOptions{
		Selector:   consts.SelectorReferencedBy,
		Values:     []string{"first/123", "second/456"},
		DocID:      dirDoc.ID(),
		DocType:    consts.Files,
		Recipients: recipients,
	}

	err = RemoveDirOrFileFromSharing(testInstance, &optsDir, true)
	assert.NoError(t, err)
	dirDoc, err = testInstance.VFS().DirByID(dirDoc.ID())
	assert.NoError(t, err)
	assert.True(t, dirDoc.DirID == consts.TrashDirID)

	optsDirToKeep := SendOptions{
		Selector:   consts.SelectorReferencedBy,
		Values:     []string{"first/123", "second/456"},
		DocID:      dirToKeep.ID(),
		DocType:    consts.Files,
		Recipients: recipients,
	}

	err = RemoveDirOrFileFromSharing(testInstance, &optsDirToKeep, false)
	assert.NoError(t, err)
	dirToKeep, err = testInstance.VFS().DirByID(dirToKeep.ID())
	assert.NoError(t, err)
	assert.True(t, dirToKeep.DirID != consts.TrashDirID)
}

func TestDeleteDirOrFile(t *testing.T) {
	fs := testInstance.VFS()
	dirDoc := createDir(t, fs, "testdeletedir")

	mpr := map[string]func(*echo.Group){
		"/files": func(router *echo.Group) {
			router.GET("/:file-id", func(c echo.Context) error {
				assert.Equal(t, dirDoc.ID(), c.Param("file-id"))
				return jsonapi.Data(c, http.StatusOK, &dir{dirDoc}, nil)
			})
		},
		"/sharings": func(router *echo.Group) {
			router.DELETE("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, dirDoc.DocType(), c.Param("doctype"))
				assert.Equal(t, dirDoc.ID(), c.Param("docid"))
				assert.Equal(t, dirDoc.Rev(),
					c.QueryParam(consts.QueryParamRev))
				return c.JSON(http.StatusOK, nil)
			})
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)
	tsURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	testRecipient := &sharings.RecipientInfo{
		URL:         tsURL.Host,
		AccessToken: auth.AccessToken{AccessToken: "inthesky"},
	}
	recipients := []*sharings.RecipientInfo{testRecipient}

	deleteSendOptions := &SendOptions{
		DocID:   dirDoc.ID(),
		DocType: dirDoc.DocType(),
		Type:    consts.DirType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", dirDoc.DocType(),
			dirDoc.ID()),
		Recipients: recipients,
	}

	err = DeleteDirOrFile(in, deleteSendOptions, false)
	assert.NoError(t, err)
}

func TestFileHasChangesSameFilesSuccess(t *testing.T) {
	newFileDoc := &vfs.FileDoc{
		DocName: "samename",
	}
	remoteFileDoc := &vfs.FileDoc{
		DocName: "samename",
	}
	opts := &SendOptions{
		Values: []string{"123"},
	}
	hasChange := fileHasChanges(in.VFS(), opts, newFileDoc, remoteFileDoc)
	assert.Equal(t, false, hasChange)
}

func TestFileHasChangesNameSuccess(t *testing.T) {
	newFileDoc := &vfs.FileDoc{
		DocName: "newname",
	}
	remoteFileDoc := &vfs.FileDoc{
		DocName: "oldname",
	}
	opts := &SendOptions{
		Values: []string{"123"},
	}
	hasChange := fileHasChanges(in.VFS(), opts, newFileDoc, remoteFileDoc)
	assert.Equal(t, true, hasChange)
}

func TestFileHasChangesTagSuccess(t *testing.T) {
	newFileDoc := &vfs.FileDoc{
		DocName: "samename",
	}
	remoteFileDoc := &vfs.FileDoc{
		DocName: "samename",
		Tags:    []string{"pimpmytag"},
	}
	opts := &SendOptions{
		Values: []string{"123"},
	}
	hasChange := fileHasChanges(in.VFS(), opts, newFileDoc, remoteFileDoc)
	assert.Equal(t, true, hasChange)
}

func TestDocHasChangesSameDocsSuccess(t *testing.T) {
	newDoc := &couchdb.JSONDoc{
		M: make(map[string]interface{}),
	}
	remoteDoc := &couchdb.JSONDoc{
		M: make(map[string]interface{}),
	}
	newDoc.M["_id"] = "sharedid"
	newDoc.M["_rev"] = "localrev"
	remoteDoc.M["_id"] = "sharedid"
	remoteDoc.M["_rev"] = "remoterev"
	newDoc.M["samefield"] = "thesame"
	remoteDoc.M["samefield"] = "thesame"

	equal := docHasChanges(newDoc, remoteDoc)
	assert.Equal(t, false, equal)
}

func TestDocHasChangesNotSameDocsSuccess(t *testing.T) {
	newDoc := &couchdb.JSONDoc{
		M: make(map[string]interface{}),
	}
	remoteDoc := &couchdb.JSONDoc{
		M: make(map[string]interface{}),
	}
	newDoc.M["_id"] = "sharedid"
	newDoc.M["_rev"] = "localrev"
	remoteDoc.M["_id"] = "sharedid"
	remoteDoc.M["_rev"] = "remoterev"
	newDoc.M["samefield"] = "thesame"
	remoteDoc.M["samefield"] = "notthesame"

	equal := docHasChanges(newDoc, remoteDoc)
	assert.Equal(t, true, equal)
}

func TestFindNewRefsSameRefSuccess(t *testing.T) {
	sharedRef := couchdb.DocReference{Type: "reftype", ID: "refid"}
	localRefs := []couchdb.DocReference{sharedRef}
	remoteRefs := []couchdb.DocReference{sharedRef}
	fileDoc := &vfs.FileDoc{
		ReferencedBy: localRefs,
	}
	remoteFileDoc := &vfs.FileDoc{
		ReferencedBy: remoteRefs,
	}
	opts := &SendOptions{
		Values: []string{"reftype/refid"},
	}
	refs := findNewRefs(opts, fileDoc, remoteFileDoc)
	var expected []couchdb.DocReference
	assert.Equal(t, expected, refs)
}

func TestExtractRelevantReferences(t *testing.T) {
	fileDoc := createFile(t, testInstance.VFS(), "extractFileRef",
		"testExtractFileRef", []couchdb.DocReference{})
	refs := []couchdb.DocReference{
		couchdb.DocReference{
			ID:   "123",
			Type: "first",
		},
		couchdb.DocReference{
			ID:   "456",
			Type: "second",
		},
		couchdb.DocReference{
			ID:   "789",
			Type: "third",
		},
	}
	fileDoc.ReferencedBy = refs

	optsBad := SendOptions{
		Selector: consts.SelectorReferencedBy,
		Values:   []string{"123", "789"},
	}
	relRefs := optsBad.extractRelevantReferences(refs)
	assert.Empty(t, relRefs)

	opts := SendOptions{
		Selector: consts.SelectorReferencedBy,
		Values:   []string{"first/123", "third/789"},
	}
	relRefs = opts.extractRelevantReferences(refs)
	assert.Len(t, relRefs, 2)
}

func TestFindMissingRefs(t *testing.T) {
	lref := []couchdb.DocReference{
		couchdb.DocReference{
			Type: "first",
			ID:   "123",
		},
		couchdb.DocReference{
			Type: "second",
			ID:   "456",
		},
		couchdb.DocReference{
			Type: "third",
			ID:   "789",
		},
	}

	rref := []couchdb.DocReference{
		couchdb.DocReference{
			Type: "first",
			ID:   "123",
		},
		couchdb.DocReference{
			Type: "third",
			ID:   "789",
		},
	}

	missRefs := findMissingRefs(lref, rref)
	assert.Len(t, missRefs, 1)
	assert.True(t, reflect.DeepEqual(missRefs[0], couchdb.DocReference{
		Type: "second", ID: "456",
	}))
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()

	// Change the default config to persist the vfs
	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		fmt.Println("Could not create temporary directory.")
		os.Exit(1)
	}
	config.GetConfig().Fs.URL = &url.URL{
		Scheme: "file",
		Host:   "localhost",
		Path:   tempdir,
	}

	setup = testutils.NewSetup(m, "share_data_test")
	testInstance = setup.GetTestInstance()

	_, err = stack.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	instance.Destroy(domainSharer)
	in, err = createInstance(domainSharer, "Alice")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(in, testDocType)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(in, consts.Sharings)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.DefineIndex(in, mango.IndexOnFields(consts.Sharings, "by-sharing-id", []string{"sharing_id"}))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// The following structures were copied just so that we could call the function
// `jsonapi.Data` in the tests. This is to mimick what the stack would normally
// reply.
type file struct {
	doc *vfs.FileDoc
}

func (f *file) ID() string         { return f.doc.ID() }
func (f *file) Rev() string        { return f.doc.Rev() }
func (f *file) SetID(id string)    { f.doc.SetID(id) }
func (f *file) SetRev(rev string)  { f.doc.SetRev(rev) }
func (f *file) DocType() string    { return f.doc.DocType() }
func (f *file) Clone() couchdb.Doc { cloned := *f; return &cloned }
func (f *file) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{
		"referenced_by": jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Self: "/files/" + f.doc.ID() + "/relationships/references",
			},
			Data: f.doc.ReferencedBy,
		},
	}
}
func (f *file) Included() []jsonapi.Object { return []jsonapi.Object{} }
func (f *file) MarshalJSON() ([]byte, error) {
	ref := f.doc.ReferencedBy
	f.doc.ReferencedBy = nil
	res, err := json.Marshal(f.doc)
	f.doc.ReferencedBy = ref
	return res, err
}
func (f *file) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/files/" + f.doc.DocID}
}

type dir struct {
	doc *vfs.DirDoc
}

func (d *dir) ID() string                             { return d.doc.ID() }
func (d *dir) Rev() string                            { return d.doc.Rev() }
func (d *dir) SetID(id string)                        { d.doc.SetID(id) }
func (d *dir) SetRev(rev string)                      { d.doc.SetRev(rev) }
func (d *dir) DocType() string                        { return d.doc.DocType() }
func (d *dir) Clone() couchdb.Doc                     { cloned := *d; return &cloned }
func (d *dir) Relationships() jsonapi.RelationshipMap { return nil }
func (d *dir) Included() []jsonapi.Object             { return nil }
func (d *dir) MarshalJSON() ([]byte, error)           { return json.Marshal(d.doc) }
func (d *dir) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/files/" + d.doc.DocID}
}
