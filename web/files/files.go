// Package files is the HTTP frontend of the vfs package. It exposes
// an HTTP api to manipulate the filesystem and offer all the
// possibilities given by the vfs.
package files

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	pkgperm "github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/vfs"
	web_errors "github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

// TagSeparator is the character separating tags
const TagSeparator = ","

var (
	// ErrDocTypeInvalid is used when the document type sent is not recognized.
	ErrDocTypeInvalid = errors.New("Invalid document type")

	// ErrInvalidContentLength is used when the Content-Length header is not
	// valid and could not be parsed as a positive number.
	ErrInvalidContentLength = errors.New("Invalid content length")

	// ErrInvalidContentMD5 is used when the given Content-MD5 is not properly
	// encoded in base64.
	ErrInvalidContentMD5 = errors.New("Invalid MD5 checksum")
)

// readWriteTimeout is the timeout duration that we use to calculate our
// timeout window for each read/write during in our upload handler. This
// timeout bypass the global timeouts of our http.Server.
var readWriteTimeout = 15 * time.Second

// CreationHandler handle all POST requests on /files/:file-id
// aiming at creating a new document in the FS. Given the Type
// parameter of the request, it will either upload a new file or
// create a new directory.
func CreationHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	queryType := c.QueryParam("Type")
	if queryType == consts.FileType {
		if err := createFileHandler(c, instance); err != nil {
			return WrapVfsError(err)
		}
		return nil
	}
	var doc jsonapi.Object
	var err error
	if queryType == consts.DirType {
		doc, err = createDirHandler(c, instance)
	} else {
		err = ErrDocTypeInvalid
	}
	if err != nil {
		return WrapVfsError(err)
	}
	return jsonapi.Data(c, http.StatusCreated, doc, nil)
}

func createFileHandler(c echo.Context, inst *instance.Instance) error {
	fs := inst.VFS()
	tags := strings.Split(c.QueryParam("Tags"), TagSeparator)

	dirID := c.Param("file-id")
	name := c.QueryParam("Name")
	doc, err := FileDocFromReq(c, name, dirID, tags)
	if err != nil {
		return err
	}

	err = checkPerm(c, "POST", nil, doc)
	if err != nil {
		return err
	}

	dst, err := fs.CreateFile(doc, nil)
	if err != nil {
		return err
	}

	uploadFileContent(c, dst, doc.Size(), func(err error) (*file, error) {
		if cerr := dst.Close(); cerr != nil && (err == nil || err == io.ErrUnexpectedEOF) {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		return newFile(doc, inst), nil
	})
	return nil
}

func createDirHandler(c echo.Context, inst *instance.Instance) (*dir, error) {
	fs := inst.VFS()
	path := c.QueryParam("Path")
	tags := utils.SplitTrimString(c.QueryParam("Tags"), TagSeparator)

	var doc *vfs.DirDoc
	var err error
	if path != "" {
		if c.QueryParam("Recursive") == "true" {
			doc, err = vfs.MkdirAll(fs, path, tags)
		} else {
			doc, err = vfs.Mkdir(fs, path, tags)
		}
		if err != nil {
			return nil, err
		}
		return newDir(doc), nil
	}

	dirID := c.Param("file-id")
	name := c.QueryParam("Name")
	doc, err = vfs.NewDirDoc(fs, name, dirID, tags)
	if err != nil {
		return nil, err
	}
	if date := c.Request().Header.Get("Date"); date != "" {
		if t, err2 := time.Parse(time.RFC1123, date); err2 == nil {
			doc.CreatedAt = t
			doc.UpdatedAt = t
		}
	}

	err = checkPerm(c, "POST", doc, nil)
	if err != nil {
		return nil, err
	}

	if err = fs.CreateDir(doc); err != nil {
		return nil, err
	}

	return newDir(doc), nil
}

// OverwriteFileContentHandler handles PUT requests on /files/:file-id
// to overwrite the content of a file given its identifier.
func OverwriteFileContentHandler(c echo.Context) (err error) {
	inst := middlewares.GetInstance(c)
	fs := inst.VFS()
	var olddoc *vfs.FileDoc
	var newdoc *vfs.FileDoc

	fileID := c.Param("file-id")
	if fileID == "" {
		fileID = c.Param("docid") // Used by sharings.updateDocument
	}

	olddoc, err = fs.FileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	newdoc, err = FileDocFromReq(
		c,
		olddoc.DocName,
		olddoc.DirID,
		olddoc.Tags,
	)
	if err != nil {
		return WrapVfsError(err)
	}

	newdoc.ReferencedBy = olddoc.ReferencedBy
	newdoc.SetID(olddoc.ID()) // The ID can be useful to check permissions

	if err = CheckIfMatch(c, olddoc.Rev()); err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permissions.PUT, nil, olddoc)
	if err != nil {
		return
	}
	err = checkPerm(c, permissions.PUT, nil, newdoc)
	if err != nil {
		return
	}

	dst, err := fs.CreateFile(newdoc, olddoc)
	if err != nil {
		return WrapVfsError(err)
	}

	uploadFileContent(c, dst, newdoc.Size(), func(err error) (*file, error) {
		if cerr := dst.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		return newFile(newdoc, inst), nil
	})
	return nil
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 64*1024)
		return &buf
	},
}

