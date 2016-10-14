package vfs

import (
	"bytes"
	"crypto/md5" // #nosec
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/spf13/afero"
)

// FileDoc is a struct containing all the informations about a file.
// It implements the couchdb.Doc and jsonapi.JSONApier interfaces.
type FileDoc struct {
	// Qualified file identifier
	FID string `json:"_id,omitempty"`
	// File revision
	FRev string `json:"_rev,omitempty"`
	// File name
	Name string `json:"name"`
	// Parent folder identifier
	FolderID string `json:"folderID"`
	// File path on VFS
	Path string `json:"path"`

	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Size       int64     `json:"size,string"`
	Tags       []string  `json:"tags"`
	MD5Sum     []byte    `json:"md5sum"`
	Executable bool      `json:"executable"`
	Class      string    `json:"class"`
	Mime       string    `json:"mime"`
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
	attrs := map[string]interface{}{
		"name":       f.Name,
		"created_at": f.CreatedAt,
		"updated_at": f.UpdatedAt,
		"size":       strconv.FormatInt(f.Size, 10),
		"tags":       f.Tags,
		"md5sum":     f.MD5Sum,
		"executable": f.Executable,
		"class":      f.Class,
		"mime":       f.Mime,
	}
	data := map[string]interface{}{
		"id":         id,
		"type":       f.DocType(),
		"rev":        f.Rev(),
		"attributes": attrs,
	}
	m := map[string]interface{}{
		"data": data,
	}
	return json.Marshal(m)
}

// NewFileDoc is the FileDoc constructor. The given name is validated.
func NewFileDoc(name, folderID string, size int64, md5Sum []byte, mime, class string, executable bool, tags []string) (doc *FileDoc, err error) {
	if err = checkFileName(name); err != nil {
		return
	}

	createDate := time.Now()
	doc = &FileDoc{
		Name:     name,
		FolderID: folderID,

		CreatedAt:  createDate,
		UpdatedAt:  createDate,
		Size:       size,
		MD5Sum:     md5Sum,
		Mime:       mime,
		Class:      class,
		Executable: executable,
		Tags:       tags,
	}

	return
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
func ServeFileContent(doc *FileDoc, req *http.Request, w http.ResponseWriter, fs afero.Fs) (err error) {
	header := w.Header()
	header.Set("Content-Type", doc.Mime)
	header.Set("Content-Disposition", "inline; filename="+doc.Name)

	if header.Get("Range") == "" {
		eTag := base64.StdEncoding.EncodeToString(doc.MD5Sum)
		header.Set("Etag", eTag)
	}

	return serveContent(req, w, fs, doc.Path, doc.Name, doc.UpdatedAt)
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
func ServeFileContentByPath(pth string, req *http.Request, w http.ResponseWriter, fs afero.Fs) error {
	fileInfo, err := fs.Stat(pth)
	if err != nil {
		return ErrDocDoesNotExist
	}

	name := path.Base(pth)
	w.Header().Set("Content-Disposition", "attachment; filename="+name)

	return serveContent(req, w, fs, pth, name, fileInfo.ModTime())
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
func CreateFileAndUpload(doc *FileDoc, fs afero.Fs, dbPrefix string, body io.Reader) error {
	var err error

	pth, _, err := createNewFilePath(doc.Name, doc.FolderID, fs, dbPrefix)
	if err != nil {
		return err
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
	var md5Sum []byte
	if written, md5Sum, err = copyOnFsAndCheckIntegrity(pth, doc.MD5Sum, doc.Executable, fs, body); err != nil {
		return err
	}

	if doc.Size < 0 {
		doc.Size = written
	}

	if doc.MD5Sum == nil {
		doc.MD5Sum = md5Sum
	}

	if doc.Size != written {
		return ErrContentLengthMismatch
	}

	doc.Path = pth

	return couchdb.CreateDoc(dbPrefix, doc)
}

// ModifyFileContent overrides the content of a file onto the
// filesystem.
//
// @TODO: make it more resilient to not lose data if the transfer
// fails.
func ModifyFileContent(oldDoc *FileDoc, newDoc *FileDoc, fs afero.Fs, dbPrefix string, body io.Reader) (err error) {
	updateDate := time.Now()

	pth := oldDoc.Path

	defer func() {
		if err != nil {
			fs.Remove(pth)
		}
	}()

	var written int64
	var md5Sum []byte
	if written, md5Sum, err = copyOnFsAndCheckIntegrity(pth, newDoc.MD5Sum, newDoc.Executable, fs, body); err != nil {
		return err
	}

	if newDoc.Size < 0 {
		newDoc.Size = written
	}

	if newDoc.MD5Sum == nil {
		newDoc.MD5Sum = md5Sum
	}

	if newDoc.Size != written {
		return ErrContentLengthMismatch
	}

	newDoc.Path = pth
	newDoc.SetID(oldDoc.ID())
	newDoc.SetRev(oldDoc.Rev())
	newDoc.CreatedAt = oldDoc.CreatedAt
	newDoc.UpdatedAt = updateDate

	return couchdb.UpdateDoc(dbPrefix, newDoc)
}

func copyOnFsAndCheckIntegrity(pth string, givenMD5 []byte, executable bool, fs afero.Fs, r io.Reader) (written int64, md5Sum []byte, err error) {
	var mode os.FileMode
	if executable {
		mode = 0755 // -rwxr-xr-x
	} else {
		mode = 0644 // -rw-r--r--
	}

	// We want to write only (O_WRONLY), create the file if it does not
	// already exist (O_CREATE) and truncate it to length 0 if necessary
	// (O_TRUNC).
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	f, err := fs.OpenFile(pth, flag, mode)
	if err != nil {
		return
	}

	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	err = fs.Chmod(pth, mode)
	if err != nil {
		return
	}

	md5H := md5.New() // #nosec

	written, err = io.Copy(f, io.TeeReader(r, md5H))
	if err != nil {
		return
	}

	doCheck := givenMD5 != nil
	md5Sum = md5H.Sum(nil)
	if doCheck && !bytes.Equal(givenMD5, md5Sum) {
		err = ErrInvalidHash
		return
	}

	return
}
