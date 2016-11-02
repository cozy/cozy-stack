package vfs

import (
	"bytes"
	"crypto/md5" // #nosec
	"encoding/base64"
	"fmt"
	"hash"
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
	ObjID string `json:"_id,omitempty"`
	// File revision
	ObjRev string `json:"_rev,omitempty"`
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
	return f.ObjID
}

// Rev returns the file revision (part of couchdb.Doc interface)
func (f *FileDoc) Rev() string {
	return f.ObjRev
}

// DocType returns the file document type (part of couchdb.Doc
// interface)
func (f *FileDoc) DocType() string {
	return FsDocType
}

// SetID is used to change the file qualified identifier (part of
// couchdb.Doc interface)
func (f *FileDoc) SetID(id string) {
	f.ObjID = id
}

// SetRev is used to change the file revision (part of couchdb.Doc
// interface)
func (f *FileDoc) SetRev(rev string) {
	f.ObjRev = rev
}

// SelfLink is used to generate a JSON-API link for the file (part of
// jsonapi.Object interface)
func (f *FileDoc) SelfLink() string {
	return "/files/" + f.ObjID
}

// Path is used to generate the file path
func (f *FileDoc) Path(c *Context) (string, error) {
	var parentPath string
	if f.FolderID == RootFolderID {
		parentPath = "/"
	} else {
		parent, err := f.Parent(c)
		if err != nil {
			return "", err
		}
		parentPath, err = parent.Path(c)
		if err != nil {
			return "", err
		}
	}
	return path.Join(parentPath, f.Name), nil
}