// timeoutReadWriter is an io.ReadCloser/http.ResponseWriter allowing reading
// the body of a request and writing the body of a response with a moving
// timeout window, when the total length of the request is known in advance.
// For each read operation the read deadline is reset with the given duration,
// bypassing the global read/write timeouts for long uploads.
//
// The Close method closes the underlying connection.
//
// At the end of the read, when all bytes have been consumed, the write
// deadline is set to the same given duration.
//
// Warning: this reader hijacks the http response, hence the response .Body
// field should not be used when using this reader.
type timeoutReadWriter struct {
	r    io.Reader
	w    *bufio.Writer
	c    echo.Context
	conn net.Conn
	n    int64
	d    time.Duration

	chunked         bool
	expectsContinue bool
}

func newTimeoutReadWriter(c echo.Context, size int64, d time.Duration) (trw *timeoutReadWriter, err error) {
	h, ok := c.Response().Writer.(http.Hijacker)
	if !ok {
		return nil, errors.New("files: respose does not implement http.Hijacker")
	}

	conn, rw, err := h.Hijack()
	if err != nil {
		return
	}

	// The connection will be closed at the end of the use of this read/writer.
	c.Response().Header().Set("Connection", "close")

	expectsContinue := c.Request().Header.Get("Expect") == "100-continue"
	trw = new(timeoutReadWriter)
	trw.r = rw.Reader
	trw.w = rw.Writer
	trw.conn = conn
	trw.n = size
	trw.d = d
	trw.c = c
	trw.expectsContinue = expectsContinue

	// body request is expected to be chunked if the size is -1
	if size < 0 {
		trw.r = httputil.NewChunkedReader(trw.r)
		trw.chunked = true
	}
	return
}

func (t *timeoutReadWriter) Read(p []byte) (n int, err error) {
	if !t.chunked {
		if t.n <= 0 {
			return 0, io.EOF
		}
		if int64(len(p)) > t.n {
			p = p[0:t.n]
		}
	}
	if t.expectsContinue {
		t.expectsContinue = false
		t.w.WriteString("HTTP/1.1 100 Continue\r\n\r\n")
		t.w.Flush()
	}
	t.conn.SetReadDeadline(time.Now().Add(t.d))
	n, err = t.r.Read(p)
	if !t.chunked {
		t.n -= int64(n)
	}
	return
}

func (t *timeoutReadWriter) Write(p []byte) (n int, err error) {
	t.conn.SetWriteDeadline(time.Now().Add(t.d))
	return t.w.Write(p)
}

