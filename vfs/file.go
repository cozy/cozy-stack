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
		return err
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
func CreateFileAndUpload(doc *FileDoc, fs afero.Fs, dbPrefix string, body io.Reader) (err error) {
	newpath, _, err := createNewFilePath(doc.Name, doc.FolderID, fs, dbPrefix)
	if err != nil {
		return err
	}

	file, err := safeCreateFile(newpath, doc.Executable, fs)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			fs.Remove(newpath)
		}
	}()

	written, md5Sum, err := copyOnFsAndCheckIntegrity(file, doc.MD5Sum, fs, body)
	if err != nil {
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

	doc.Path = newpath

	return couchdb.CreateDoc(dbPrefix, doc)
}

// ModifyFileContent overrides the content of a file onto the
// filesystem.
//
// This method should change the file content atomically. If any error
// happens while copying the content, the previous file revision is
// kept undamaged.
func ModifyFileContent(olddoc *FileDoc, newdoc *FileDoc, fs afero.Fs, dbPrefix string, body io.Reader) (err error) {
	mdate := time.Now()

	tmppath := "/" + olddoc.ID() + "_" + olddoc.Rev() + "_" + strconv.FormatInt(mdate.UnixNano(), 10)
	newpath := olddoc.Path
	if err != nil {
		return err
	}

	file, err := safeCreateFile(tmppath, newdoc.Executable, fs)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			fs.Remove(tmppath)
		}
	}()

	written, md5Sum, err := copyOnFsAndCheckIntegrity(file, newdoc.MD5Sum, fs, body)
	if err != nil {
		return err
	}

	if newdoc.Size < 0 {
		newdoc.Size = written
	}

	if newdoc.MD5Sum == nil {
		newdoc.MD5Sum = md5Sum
	}

	if newdoc.Size != written {
		return ErrContentLengthMismatch
	}

	newdoc.Path = newpath
	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	newdoc.CreatedAt = olddoc.CreatedAt
	newdoc.UpdatedAt = mdate

	err = couchdb.UpdateDoc(dbPrefix, newdoc)
	if err != nil {
		return err
	}

	return fs.Rename(tmppath, newpath)
}

func safeCreateFile(pth string, executable bool, fs afero.Fs) (afero.File, error) {
	// write only (O_WRONLY), try to create the file and check that it
	// does not already exist (O_CREATE|O_EXCL).
	flag := os.O_WRONLY | os.O_CREATE | os.O_EXCL

	var mode os.FileMode
	if executable {
		mode = 0755 // -rwxr-xr-x
	} else {
		mode = 0644 // -rw-r--r--
	}

	return fs.OpenFile(pth, flag, mode)
}

func copyOnFsAndCheckIntegrity(file afero.File, givenMD5 []byte, fs afero.Fs, r io.Reader) (written int64, md5Sum []byte, err error) {
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	md5H := md5.New() // #nosec

	written, err = io.Copy(file, io.TeeReader(r, md5H))
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
