package vfs

import (
	"bytes"
	"crypto/md5" // #nosec
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/spf13/afero"
)

// FileDoc is a struct containing all the informations about a file.
// It implements the couchdb.Doc and jsonapi.Object interfaces.
type FileDoc struct {
	// Type of document. Useful to (de)serialize and filter the data
	// from couch.
	Type string `json:"type"`
	// Qualified file identifier
	FID string `json:"_id,omitempty"`
	// File revision
	FRev string `json:"_rev,omitempty"`
	// File name
	Name string `json:"name"`
	// Parent folder identifier
	FolderID string `json:"folder_id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Size       int64    `json:"size,string"`
	MD5Sum     []byte   `json:"md5sum"`
	Mime       string   `json:"mime"`
	Class      string   `json:"class"`
	Executable bool     `json:"executable"`
	Tags       []string `json:"tags"`

	parent *DirDoc
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
	return FsDocType
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

// SelfLink is used to generate a JSON-API link for the file (part of
// jsonapi.Object interface)
func (f *FileDoc) SelfLink() string {
	return "/files/" + f.FID
}

// Path is used to generate the file path
func (f *FileDoc) Path(c *Context) (string, error) {
	var parentPath string
	if f.FolderID == RootFolderID {
		parentPath = "/"
	} else if f.parent == nil {
		parent, err := GetDirectoryDoc(c, f.FolderID, false)
		if err != nil {
			return "", err
		}
		f.parent = parent
		parentPath = parent.Path
	} else {
		parentPath = f.parent.Path
	}
	return path.Join(parentPath, f.Name), nil
}

// Relationships is used to generate the parent relationship in JSON-API format
// (part of the jsonapi.Object interface)
func (f *FileDoc) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{
		"parent": jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Related: "/files/" + f.FolderID,
			},
			Data: jsonapi.ResourceIdentifier{
				ID:   f.FolderID,
				Type: FsDocType,
			},
		},
	}
}

// Included is part of the jsonapi.Object interface
func (f *FileDoc) Included() []jsonapi.Object {
	return []jsonapi.Object{}
}

// NewFileDoc is the FileDoc constructor. The given name is validated.
func NewFileDoc(name, folderID string, size int64, md5Sum []byte, mime, class string, executable bool, tags []string, parent *DirDoc) (doc *FileDoc, err error) {
	if err = checkFileName(name); err != nil {
		return
	}

	if folderID == "" {
		folderID = RootFolderID
	}

	tags = appendTags(nil, tags)

	createDate := time.Now()
	doc = &FileDoc{
		Type:     FileType,
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

		parent: parent,
	}

	return
}

// GetFileDoc is used to fetch file document information form the
// database.
func GetFileDoc(c *Context, fileID string) (doc *FileDoc, err error) {
	doc = &FileDoc{}
	err = couchdb.GetDoc(c.db, FsDocType, fileID, doc)
	return doc, err
}

// GetFileDocFromPath is used to fetch file document information from
// the database from its path.
func GetFileDocFromPath(c *Context, pth string) (*FileDoc, error) {
	var err error
	var folderID string

	dirpath := path.Dir(pth)
	if dirpath != "/" {
		var parent *DirDoc
		parent, err = GetDirectoryDocFromPath(c, dirpath, false)
		if err != nil {
			return nil, err
		}
		folderID = parent.ID()
	} else {
		folderID = RootFolderID
	}

	selector := mango.And(
		mango.Equal("folder_id", folderID),
		mango.Equal("name", path.Base(pth)),
	)

	var docs []*FileDoc
	req := &couchdb.FindRequest{
		Selector: selector,
		Limit:    1,
	}
	err = couchdb.FindDocs(c.db, FsDocType, req, &docs)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, os.ErrNotExist
	}
	return docs[0], nil
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
func ServeFileContent(c *Context, doc *FileDoc, disposition string, req *http.Request, w http.ResponseWriter) (err error) {
	header := w.Header()
	header.Set("Content-Type", doc.Mime)
	header.Set("Content-Disposition", disposition+"; filename="+doc.Name)

	if header.Get("Range") == "" {
		eTag := base64.StdEncoding.EncodeToString(doc.MD5Sum)
		header.Set("Etag", eTag)
	}

	pth, err := doc.Path(c)
	if err != nil {
		return
	}

	content, err := c.fs.Open(pth)
	if err != nil {
		return
	}
	defer content.Close()

	http.ServeContent(w, req, doc.Name, doc.UpdatedAt, content)
	return
}

// CreateFileAndUpload is the method for uploading a file onto the filesystem.
func CreateFileAndUpload(c *Context, doc *FileDoc, body io.Reader) (err error) {
	newpath, _, err := getFilePath(c, doc.Name, doc.FolderID)
	if err != nil {
		return err
	}

	file, err := safeCreateFile(newpath, doc.Executable, c.fs)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			c.fs.Remove(newpath)
		}
	}()

	written, md5Sum, err := copyOnFsAndCheckIntegrity(file, doc.MD5Sum, c.fs, body)
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

	return couchdb.CreateDoc(c.db, doc)
}

// ModifyFileContent overrides the content of a file onto the
// filesystem.
//
// This method should change the file content atomically. If any error
// happens while copying the content, the previous file revision is
// kept undamaged.
func ModifyFileContent(c *Context, olddoc *FileDoc, newdoc *FileDoc, body io.Reader) (err error) {
	mdate := time.Now()

	tmppath := "/" + olddoc.ID() + "_" + olddoc.Rev() + "_" + strconv.FormatInt(mdate.UnixNano(), 10)
	newpath, err := olddoc.Path(c)
	if err != nil {
		return err
	}

	file, err := safeCreateFile(tmppath, newdoc.Executable, c.fs)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			c.fs.Remove(tmppath)
		}
	}()

	written, md5Sum, err := copyOnFsAndCheckIntegrity(file, newdoc.MD5Sum, c.fs, body)
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

	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	newdoc.CreatedAt = olddoc.CreatedAt
	newdoc.UpdatedAt = mdate

	err = couchdb.UpdateDoc(c.db, newdoc)
	if err != nil {
		return err
	}

	return renameFile(tmppath, newpath, c.fs)
}

// ModifyFileMetadata modify the metadata associated to a file. It can
// be used to rename or move the file in the VFS.
func ModifyFileMetadata(c *Context, olddoc *FileDoc, data *DocMetaAttributes) (newdoc *FileDoc, err error) {
	oldpath, err := olddoc.Path(c)
	if err != nil {
		return
	}
	newpath := oldpath
	name := olddoc.Name
	tags := olddoc.Tags
	exec := olddoc.Executable
	folderID := olddoc.FolderID
	mdate := olddoc.UpdatedAt
	parent := olddoc.parent

	if data.FolderID != nil && *data.FolderID != folderID {
		folderID = *data.FolderID
		newpath, parent, err = getFilePath(c, name, folderID)
		if err != nil {
			return
		}
	}

	if data.Name != "" {
		name = data.Name
		newpath = path.Join(path.Dir(newpath), name)
	}

	if data.Tags != nil {
		tags = appendTags(tags, data.Tags)
	}

	if data.Executable != nil {
		exec = *data.Executable
	}

	if data.UpdatedAt != nil {
		mdate = *data.UpdatedAt
	}

	if mdate.Before(olddoc.CreatedAt) {
		err = ErrIllegalTime
		return
	}

	newdoc, err = NewFileDoc(
		name,
		folderID,
		olddoc.Size,
		olddoc.MD5Sum,
		olddoc.Mime,
		olddoc.Class,
		exec,
		tags,
		parent,
	)
	if err != nil {
		return
	}

	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	newdoc.CreatedAt = olddoc.CreatedAt
	newdoc.UpdatedAt = mdate

	if newpath != oldpath {
		err = renameFile(oldpath, newpath, c.fs)
		if err != nil {
			return
		}
	}

	if exec != olddoc.Executable {
		err = c.fs.Chmod(newpath, getFileMode(exec))
		if err != nil {
			return
		}
	}

	err = couchdb.UpdateDoc(c.db, newdoc)
	return
}

func safeCreateFile(pth string, executable bool, fs afero.Fs) (afero.File, error) {
	// write only (O_WRONLY), try to create the file and check that it
	// does not already exist (O_CREATE|O_EXCL).
	flag := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	mode := getFileMode(executable)
	return fs.OpenFile(pth, flag, mode)
}

func copyOnFsAndCheckIntegrity(file io.WriteCloser, givenMD5 []byte, fs afero.Fs, r io.Reader) (written int64, md5Sum []byte, err error) {
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

func renameFile(oldpath, newpath string, fs afero.Fs) error {
	newpath = path.Clean(newpath)
	oldpath = path.Clean(oldpath)

	if !path.IsAbs(newpath) || !path.IsAbs(oldpath) {
		return fmt.Errorf("renameFile: paths should be absolute")
	}

	return fs.Rename(oldpath, newpath)
}

func getFileMode(executable bool) (mode os.FileMode) {
	if executable {
		mode = 0755 // -rwxr-xr-x
	} else {
		mode = 0644 // -rw-r--r--
	}
	return
}

var (
	_ couchdb.Doc    = &FileDoc{}
	_ jsonapi.Object = &FileDoc{}
)