func (t *timeoutReadWriter) WriteError(err error) {
	web_errors.WriteError(WrapVfsError(err), t, t.c)
}

func (t *timeoutReadWriter) WriteData(status int, o jsonapi.Object, links *jsonapi.LinksList) {
	var b bytes.Buffer
	jsonapi.WriteData(&b, o, links)
	t.Header().Set("Content-Type", jsonapi.ContentType)
	t.Header().Set("Content-Length", strconv.Itoa(b.Len()))
	t.WriteHeader(status)
	t.Write(b.Bytes())
}

func (t *timeoutReadWriter) Header() http.Header {
	return t.c.Response().Header()
}

func (t *timeoutReadWriter) WriteHeader(code int) {
	// set the Committed flag to avoid any usage of the response by another
	// handler, for instance the error handler.
	t.c.Response().Committed = true
	t.c.Response().Status = code

	fmt.Fprintf(t.w, "HTTP/1.1 %03d %s\r\n", code, http.StatusText(code))
	t.Header().Write(t.w)
	fmt.Fprintf(t.w, "\r\n")
}

func (t *timeoutReadWriter) Close() error {
	t.conn.SetWriteDeadline(time.Now().Add(t.d))
	t.w.Flush()
	return t.conn.Close()
}

func uploadFileContent(c echo.Context, dst io.Writer, size int64, deferred func(error) (*file, error)) {
	trw, err := newTimeoutReadWriter(c, size, readWriteTimeout)
	defer func() {
		f, errc := deferred(err)
		if err == nil {
			if errc != nil {
				trw.WriteError(errc)
			} else {
				trw.WriteData(http.StatusCreated, f, nil)
			}
		}
	}()

	if err == nil {
		defer trw.Close()
		// TODO: we could probably reduce the number of intermediary buffers to
		// copy the request body into the VFS, maybe benefit from the underlying
		// bufio.Reader and/or bufio.Writer thanks to the WriteTo/ReadFrom methods.
		bufP := bufferPool.Get().(*[]byte)
		_, err = io.CopyBuffer(dst, trw, *bufP)
		bufferPool.Put(bufP)
	}
}

func serveFileContent(c echo.Context, fs vfs.VFS, doc *vfs.FileDoc, disposition string) {
	trw, err := newTimeoutReadWriter(c, c.Request().ContentLength, readWriteTimeout)
	if err != nil {
		return
	}

	defer trw.Close()

	err = vfs.ServeFileContent(fs, doc, disposition, c.Request(), trw)
	if err != nil {
		trw.WriteError(err)
	}
}

