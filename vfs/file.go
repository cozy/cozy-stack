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
	"strings"
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
	DocID string `json:"_id,omitempty"`
	// File revision
	DocRev string `json:"_rev,omitempty"`
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

// ID returns the file qualified identifier
func (f *FileDoc) ID() string { return f.DocID }

// Rev returns the file revision
func (f *FileDoc) Rev() string { return f.DocRev }

// DocType returns the file document type
func (f *FileDoc) DocType() string { return FsDocType }

// SetID changes the file qualified identifier
func (f *FileDoc) SetID(id string) { f.DocID = id }

// SetRev changes the file revision
func (f *FileDoc) SetRev(rev string) { f.DocRev = rev }

// Path is used to generate the file path
func (f *FileDoc) Path(c Context) (string, error) {
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
func (f *FileDoc) Parent(c Context) (*DirDoc, error) {
	parent, err := getParentDir(c, f.parent, f.FolderID)
	if err != nil {
		return nil, err
	}
	f.parent = parent
	return parent, nil
}

// SelfLink is used to generate a JSON-API link for the file (part of
// jsonapi.Object interface)
func (f *FileDoc) SelfLink() string {
	return "/files/" + f.DocID
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
func NewFileDoc(name, folderID string, size int64, md5Sum []byte, mime, class string, executable bool, tags []string) (doc *FileDoc, err error) {
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
	}

	return
}

// GetFileDoc is used to fetch file document information form the
// database.
func GetFileDoc(c Context, fileID string) (*FileDoc, error) {
	doc := &FileDoc{}
	err := couchdb.GetDoc(c, FsDocType, fileID, doc)
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
func GetFileDocFromPath(c Context, name string) (*FileDoc, error) {
	if !path.IsAbs(name) {
		return nil, ErrNonAbsolutePath
	}

	var err error
	dirpath := path.Dir(name)
	var parent *DirDoc
	parent, err = GetDirDocFromPath(c, dirpath, false)

	if err != nil {
		return nil, err
	}

	folderID := parent.ID()
	selector := mango.Map{
		"folder_id": folderID,
		"name":      path.Base(name),
		"type":      FileType,
	}

	var docs []*FileDoc
	req := &couchdb.FindRequest{
		Selector: selector,
		Limit:    1,
	}
	err = couchdb.FindDocs(c, FsDocType, req, &docs)
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
func ServeFileContent(c Context, doc *FileDoc, disposition string, req *http.Request, w http.ResponseWriter) (err error) {
	header := w.Header()
	header.Set("Content-Type", doc.Mime)
	if disposition != "" {
		header.Set("Content-Disposition", fmt.Sprintf("%s; filename=%s", disposition, doc.Name))
	}

	if header.Get("Range") == "" {
		eTag := base64.StdEncoding.EncodeToString(doc.MD5Sum)
		header.Set("Etag", eTag)
	}

	name, err := doc.Path(c)
	if err != nil {
		return
	}

	content, err := c.FS().Open(name)
	if err != nil {
		return
	}
	defer content.Close()

	http.ServeContent(w, req, doc.Name, doc.UpdatedAt, content)
	return
}

// File represents a file handle. It can be used either for writing OR
// reading, but not both at the same time.
type File struct {
	c  Context       // vfs context
	f  afero.File    // file handle
	fc *fileCreation // file creation handle
}

// fileCreation represents a file open for writing. It is used to
// create of file or to modify the content of a file.
//
// fileCreation implements io.WriteCloser.
type fileCreation struct {
	w         int64     // total size written
	newdoc    *FileDoc  // new document
	olddoc    *FileDoc  // old document if any
	newpath   string    // file new path
	bakpath   string    // backup file path in case of modifying an existing file
	checkHash bool      // whether or not we need the assert the hash is good
	hash      hash.Hash // hash we build up along the file
	err       error     // write error
}

// Open returns a file handle that can be used to read form the file
// specified by the given document.
func Open(c Context, doc *FileDoc) (*File, error) {
	name, err := doc.Path(c)
	if err != nil {
		return nil, err
	}
	f, err := c.FS().Open(name)
	if err != nil {
		return nil, err
	}
	return &File{c, f, nil}, nil
}

// CreateFile is used to create file or modify an existing file
// content. It returns a fileCreation handle. Along with the vfs
// context, it receives the new file document that you want to create.
// It can also receive the old document, representing the current
// revision of the file. In this case it will try to modify the file,
// otherwise it will create it.
//
// Warning: you MUST call the Close() method and check for its error.
// The Close() method will actually create or update the document in
// couchdb. It will also check the md5 hash if required.
func CreateFile(c Context, newdoc, olddoc *FileDoc) (*File, error) {
	now := time.Now()

	newpath, err := newdoc.Path(c)
	if err != nil {
		return nil, err
	}

	var bakpath string
	if olddoc != nil {
		bakpath = fmt.Sprintf("/.%s_%s", olddoc.ID(), olddoc.Rev())
		if err = safeRenameFile(c, newpath, bakpath); err != nil {
			// in case of a concurrent access to this method, it can happend
			// that the file has already been renamed. In this case the
			// safeRenameFile will return an os.ErrNotExist error. But this
			// error is misleading since it does not reflect the conflict.
			if os.IsNotExist(err) {
				err = ErrConflict
			}
			return nil, err
		}
	}

	if olddoc != nil {
		newdoc.SetID(olddoc.ID())
		newdoc.SetRev(olddoc.Rev())
		newdoc.CreatedAt = olddoc.CreatedAt
	} else {
		newdoc.CreatedAt = now
	}

	newdoc.UpdatedAt = now

	f, err := safeCreateFile(newpath, newdoc.Executable, c.FS())
	if err != nil {
		return nil, err
	}

	hash := md5.New() // #nosec

	fc := &fileCreation{
		w: 0,

		newdoc:  newdoc,
		olddoc:  olddoc,
		bakpath: bakpath,
		newpath: newpath,

		checkHash: newdoc.MD5Sum != nil,
		hash:      hash,
	}

	return &File{c, f, fc}, nil
}

// Read bytes from the file into given buffer - part of io.Reader
// This method can be called on read mode only
func (f *File) Read(p []byte) (n int, err error) {
	if f.fc != nil {
		return 0, os.ErrInvalid
	}
	return f.f.Read(p)
}

// Seek into the file - part of io.Reader
// This method can be called on read mode only
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.fc != nil {
		return 0, os.ErrInvalid
	}
	return f.f.Seek(offset, whence)
}

// Write bytes to the file - part of io.WriteCloser
// This method can be called in write mode only
func (f *File) Write(p []byte) (n int, err error) {
	if f.fc == nil {
		return 0, os.ErrInvalid
	}

	n, err = f.f.Write(p)
	if err != nil {
		f.fc.err = err
		return
	}

	f.fc.w += int64(n)

	_, err = f.fc.hash.Write(p)
	return
}

// Close the handle and commit the document in database if all checks
// are OK. It is important to check errors returned by this method.
func (f *File) Close() error {
	if f.fc == nil {
		return f.f.Close()
	}

	var err error
	fc, c := f.fc, f.c

	defer func() {
		werr := fc.err
		if fc.olddoc != nil {
			// put back backup file revision in case on error occured while
			// modifying file content or remove the backup file otherwise
			if err != nil || werr != nil {
				c.FS().Rename(fc.bakpath, fc.newpath)
			} else {
				c.FS().Remove(fc.bakpath)
			}
		} else if err != nil || werr != nil {
			// remove file if an error occured while file creation
			c.FS().Remove(fc.newpath)
		}
	}()

	err = f.f.Close()
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
		err = couchdb.UpdateDoc(c, newdoc)
	} else {
		err = couchdb.CreateDoc(c, newdoc)
	}

	return err
}

// ModifyFileMetadata modify the metadata associated to a file. It can
// be used to rename or move the file in the VFS.
func ModifyFileMetadata(c Context, olddoc *FileDoc, patch *DocPatch) (newdoc *FileDoc, err error) {
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
		err = c.FS().Chmod(newpath, getFileMode(newdoc.Executable))
		if err != nil {
			return
		}
	}

	err = couchdb.UpdateDoc(c, newdoc)
	return
}

