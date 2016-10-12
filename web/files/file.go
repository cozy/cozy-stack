package files

import (
	"bytes"
	"crypto/md5" // #nosec
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/spf13/afero"
)

// DefaultContentType is used for files uploaded with no content-type
const DefaultContentType = "application/octet-stream"

type fileAttributes struct {
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Size       int64     `json:"size,string"`
	Tags       []string  `json:"tags"`
	MD5Sum     []byte    `json:"md5sum"`
	Executable bool      `json:"executable"`
	Class      string    `json:"class"`
	Mime       string    `json:"mime"`
}

// FileDoc is a struct containing all the informations about a file.
// It implements the couchdb.Doc and jsonapi.JSONApier interfaces.
type FileDoc struct {
	// Qualified file identifier
	FID string `json:"_id,omitempty"`
	// File revision
	FRev string `json:"_rev,omitempty"`
	// File attributes
	Attrs *fileAttributes `json:"attributes"`
	// Parent folder identifier
	FolderID string `json:"folderID"`
	// File path on VFS
	Path string `json:"path"`
}

// ID returns the file qualified identifier (part of couchdb.Doc
// interface)
func (f *FileDoc) ID() string {
	return f.FID
}

// Rev returns the file revision (part of couchdb.Doc interface)
func (f *FileDoc) Rev() string {
	return f.FRev
}

// DocType returns the file document type (part of couchdb.Doc
// interface)
func (f *FileDoc) DocType() string {
	return string(FileDocType)
}

// SetID is used to change the file qualified identifier (part of
// couchdb.Doc interface)
func (f *FileDoc) SetID(id string) {
	f.FID = id
}

// SetRev is used to change the file revision (part of couchdb.Doc
// interface)
func (f *FileDoc) SetRev(rev string) {
	f.FRev = rev
}

// ToJSONApi implements temporary interface JSONApier to serialize
// the file document
func (f *FileDoc) ToJSONApi() ([]byte, error) {
	id := f.FID
	data := map[string]interface{}{
		"id":         id,
		"type":       f.DocType(),
		"rev":        f.Rev(),
		"attributes": f.Attrs,
	}
	m := map[string]interface{}{
		"data": data,
	}
	return json.Marshal(m)
}

// GetFileDoc is used to fetch file document information form our
// database.
func GetFileDoc(fileID, dbPrefix string) (doc *FileDoc, err error) {
	doc = &FileDoc{}
	err = couchdb.GetDoc(dbPrefix, string(FileDocType), fileID, doc)
	return
}

// ServeFileContent replies to a http request using the content of a
// file given its FileDoc.
//
// It uses internally http.ServeContent and benefits from it by
// offering support to Range, If-Modified-Since and If-None-Match
// requests. It uses the revision of the file as the Etag value for
// non-ranged requests
//
// The content disposition is inlined.
func ServeFileContent(fileDoc *FileDoc, req *http.Request, w http.ResponseWriter, fs afero.Fs) (err error) {
	attrs := fileDoc.Attrs
	header := w.Header()
	header.Set("Content-Type", attrs.Mime)
	header.Set("Content-Disposition", "inline; filename="+attrs.Name)

	if header.Get("Range") == "" {
		eTag := base64.StdEncoding.EncodeToString(fileDoc.Attrs.MD5Sum)
		header.Set("Etag", eTag)
	}

	serveContent(req, w, fs, fileDoc.Path, attrs.Name, attrs.UpdatedAt)
	return
}

// ServeFileContentByPath replies to a http request using the content
// of a file identified by its full path on the VFS. Unlike
// ServeFileContent, this method does not require the full file
// document but only its path.
//
// It also uses internally http.ServeContent but does not provide an
// Etag.
//
// The content disposition is attached
func ServeFileContentByPath(pth string, req *http.Request, w http.ResponseWriter, fs afero.Fs) (err error) {
	fileInfo, err := fs.Stat(pth)
	if err != nil {
		return
	}

	name := path.Base(pth)
	w.Header().Set("Content-Disposition", "attachment; filename="+name)

	serveContent(req, w, fs, pth, name, fileInfo.ModTime())
	return
}

func serveContent(req *http.Request, w http.ResponseWriter, fs afero.Fs, pth, name string, modtime time.Time) (err error) {
	content, err := fs.Open(pth)
	if err != nil {
		return
	}

	defer content.Close()
	http.ServeContent(w, req, name, modtime, content)
	return
}

// CreateFileAndUpload is the method for uploading a file onto the filesystem.
func CreateFileAndUpload(m *DocMetadata, fs afero.Fs, contentType string, contentLength int64, dbPrefix string, body io.ReadCloser) (doc *FileDoc, err error) {
	if m.Type != FileDocType {
		err = errDocTypeInvalid
		return
	}

	pth, _, err := createNewFilePath(m, fs, dbPrefix)
	if err != nil {
		return
	}

	mime, class := extractMimeAndClass(contentType)
	createDate := time.Now()
	attrs := &fileAttributes{
		Name:       m.Name,
		CreatedAt:  createDate,
		UpdatedAt:  createDate,
		Size:       contentLength,
		Tags:       m.Tags,
		MD5Sum:     m.GivenMD5,
		Executable: m.Executable,
		Class:      class,
		Mime:       mime,
	}

	doc = &FileDoc{
		Attrs:    attrs,
		FolderID: m.FolderID,
		Path:     pth,
	}

	// Error handling to make sure the steps of uploading the file and
	// creating the corresponding are both rollbacked in case of an
	// error. This should preserve our VFS coherency a little.
	defer func() {
		if err != nil {
			fs.Remove(pth)
		}
	}()

	var written int64
	if written, err = copyOnFsAndCheckIntegrity(m, fs, pth, body); err != nil {
		return
	}

	if contentLength >= 0 && written != contentLength {
		err = errContentLengthMismatch
		return
	}

	if contentLength < 0 {
		attrs.Size = written
	}

	if err = couchdb.CreateDoc(dbPrefix, doc); err != nil {
		return
	}

	return
}

func copyOnFsAndCheckIntegrity(m *DocMetadata, fs afero.Fs, pth string, r io.ReadCloser) (written int64, err error) {
	f, err := fs.Create(pth)
	if err != nil {
		return
	}

	defer f.Close()
	defer r.Close()

	md5H := md5.New() // #nosec
	written, err = io.Copy(f, io.TeeReader(r, md5H))
	if err != nil {
		return
	}

	calcMD5 := md5H.Sum(nil)
	if !bytes.Equal(m.GivenMD5, calcMD5) {
		err = errInvalidHash
		return
	}

	return
}

func extractMimeAndClass(contentType string) (mime, class string) {
	if contentType == "" {
		contentType = DefaultContentType
	}

	charsetIndex := strings.Index(contentType, ";")
	if charsetIndex >= 0 {
		mime = contentType[:charsetIndex]
	} else {
		mime = contentType
	}

	// @TODO improve for specific mime types
	slashIndex := strings.Index(contentType, "/")
	if slashIndex >= 0 {
		class = contentType[:slashIndex]
	} else {
		class = contentType
	}

	return
}