// ModifyMetadataByIDHandler handles PATCH requests on /files/:file-id
//
// It can be used to modify the file or directory metadata, as well as
// moving and renaming it in the filesystem.
func ModifyMetadataByIDHandler(c echo.Context) error {
	patch, err := getPatch(c)
	if err != nil {
		return WrapVfsError(err)
	}

	instance := middlewares.GetInstance(c)
	dir, file, err := instance.VFS().DirOrFileByID(c.Param("file-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	return applyPatch(c, instance, patch, dir, file)
}

// ModifyMetadataByPathHandler handles PATCH requests on /files/:file-id
//
// It can be used to modify the file or directory metadata, as well as
// moving and renaming it in the filesystem.
func ModifyMetadataByPathHandler(c echo.Context) error {
	patch, err := getPatch(c)
	if err != nil {
		return WrapVfsError(err)
	}

	instance := middlewares.GetInstance(c)
	dir, file, err := instance.VFS().DirOrFileByPath(c.QueryParam("Path"))
	if err != nil {
		return WrapVfsError(err)
	}

	return applyPatch(c, instance, patch, dir, file)
}

func getPatch(c echo.Context) (*vfs.DocPatch, error) {
	var patch vfs.DocPatch

	obj, err := jsonapi.Bind(c.Request(), &patch)
	if err != nil {
		return nil, jsonapi.BadJSON()
	}

	if rel, ok := obj.GetRelationship("parent"); ok {
		rid, ok := rel.ResourceIdentifier()
		if !ok {
			return nil, jsonapi.BadJSON()
		}
		patch.DirID = &rid.ID
	}

	patch.RestorePath = nil
	return &patch, nil
}

func applyPatch(c echo.Context, instance *instance.Instance, patch *vfs.DocPatch, dir *vfs.DirDoc, file *vfs.FileDoc) error {
	var rev string
	if dir != nil {
		rev = dir.Rev()
	} else {
		rev = file.Rev()
	}

	if err := CheckIfMatch(c, rev); err != nil {
		return WrapVfsError(err)
	}

	if err := checkPerm(c, permissions.PATCH, dir, file); err != nil {
		return err
	}

	if dir != nil {
		doc, err := vfs.ModifyDirMetadata(instance.VFS(), dir, patch)
		if err != nil {
			return WrapVfsError(err)
		}
		return dirData(c, http.StatusOK, doc)
	}

	doc, err := vfs.ModifyFileMetadata(instance.VFS(), file, patch)
	if err != nil {
		return WrapVfsError(err)
	}
	return fileData(c, http.StatusOK, doc, nil)
}

// ReadMetadataFromIDHandler handles all GET requests on /files/:file-
// id aiming at getting file metadata from its id.
func ReadMetadataFromIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	if err := checkPerm(c, permissions.GET, dir, file); err != nil {
		return err
	}

	if dir != nil {
		return dirData(c, http.StatusOK, dir)
	}
	return fileData(c, http.StatusOK, file, nil)
}

// GetChildrenHandler returns a list of children of a folder
func GetChildrenHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	if file != nil {
		return jsonapi.NewError(400, "cant read children of file "+fileID)
	}

	return dirDataList(c, http.StatusOK, dir)
}

// ReadMetadataFromPathHandler handles all GET requests on
// /files/metadata aiming at getting file metadata from its path.
func ReadMetadataFromPathHandler(c echo.Context) error {
	var err error

	instance := middlewares.GetInstance(c)

	dir, file, err := instance.VFS().DirOrFileByPath(c.QueryParam("Path"))
	if err != nil {
		return WrapVfsError(err)
	}

	if err := checkPerm(c, permissions.GET, dir, file); err != nil {
		return err
	}

	if dir != nil {
		return dirData(c, http.StatusOK, dir)
	}
	return fileData(c, http.StatusOK, file, nil)
}