// Parent returns the parent directory document
func (f *FileDoc) Parent(c *Context) (*DirDoc, error) {
	parent, err := getParentDir(c, f.parent, f.FolderID)
	if err != nil {
		return nil, err
	}
	f.parent = parent
	return parent, nil
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

	tags = uniqueTags(tags)

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
func GetFileDoc(c *Context, fileID string) (*FileDoc, error) {
	doc := &FileDoc{}
	err := couchdb.GetDoc(c.db, FsDocType, fileID, doc)
	if err != nil {
		return nil, err
	}
	if doc.Type != FileType {
		return nil, os.ErrNotExist
	}
	return doc, nil
}

// GetFileDocFromPath is used to fetch file document information from
// the database from its path.
func GetFileDocFromPath(c *Context, pth string) (*FileDoc, error) {
	var err error

	dirpath := path.Dir(pth)
	var parent *DirDoc
	parent, err = GetDirDocFromPath(c, dirpath, false)

	if err != nil {
		return nil, err
	}

	folderID := parent.ID()
	selector := mango.And(
		mango.Equal("folder_id", folderID),
		mango.Equal("name", path.Base(pth)),
		mango.Equal("type", FileType),
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

	fileDoc := docs[0]
	fileDoc.parent = parent

	return fileDoc, nil
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

// FileCreation represents a file open for writing. It is used to
// create of file or to modify the content of a file.
//
// FileCreation implements io.WriteCloser.
type FileCreation struct {
	c *Context   // vfs context
	f afero.File // file handle
	w int64      // total size written

	newdoc    *FileDoc  // new document
	olddoc    *FileDoc  // old document if any
	path      string    // file full path
	tmppath   string    // temporary file path in case of modifying an existing file
	checkHash bool      // whether or not we need the assert the hash is good
	hash      hash.Hash // hash we build up along the file
}

// CreateFile is used to create file or modify an existing file
// content. It returns a FileCreation handle. Along with the vfs
// context, it receives the new file document that you want to create.
// It can also receive the old document, representing the current
// revision of the file. In this case it will try to modify the file,
// otherwise it will create it.
//
// Warning: you MUST call the Close() method and check for its error.
// The Close() method will actually create or update the document in
// couchdb. It will also check the md5 hash if required.
func CreateFile(c *Context, newdoc, olddoc *FileDoc) (*FileCreation, error) {
	now := time.Now()

	newpath, err := newdoc.Path(c)
	if err != nil {
		return nil, err
	}

	var tmppath string
	if olddoc != nil {
		tmppath = "/" + olddoc.ID() + "_" + olddoc.Rev() + "_" + strconv.FormatInt(now.UnixNano(), 10)
	} else {
		tmppath = newpath
	}

	if olddoc != nil {
		newdoc.SetID(olddoc.ID())
		newdoc.SetRev(olddoc.Rev())
		newdoc.CreatedAt = olddoc.CreatedAt
	} else {
		newdoc.CreatedAt = now
	}

	newdoc.UpdatedAt = now

	f, err := safeCreateFile(tmppath, newdoc.Executable, c.fs)
	if err != nil {
		return nil, err
	}

	hash := md5.New() // #nosec

	return &FileCreation{
		c: c,
		f: f,
		w: 0,

		newdoc:  newdoc,
		olddoc:  olddoc,
		tmppath: tmppath,
		path:    newpath,

		checkHash: newdoc.MD5Sum != nil,
		hash:      hash,
	}, nil
}

// Write bytes to the file - part of io.WriteCloser
func (fc *FileCreation) Write(p []byte) (n int, err error) {
	n, err = fc.f.Write(p)
	if err != nil {
		return
	}

	fc.w += int64(n)

	_, err = fc.hash.Write(p)
	return
}

// Close the handle and commit the document in database if all checks
// are OK. It is important to check errors returned by this method.
func (fc *FileCreation) Close() error {
	var err error
	c := fc.c

	defer func() {
		if err != nil {
			c.fs.Remove(fc.tmppath)
		}
	}()

	err = fc.f.Close()
	if err != nil {
		return err
	}

	newdoc, olddoc, written := fc.newdoc, fc.olddoc, fc.w

	md5sum := fc.hash.Sum(nil)
	if fc.checkHash && !bytes.Equal(newdoc.MD5Sum, md5sum) {
		err = ErrInvalidHash
		return err
	}

	if newdoc.Size < 0 {
		newdoc.Size = written
	}

	if newdoc.MD5Sum == nil {
		newdoc.MD5Sum = md5sum
	}

	if newdoc.Size != written {
		err = ErrContentLengthMismatch
		return err
	}

	if olddoc != nil {
		err = couchdb.UpdateDoc(c.db, newdoc)
	} else {
		err = couchdb.CreateDoc(c.db, newdoc)
	}

	if err != nil {
		return err
	}

	if fc.tmppath != fc.path {
		err = c.fs.Rename(fc.tmppath, fc.path)
	}

	return err
}

// ModifyFileMetadata modify the metadata associated to a file. It can
// be used to rename or move the file in the VFS.
func ModifyFileMetadata(c *Context, olddoc *FileDoc, patch *DocPatch) (newdoc *FileDoc, err error) {
	cdate := olddoc.CreatedAt
	patch, err = normalizeDocPatch(&DocPatch{
		Name:       &olddoc.Name,
		FolderID:   &olddoc.FolderID,
		Tags:       &olddoc.Tags,
		UpdatedAt:  &olddoc.UpdatedAt,
		Executable: &olddoc.Executable,
	}, patch, cdate)

	if err != nil {
		return
	}

	newdoc, err = NewFileDoc(
		*patch.Name,
		*patch.FolderID,
		olddoc.Size,
		olddoc.MD5Sum,
		olddoc.Mime,
		olddoc.Class,
		*patch.Executable,
		*patch.Tags,
		nil,
	)
	if err != nil {
		return
	}

	var parent *DirDoc
	if newdoc.FolderID != olddoc.FolderID {
		parent, err = newdoc.Parent(c)
	} else {
		parent = olddoc.parent
	}

	if err != nil {
		return
	}

	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	newdoc.CreatedAt = cdate
	newdoc.UpdatedAt = *patch.UpdatedAt
	newdoc.parent = parent

	oldpath, err := olddoc.Path(c)
	if err != nil {
		return
	}
	newpath, err := newdoc.Path(c)
	if err != nil {
		return
	}

	if newpath != oldpath {
		err = safeRenameFile(c, oldpath, newpath)
		if err != nil {
			return
		}
	}

	if newdoc.Executable != olddoc.Executable {
		err = c.fs.Chmod(newpath, getFileMode(newdoc.Executable))
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

func safeRenameFile(c *Context, oldpath, newpath string) error {
	newpath = path.Clean(newpath)
	oldpath = path.Clean(oldpath)

	if !path.IsAbs(newpath) || !path.IsAbs(oldpath) {
		return fmt.Errorf("paths should be absolute")
	}

	_, err := c.fs.Stat(newpath)
	if err == nil {
		return os.ErrExist
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return c.fs.Rename(oldpath, newpath)
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