// TrashFile is used to delete a file given its document
func TrashFile(c Context, olddoc *FileDoc) (newdoc *FileDoc, err error) {
	oldpath, err := olddoc.Path(c)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(oldpath, TrashDirName) {
		return nil, ErrFileInTrash
	}
	trashFolderID := TrashFolderID
	tryOrUseSuffix(olddoc.Name, "%scozy__%s", func(name string) error {
		newdoc, err = ModifyFileMetadata(c, olddoc, &DocPatch{
			FolderID: &trashFolderID,
			Name:     &name,
		})
		return err
	})
	return
}

func safeCreateFile(name string, executable bool, fs afero.Fs) (afero.File, error) {
	// write only (O_WRONLY), try to create the file and check that it
	// does not already exist (O_CREATE|O_EXCL).
	flag := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	mode := getFileMode(executable)
	return fs.OpenFile(name, flag, mode)
}

func safeRenameFile(c Context, oldpath, newpath string) error {
	newpath = path.Clean(newpath)
	oldpath = path.Clean(oldpath)

	if !path.IsAbs(newpath) || !path.IsAbs(oldpath) {
		return ErrNonAbsolutePath
	}

	_, err := c.FS().Stat(newpath)
	if err == nil {
		return os.ErrExist
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return c.FS().Rename(oldpath, newpath)
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