// ReadFileContentFromIDHandler handles all GET requests on /files/:file-id
// aiming at downloading a file given its ID. It serves the file in inline
// mode.
func ReadFileContentFromIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	doc, err := instance.VFS().FileByID(c.Param("file-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permissions.GET, nil, doc)
	if err != nil {
		return err
	}

	disposition := "inline"
	if c.QueryParam("Dl") == "1" {
		disposition = "attachment"
	}

	serveFileContent(c, instance.VFS(), doc, disposition)
	return nil
}

// HeadDirOrFile handles HEAD requests on directory or file to check their
// existence
func HeadDirOrFile(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	dir, file, err := instance.VFS().DirOrFileByID(c.Param("file-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	if dir != nil {
		err = checkPerm(c, permissions.GET, dir, nil)
	} else {
		err = checkPerm(c, permissions.GET, nil, file)
	}
	if err != nil {
		return err
	}

	return nil
}

// ThumbnailHandler serves thumbnails of the images/photos
func ThumbnailHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	secret := c.Param("secret")
	path, err := vfs.GetStore().GetFile(instance.Domain, secret)
	if err != nil {
		return WrapVfsError(err)
	}
	if path == "" {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}

	doc, err := instance.VFS().FileByID(c.Param("file-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	expected, err := doc.Path(instance.VFS())
	if err != nil {
		return WrapVfsError(err)
	}
	if expected != path {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}

	fs := instance.ThumbsFS()
	return fs.ServeThumbContent(c.Response(), c.Request(), doc, c.Param("format"))
}

func sendFileFromPath(c echo.Context, path string, checkPermission bool) error {
	instance := middlewares.GetInstance(c)

	doc, err := instance.VFS().FileByPath(path)
	if err != nil {
		return WrapVfsError(err)
	}

	if checkPermission {
		err = permissions.Allow(c, permissions.GET, doc)
		if err != nil {
			return err
		}
	}

	disposition := "inline"
	if c.QueryParam("Dl") == "1" {
		disposition = "attachment"
	} else if !checkPermission {
		// Allow some files to be displayed by the browser in the client-side apps
		if doc.Mime == "text/plain" || doc.Class == "image" || doc.Class == "audio" || doc.Class == "video" || doc.Mime == "application/pdf" {
			c.Response().Header().Del(echo.HeaderXFrameOptions)
		}
	}

	serveFileContent(c, instance.VFS(), doc, disposition)
	return nil
}

// ReadFileContentFromPathHandler handles all GET request on /files/download
// aiming at downloading a file given its path. It serves the file in in
// attachment mode.
func ReadFileContentFromPathHandler(c echo.Context) error {
	return sendFileFromPath(c, c.QueryParam("Path"), true)
}

// ArchiveDownloadCreateHandler handles requests to /files/archive and stores the
// paremeters with a secret to be used in download handler below.s
func ArchiveDownloadCreateHandler(c echo.Context) error {
	archive := &vfs.Archive{}
	if _, err := jsonapi.Bind(c.Request(), archive); err != nil {
		return err
	}
	if len(archive.Files) == 0 && len(archive.IDs) == 0 {
		return c.JSON(http.StatusBadRequest, "Can't create an archive with no files")
	}
	if strings.Contains(archive.Name, "/") {
		return c.JSON(http.StatusBadRequest, "The archive filename can't contain a /")
	}
	if archive.Name == "" {
		archive.Name = "archive"
	}
	instance := middlewares.GetInstance(c)

	entries, err := archive.GetEntries(instance.VFS())
	if err != nil {
		return WrapVfsError(err)
	}

	for _, e := range entries {
		err = checkPerm(c, permissions.GET, e.Dir, e.File)
		if err != nil {
			return err
		}
	}

	// if accept header is application/zip, send the archive immediately
	if c.Request().Header.Get("Accept") == "application/zip" {
		return archive.Serve(instance.VFS(), c.Response())
	}

	secret, err := vfs.GetStore().AddArchive(instance.Domain, archive)
	if err != nil {
		return WrapVfsError(err)
	}
	archive.Secret = secret

	fakeName := url.PathEscape(archive.Name)

	links := &jsonapi.LinksList{
		Related: "/files/archive/" + secret + "/" + fakeName + ".zip",
	}

	return jsonapi.Data(c, http.StatusOK, &apiArchive{archive}, links)
}

// FileDownloadCreateHandler stores the required path into a secret
// usable for download handler below.
func FileDownloadCreateHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	var doc *vfs.FileDoc
	var err error
	var path string

	if path = c.QueryParam("Path"); path != "" {
		if doc, err = instance.VFS().FileByPath(path); err != nil {
			return WrapVfsError(err)
		}
	} else if id := c.QueryParam("Id"); id != "" {
		if doc, err = instance.VFS().FileByID(id); err != nil {
			return WrapVfsError(err)
		}
		if path, err = doc.Path(instance.VFS()); err != nil {
			return WrapVfsError(err)
		}
	}

	err = checkPerm(c, "GET", nil, doc)
	if err != nil {
		return err
	}

	secret, err := vfs.GetStore().AddFile(instance.Domain, path)
	if err != nil {
		return WrapVfsError(err)
	}

	links := &jsonapi.LinksList{
		Related: "/files/downloads/" + secret + "/" + doc.DocName,
	}

	return fileData(c, http.StatusOK, doc, links)
}

// ArchiveDownloadHandler handles requests to /files/archive/:secret/whatever.zip
// and creates on the fly zip archive from the parameters linked to secret.
func ArchiveDownloadHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	secret := c.Param("secret")
	archive, err := vfs.GetStore().GetArchive(instance.Domain, secret)
	if err != nil {
		return WrapVfsError(err)
	}
	if archive == nil {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}
	return archive.Serve(instance.VFS(), c.Response())
}

// FileDownloadHandler send a file that have previously be defined
// through FileDownloadCreateHandler
func FileDownloadHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	secret := c.Param("secret")
	path, err := vfs.GetStore().GetFile(instance.Domain, secret)
	if err != nil {
		return WrapVfsError(err)
	}
	if path == "" {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}
	return sendFileFromPath(c, path, false)
}

