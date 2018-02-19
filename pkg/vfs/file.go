package vfs

import (
	// #nosec
	"encoding/base64"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	webutils "github.com/cozy/cozy-stack/web/utils"
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
	DocName string `json:"name"`
	// Parent directory identifier
	DirID       string `json:"dir_id,omitempty"`
	RestorePath string `json:"restore_path,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ByteSize   int64    `json:"size,string"` // Serialized in JSON as a string, because JS has some issues with big numbers
	MD5Sum     []byte   `json:"md5sum"`
	Mime       string   `json:"mime"`
	Class      string   `json:"class"`
	Executable bool     `json:"executable"`
	Trashed    bool     `json:"trashed"`
	Tags       []string `json:"tags"`

	Metadata Metadata `json:"metadata,omitempty"`

	ReferencedBy []couchdb.DocReference `json:"referenced_by,omitempty"`

	// Cache of the fullpath of the file. Should not have to be invalidated
	// since we use FileDoc as immutable data-structures.
	fullpath string

	// NOTE: Do not forget to propagate changes made to this structure to the
	// structure DirOrFileDoc in pkg/vfs/vfs.go.
}

// ID returns the file qualified identifier
func (f *FileDoc) ID() string { return f.DocID }

// Rev returns the file revision
func (f *FileDoc) Rev() string { return f.DocRev }

// DocType returns the file document type
func (f *FileDoc) DocType() string { return consts.Files }

// Clone implements couchdb.Doc
func (f *FileDoc) Clone() couchdb.Doc {
	cloned := *f
	cloned.MD5Sum = make([]byte, len(f.MD5Sum))
	copy(cloned.MD5Sum, f.MD5Sum)
	cloned.Tags = make([]string, len(f.Tags))
	copy(cloned.Tags, f.Tags)
	cloned.ReferencedBy = make([]couchdb.DocReference, len(f.ReferencedBy))
	copy(cloned.ReferencedBy, f.ReferencedBy)
	cloned.Metadata = make(Metadata, len(f.Metadata))
	for k, v := range f.Metadata {
		cloned.Metadata[k] = v
	}
	return &cloned
}

// Reset removes the cached fullpath
func (f *FileDoc) Reset() {
	f.fullpath = ""
}

// SetID changes the file qualified identifier
func (f *FileDoc) SetID(id string) { f.DocID = id }

// SetRev changes the file revision
func (f *FileDoc) SetRev(rev string) { f.DocRev = rev }

// Path is used to generate the file path
func (f *FileDoc) Path(fp FilePather) (string, error) {
	if f.fullpath != "" {
		return f.fullpath, nil
	}
	var err error
	f.fullpath, err = fp.FilePath(f)
	return f.fullpath, err
}

// Parent returns the parent directory document
func (f *FileDoc) Parent(fs VFS) (*DirDoc, error) {
	parent, err := fs.DirByID(f.DirID)
	if os.IsNotExist(err) {
		err = ErrParentDoesNotExist
	}
	return parent, err
}

// Name returns base name of the file
func (f *FileDoc) Name() string { return f.DocName }

// Size returns the length in bytes for regular files; system-dependent for others
func (f *FileDoc) Size() int64 { return f.ByteSize }

// Mode returns the file mode bits
func (f *FileDoc) Mode() os.FileMode { return getFileMode(f.Executable) }

// ModTime returns the modification time
func (f *FileDoc) ModTime() time.Time { return f.UpdatedAt }

// IsDir returns the abbreviation for Mode().IsDir()
func (f *FileDoc) IsDir() bool { return false }

// Sys returns the underlying data source (can return nil)
func (f *FileDoc) Sys() interface{} { return nil }

// AddReferencedBy adds referenced_by to the file
func (f *FileDoc) AddReferencedBy(ri ...couchdb.DocReference) {
	f.ReferencedBy = append(f.ReferencedBy, ri...)
}

func containsReferencedBy(haystack []couchdb.DocReference, needle couchdb.DocReference) bool {
	for _, ref := range haystack {
		if ref.ID == needle.ID && ref.Type == needle.Type {
			return true
		}
	}
	return false
}

// RemoveReferencedBy adds referenced_by to the file
func (f *FileDoc) RemoveReferencedBy(ri ...couchdb.DocReference) {
	// https://github.com/golang/go/wiki/SliceTricks#filtering-without-allocating
	referenced := f.ReferencedBy[:0]
	for _, ref := range f.ReferencedBy {
		if !containsReferencedBy(ri, ref) {
			referenced = append(referenced, ref)
		}
	}
	f.ReferencedBy = referenced
}

// NewFileDoc is the FileDoc constructor. The given name is validated.
func NewFileDoc(name, dirID string, size int64, md5Sum []byte, mime, class string, cdate time.Time, executable, trashed bool, tags []string) (*FileDoc, error) {
	if err := checkFileName(name); err != nil {
		return nil, err
	}

	if dirID == "" {
		dirID = consts.RootDirID
	}

	tags = uniqueTags(tags)

	doc := &FileDoc{
		Type:    consts.FileType,
		DocName: name,
		DirID:   dirID,

		CreatedAt:  cdate,
		UpdatedAt:  cdate,
		ByteSize:   size,
		MD5Sum:     md5Sum,
		Mime:       mime,
		Class:      class,
		Executable: executable,
		Trashed:    trashed,
		Tags:       tags,
	}

	return doc, nil
}

// ServeFileContent replies to a http request using the content of a
// file given its FileDoc.
//
// It uses internally http.ServeContent and benefits from it by
// offering support to Range, If-Modified-Since and If-None-Match
// requests. It uses the revision of the file as the Etag value for
// non-ranged requests
func ServeFileContent(fs VFS, doc *FileDoc, disposition string, req *http.Request, w http.ResponseWriter) error {
	header := w.Header()
	if disposition != "" {
		header.Set("Content-Disposition", ContentDisposition(disposition, doc.DocName))
	}

	if header.Get("Range") == "" {
		eTag := base64.StdEncoding.EncodeToString(doc.MD5Sum)
		if webutils.CheckPreconditions(w, req, eTag) {
			return nil
		}
	}

	content, err := fs.OpenFile(doc)
	if err != nil {
		return err
	}
	defer content.Close()

	webutils.ServeContentRanges(w, req, doc.Mime, doc.Size(), content)
	return nil
}

// ModifyFileMetadata modify the metadata associated to a file. It can
// be used to rename or move the file in the VFS.
func ModifyFileMetadata(fs VFS, olddoc *FileDoc, patch *DocPatch) (*FileDoc, error) {
	var err error
	rename := patch.Name != nil
	cdate := olddoc.CreatedAt
	oname := olddoc.DocName
	trashed := olddoc.Trashed
	if patch.RestorePath != nil {
		trashed = *patch.RestorePath != ""
	}
	patch, err = normalizeDocPatch(&DocPatch{
		Name:        &oname,
		DirID:       &olddoc.DirID,
		RestorePath: &olddoc.RestorePath,
		Tags:        &olddoc.Tags,
		UpdatedAt:   &olddoc.UpdatedAt,
		Executable:  &olddoc.Executable,
	}, patch, cdate)
	if err != nil {
		return nil, err
	}

	// in case of a renaming of the file, if the extension of the file has
	// changed, we consider recalculating the mime and class attributes, using
	// the new extension.
	newname := *patch.Name
	oldname := olddoc.DocName
	var mime, class string
	if patch.Class != nil || (rename && path.Ext(newname) != path.Ext(oldname)) {
		mime, class = ExtractMimeAndClassFromFilename(newname)
	} else {
		mime, class = olddoc.Mime, olddoc.Class
	}

	newdoc, err := NewFileDoc(
		newname,
		*patch.DirID,
		olddoc.Size(),
		olddoc.MD5Sum,
		mime,
		class,
		cdate,
		*patch.Executable,
		trashed,
		*patch.Tags,
	)
	if err != nil {
		return nil, err
	}

	newdoc.RestorePath = *patch.RestorePath
	newdoc.UpdatedAt = *patch.UpdatedAt
	newdoc.Metadata = olddoc.Metadata
	newdoc.ReferencedBy = olddoc.ReferencedBy

	if patch.MD5Sum != nil {
		newdoc.MD5Sum = *patch.MD5Sum
	}

	if err = fs.UpdateFileDoc(olddoc, newdoc); err != nil {
		return nil, err
	}
	return newdoc, nil
}

// TrashFile is used to delete a file given its document
func TrashFile(fs VFS, olddoc *FileDoc) (*FileDoc, error) {
	oldpath, err := olddoc.Path(fs)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(oldpath, TrashDirName) {
		return nil, ErrFileInTrash
	}

	trashDirID := consts.TrashDirID
	restorePath := path.Dir(oldpath)

	var newdoc *FileDoc
	tryOrUseSuffix(olddoc.DocName, conflictFormat, func(name string) error {
		newdoc, err = ModifyFileMetadata(fs, olddoc, &DocPatch{
			DirID:       &trashDirID,
			RestorePath: &restorePath,
			Name:        &name,
		})
		return err
	})
	return newdoc, err
}

// RestoreFile is used to restore a trashed file given its document
func RestoreFile(fs VFS, olddoc *FileDoc) (*FileDoc, error) {
	oldpath, err := olddoc.Path(fs)
	if err != nil {
		return nil, err
	}
	restoreDir, err := getRestoreDir(fs, oldpath, olddoc.RestorePath)
	if err != nil {
		return nil, err
	}
	var newdoc *FileDoc
	var emptyStr string
	name := stripSuffix(olddoc.DocName, conflictSuffix)
	tryOrUseSuffix(name, "%s (%s)", func(name string) error {
		newdoc, err = ModifyFileMetadata(fs, olddoc, &DocPatch{
			DirID:       &restoreDir.DocID,
			RestorePath: &emptyStr,
			Name:        &name,
		})
		return err
	})
	return newdoc, err
}

func getFileMode(executable bool) os.FileMode {
	if executable {
		return 0755 // -rwxr-xr-x
	}
	return 0644 // -rw-r--r--
}

var (
	_ couchdb.Doc = &FileDoc{}
	_ os.FileInfo = &FileDoc{}
)
