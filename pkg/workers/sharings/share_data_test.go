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

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/labstack/echo"
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

func createFile(t *testing.T, fs vfs.VFS, name, content string) *vfs.FileDoc {
	doc, err := vfs.NewFileDoc(name, "", -1, nil, "foo/bar", "foo", time.Now(), false, false, []string{"this", "is", "spartest"})
	assert.NoError(t, err)

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
		Recipients: []*RecipientInfo{},
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
		Recipients: []*RecipientInfo{},
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

	rec := &RecipientInfo{
		URL:   "nowhere",
		Token: "inthesky",
	}

	msg, err := jobs.NewMessage(jobs.JSONEncoding, SendOptions{
		DocID:      testDocID,
		DocType:    testDocType,
		Recipients: []*RecipientInfo{rec},
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
				assert.Equal(t, randomrev, c.QueryParam("rev"))
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
		Recipients: []*RecipientInfo{
			&RecipientInfo{
				URL:   tsURL.Host,
				Token: "whoneedsone?",
			},
		},
	}

	err = DeleteDoc(opts)
	assert.NoError(t, err)
}

func TestSendFile(t *testing.T) {
	fs := testInstance.VFS()
	fileDoc := createFile(t, fs, "filetestsend", "Hello, it's me again.")

	mpr := map[string]func(*echo.Group){
		"/sharings": func(router *echo.Group) {
			router.POST("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, fileDoc.DocType(), c.Param("doctype"))
				assert.Equal(t, fileDoc.ID(), c.Param("docid"))
				assert.Equal(t, consts.FileType, c.QueryParam("Type"))
				assert.Equal(t, fileDoc.DocName, c.QueryParam("Name"))
				sentFileDoc, err := files.FileDocFromReq(c, fileDoc.DocName,
					consts.SharedWithMeDirID, nil)
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

	recipient := &RecipientInfo{
		URL:   tsURL.Host,
		Token: "idontneedoneImtesting",
	}
	recipients := []*RecipientInfo{recipient}

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

func TestSendDir(t *testing.T) {
	fs := testInstance.VFS()
	dirDoc := createDir(t, fs, "testsenddir")

	mpr := map[string]func(*echo.Group){
		"/sharings": func(router *echo.Group) {
			router.POST("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, dirDoc.DocType(), c.Param("doctype"))
				assert.Equal(t, dirDoc.ID(), c.Param("docid"))
				assert.Equal(t, consts.DirType, c.QueryParam("Type"))
				assert.Equal(t, dirDoc.DocName, c.QueryParam("Name"))
				dirTags := strings.Join(dirDoc.Tags, files.TagSeparator)
				assert.Equal(t, dirTags, c.QueryParam("Tags"))
				dirDocPath, err := dirDoc.Path(fs)
				assert.NoError(t, err)
				assert.Equal(t, dirDocPath, c.QueryParam("Path"))
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

	recipient := &RecipientInfo{
		URL:   tsURL.Host,
		Token: "idontneedoneImtesting",
	}
	recipients := []*RecipientInfo{recipient}

	sendDirOpts := &SendOptions{
		DocID:   dirDoc.ID(),
		DocType: dirDoc.DocType(),
		Type:    consts.FileType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", dirDoc.DocType(),
			dirDoc.ID()),
		Recipients: recipients,
	}

	err = SendDir(testInstance, sendDirOpts, dirDoc)
	assert.NoError(t, err)
}

func TestUpdateOrPatchFile(t *testing.T) {
	fs := testInstance.VFS()
	fileDoc := createFile(t, fs, "original", "original file")
	updatedFileDoc := createFile(t, fs, "update", "this is an update")

	mpr := map[string]func(*echo.Group){
		"/files": func(router *echo.Group) {
			router.GET("/:file-id", func(c echo.Context) error {
				assert.NotEmpty(t, c.Param("file-id"))
				return jsonapi.Data(c, http.StatusOK, &file{fileDoc}, nil)
			})
		},
		"/sharings": func(router *echo.Group) {
			router.PATCH("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, fileDoc.ID(), c.Param("docid"))
				assert.Equal(t, fileDoc.Rev(), c.QueryParam("rev"))
				assert.Equal(t, consts.FileType, c.QueryParam("Type"))

				var patch vfs.DocPatch
				_, err := jsonapi.Bind(c.Request(), &patch)
				assert.NoError(t, err)
				assert.Equal(t, fileDoc.DocName, *patch.Name)
				assert.Equal(t, fileDoc.DirID, *patch.DirID)
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
				sentFileDoc, err := files.FileDocFromReq(c,
					updatedFileDoc.DocName, consts.SharedWithMeDirID, nil)
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
	testRecipient := &RecipientInfo{
		URL:   tsURL.Host,
		Token: "dontneedoneImtesting",
	}
	recipients := []*RecipientInfo{testRecipient}

	patchSendOptions := &SendOptions{
		DocID:   fileDoc.ID(),
		DocType: fileDoc.DocType(),
		Type:    consts.FileType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
			fileDoc.ID()),
		Recipients: recipients,
	}
	err = UpdateOrPatchFile(testInstance, patchSendOptions, fileDoc)
	assert.NoError(t, err)

	updateSendOptions := &SendOptions{
		DocID:   updatedFileDoc.ID(),
		DocType: updatedFileDoc.DocType(),
		Type:    consts.FileType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", updatedFileDoc.DocType(),
			updatedFileDoc.ID()),
		Recipients: recipients,
	}
	err = UpdateOrPatchFile(testInstance, updateSendOptions, updatedFileDoc)
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
				assert.Equal(t, dirDoc.Rev(), c.QueryParam("rev"))
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
	testRecipient := &RecipientInfo{
		URL:   tsURL.Host,
		Token: "dontneedoneImtesting",
	}
	recipients := []*RecipientInfo{testRecipient}

	patchSendOptions := &SendOptions{
		DocID:   dirDoc.ID(),
		DocType: dirDoc.DocType(),
		Type:    consts.DirType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", dirDoc.DocType(),
			dirDoc.ID()),
		Recipients: recipients,
	}

	err = PatchDir(patchSendOptions, dirDoc)
	assert.NoError(t, err)
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
				assert.Equal(t, dirDoc.Rev(), c.QueryParam("rev"))
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
	testRecipient := &RecipientInfo{
		URL:   tsURL.Host,
		Token: "dontneedoneImtesting",
	}
	recipients := []*RecipientInfo{testRecipient}

	deleteSendOptions := &SendOptions{
		DocID:   dirDoc.ID(),
		DocType: dirDoc.DocType(),
		Type:    consts.DirType,
		Path: fmt.Sprintf("/sharings/doc/%s/%s", dirDoc.DocType(),
			dirDoc.ID()),
		Recipients: recipients,
	}

	err = DeleteDirOrFile(deleteSendOptions)
	assert.NoError(t, err)
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
	config.GetConfig().Fs.URL = fmt.Sprintf("file://localhost%s", tempdir)

	setup = testutils.NewSetup(m, "share_data_test")
	testInstance = setup.GetTestInstance()

	err = stack.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	_, _ = instance.Destroy(domainSharer)
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
	return jsonapi.RelationshipMap{}
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