// TrashHandler handles all DELETE requests on /files/:file-id and
// moves the file or directory with the specified file-id to the
// trash.
func TrashHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")
	if fileID == "" {
		fileID = c.Param("docid") // Used by sharings.deleteDocument
	}

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permissions.PUT, dir, file)
	if err != nil {
		return err
	}

	var rev string
	if dir != nil {
		rev = dir.Rev()
	} else {
		rev = file.Rev()
	}

	if err := CheckIfMatch(c, rev); err != nil {
		return WrapVfsError(err)
	}

	if dir != nil {
		doc, errt := vfs.TrashDir(instance.VFS(), dir)
		if errt != nil {
			return WrapVfsError(errt)
		}
		return dirData(c, http.StatusOK, doc)
	}

	doc, errt := vfs.TrashFile(instance.VFS(), file)
	if errt != nil {
		return WrapVfsError(errt)
	}
	return fileData(c, http.StatusOK, doc, nil)
}

// ReadTrashFilesHandler handle GET requests on /files/trash and return the
// list of trashed files and directories
func ReadTrashFilesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	trash, err := instance.VFS().DirByID(consts.TrashDirID)
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permissions.GET, trash, nil)
	if err != nil {
		return err
	}

	return dirDataList(c, http.StatusOK, trash)
}

// RestoreTrashFileHandler handle POST requests on /files/trash/file-id and
// can be used to restore a file or directory from the trash.
func RestoreTrashFileHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permissions.PUT, dir, file)
	if err != nil {
		return err
	}

	if dir != nil {
		doc, errt := vfs.RestoreDir(instance.VFS(), dir)
		if errt != nil {
			return WrapVfsError(errt)
		}
		return dirData(c, http.StatusOK, doc)
	}

	doc, errt := vfs.RestoreFile(instance.VFS(), file)
	if errt != nil {
		return WrapVfsError(errt)
	}
	return fileData(c, http.StatusOK, doc, nil)
}

// ClearTrashHandler handles DELETE request to clear the trash
func ClearTrashHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	trash, err := instance.VFS().DirByID(consts.TrashDirID)
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permissions.DELETE, trash, nil)
	if err != nil {
		return err
	}

	err = instance.VFS().DestroyDirContent(trash)
	if err != nil {
		return WrapVfsError(err)
	}

	return c.NoContent(204)
}

// DestroyFileHandler handles DELETE request to clear one element from the trash
func DestroyFileHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permissions.DELETE, dir, file)
	if err != nil {
		return err
	}

	var rev string
	if dir != nil {
		rev = dir.Rev()
	} else {
		rev = file.Rev()
	}

	if err = CheckIfMatch(c, rev); err != nil {
		return WrapVfsError(err)
	}

	if dir != nil {
		err = instance.VFS().DestroyDirAndContent(dir)
	} else {
		err = instance.VFS().DestroyFile(file)
	}
	if err != nil {
		return WrapVfsError(err)
	}

	return c.NoContent(204)
}

const maxMangoLimit = 100

// FindFilesMango is the route POST /files/_find
// used to retrieve files and their metadata from a mango query.
func FindFilesMango(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	var findRequest map[string]interface{}

	if err := json.NewDecoder(c.Request().Body).Decode(&findRequest); err != nil {
		return jsonapi.NewError(http.StatusBadRequest, err)
	}

	if err := permissions.AllowWholeType(c, permissions.GET, consts.Files); err != nil {
		return err
	}

	// drop the fields, they can cause issues if not properly manipulated
	// TODO : optimization potential, necessary fields so far are class & type
	delete(findRequest, "fields")

	limit, hasLimit := findRequest["limit"].(float64)
	if !hasLimit || limit > maxMangoLimit {
		limit = 100
	}
	skip := 0
	skipF64, hasSkip := findRequest["skip"].(float64)
	if hasSkip {
		skip = int(skipF64)
	}

	// add 1 so we know if there is more.
	findRequest["limit"] = limit + 1

	var results []vfs.DirOrFileDoc
	err := couchdb.FindDocsRaw(instance, consts.Files, &findRequest, &results)
	if err != nil {
		return err
	}

	var total int
	if len(results) > int(limit) {
		total = math.MaxInt32 - 1          // we dont know the actual number
		results = results[:len(results)-1] // loose the last item
	} else {
		total = skip + len(results) // let the client know its done.
	}

	out := make([]jsonapi.Object, len(results))
	for i, dof := range results {
		d, f := dof.Refine()
		if d != nil {
			out[i] = newDir(d)
		} else {
			out[i] = newFile(f, instance)
		}
	}

	return jsonapi.DataListWithTotal(c, http.StatusOK, total, out, nil)

}

// Routes sets the routing for the files service
func Routes(router *echo.Group) {
	router.HEAD("/download", ReadFileContentFromPathHandler)
	router.GET("/download", ReadFileContentFromPathHandler)
	router.HEAD("/download/:file-id", ReadFileContentFromIDHandler)
	router.GET("/download/:file-id", ReadFileContentFromIDHandler)

	router.POST("/_find", FindFilesMango)

	router.HEAD("/:file-id", HeadDirOrFile)

	router.GET("/metadata", ReadMetadataFromPathHandler)
	router.GET("/:file-id", ReadMetadataFromIDHandler)
	router.GET("/:file-id/relationships/contents", GetChildrenHandler)

	router.PATCH("/metadata", ModifyMetadataByPathHandler)
	router.PATCH("/:file-id", ModifyMetadataByIDHandler)

	router.POST("/", CreationHandler)
	router.POST("/:file-id", CreationHandler)
	router.PUT("/:file-id", OverwriteFileContentHandler)

	router.GET("/:file-id/thumbnails/:secret/:format", ThumbnailHandler)

	router.POST("/archive", ArchiveDownloadCreateHandler)
	router.GET("/archive/:secret/:fake-name", ArchiveDownloadHandler)

	router.POST("/downloads", FileDownloadCreateHandler)
	router.GET("/downloads/:secret/:fake-name", FileDownloadHandler)

	router.POST("/:file-id/relationships/referenced_by", AddReferencedHandler)
	router.DELETE("/:file-id/relationships/referenced_by", RemoveReferencedHandler)

	router.GET("/trash", ReadTrashFilesHandler)
	router.DELETE("/trash", ClearTrashHandler)

	router.POST("/trash/:file-id", RestoreTrashFileHandler)
	router.DELETE("/trash/:file-id", DestroyFileHandler)

	router.DELETE("/:file-id", TrashHandler)
}

// WrapVfsError returns a formatted error from a golang error emitted by the vfs
func WrapVfsError(err error) error {
	switch err {
	case ErrDocTypeInvalid:
		return jsonapi.InvalidAttribute("type", err)
	case vfs.ErrParentDoesNotExist:
		return jsonapi.NotFound(err)
	case vfs.ErrParentInTrash:
		return jsonapi.NotFound(err)
	case vfs.ErrForbiddenDocMove:
		return jsonapi.PreconditionFailed("dir-id", err)
	case vfs.ErrIllegalFilename:
		return jsonapi.InvalidParameter("name", err)
	case vfs.ErrIllegalTime:
		return jsonapi.InvalidParameter("UpdatedAt", err)
	case vfs.ErrInvalidHash:
		return jsonapi.PreconditionFailed("Content-MD5", err)
	case vfs.ErrContentLengthMismatch:
		return jsonapi.PreconditionFailed("Content-Length", err)
	case vfs.ErrConflict:
		return jsonapi.Conflict(err)
	case vfs.ErrFileInTrash, vfs.ErrNonAbsolutePath,
		vfs.ErrDirNotEmpty:
		return jsonapi.BadRequest(err)
	case vfs.ErrFileTooBig:
		return jsonapi.NewError(http.StatusRequestEntityTooLarge, err)
	}
	return err
}

// FileDocFromReq creates a FileDoc from an incoming request.
func FileDocFromReq(c echo.Context, name, dirID string, tags []string) (*vfs.FileDoc, error) {
	header := c.Request().Header

	var err error
	var md5Sum []byte
	if md5Str := header.Get("Content-MD5"); md5Str != "" {
		md5Sum, err = parseMD5Hash(md5Str)
	}
	if err != nil {
		err = jsonapi.InvalidParameter("Content-MD5", err)
		return nil, err
	}

	cdate := time.Now()
	if date := header.Get("Date"); date != "" {
		if t, err := time.Parse(time.RFC1123, date); err == nil {
			cdate = t
		}
	}

	var mime, class string
	contentType := header.Get("Content-Type")
	if contentType == "" {
		mime, class = vfs.ExtractMimeAndClassFromFilename(name)
	} else if contentType == "application/octet-stream" {
		// TODO: remove this special path for the heic/heif file extensions with
		// when we deal with a better detection of the files magic numbers.
		switch strings.ToLower(path.Ext(name)) {
		case ".heif":
			contentType = "image/heif"
		case ".heic":
			contentType = "image/heic"
		}
		mime, class = vfs.ExtractMimeAndClass(contentType)
	} else {
		mime, class = vfs.ExtractMimeAndClass(contentType)
	}

	size := c.Request().ContentLength
	executable := c.QueryParam("Executable") == "true"
	trashed := false
	return vfs.NewFileDoc(
		name,
		dirID,
		size,
		md5Sum,
		mime,
		class,
		cdate,
		executable,
		trashed,
		tags,
	)
}

// CheckIfMatch checks if the revision provided matches the revision number
// given in the request, in the header and/or the query.
func CheckIfMatch(c echo.Context, rev string) error {
	ifMatch := c.Request().Header.Get("If-Match")
	revQuery := c.QueryParam("rev")
	var wantedRev string
	if ifMatch != "" {
		wantedRev = ifMatch
	}
	if revQuery != "" && wantedRev == "" {
		wantedRev = revQuery
	}
	if wantedRev != "" && rev != wantedRev {
		return jsonapi.PreconditionFailed("If-Match", fmt.Errorf("Revision does not match"))
	}
	return nil
}

func checkPerm(c echo.Context, v pkgperm.Verb, d *vfs.DirDoc, f *vfs.FileDoc) error {
	if d != nil {
		return permissions.AllowVFS(c, v, d)
	}

	return permissions.AllowVFS(c, v, f)
}

func parseMD5Hash(md5B64 string) ([]byte, error) {
	if base64.StdEncoding.DecodedLen(len(md5B64)) > 18 {
		return nil, ErrInvalidContentMD5
	}
	md5Sum, err := base64.StdEncoding.DecodeString(md5B64)
	if err != nil || len(md5Sum) != 16 {
		return nil, ErrInvalidContentMD5
	}
	return md5Sum, nil
}
