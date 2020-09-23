package vfs

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// FsckLogType is the type of a FsckLog
type FsckLogType string

const (
	// IndexMissingRoot is used when the index does not have a root object
	IndexMissingRoot FsckLogType = "index_missing_root"
	// IndexMissingTrash is used when the index does not have a trash folder
	IndexMissingTrash FsckLogType = "index_missing_trash"
	// IndexOrphanTree used when a part of the tree is detached from the main
	// root of the index.
	IndexOrphanTree FsckLogType = "index_orphan_tree"
	// IndexBadFullpath used when a directory does not have the correct path
	// field given its position in the index.
	IndexBadFullpath FsckLogType = "index_bad_fullpath"
	// FSMissing used when a file data is missing on the filesystem from its
	// index entry.
	FSMissing FsckLogType = "filesystem_missing"
	// IndexMissing is used when the index entry is missing from a file data.
	IndexMissing FsckLogType = "index_missing"
	// TypeMismatch is used when a document type does not match in the index and
	// underlying filesystem.
	TypeMismatch FsckLogType = "type_mismatch"
	// ContentMismatch is used when a document content checksum does not match
	// with the one in the underlying fs.
	ContentMismatch FsckLogType = "content_mismatch"
	// FileMissing is used when a version is present for a file that is not in
	// the index.
	FileMissing FsckLogType = "file_missing"
	// IndexFileWithPath is used when a file has a path in the index (only
	// directories should have one).
	IndexFileWithPath = "index_file_with_path"
	// IndexDuplicateName is used when two files or directories have the same
	// name inside the same folder (ie they have the same path).
	IndexDuplicateName = "index_duplicate_name"
	// TrashedNotInTrash is used when a file has trashed: true but its parent
	// directory is not the trash, not is in the trash.
	TrashedNotInTrash = "trashed_not_in_trash"
	// NotTrashedInTrash is used when a file has trashed: false but its parent
	// directory is the trash or a directory in the trash.
	NotTrashedInTrash = "not_trashed_in_trash"
	// ConflictInIndex is used when there is a conflict in CouchDB with 2
	// branches of revisions.
	ConflictInIndex = "conflict_in_index"
	// ThumbnailWithNoFile is used when there is a thumbnail but not the file
	// that was used to create it.
	ThumbnailWithNoFile = "thumbnail_with_no_file"
)

// FsckLog is a struct for an inconsistency in the VFS
type FsckLog struct {
	Type             FsckLogType          `json:"type"`
	FileDoc          *TreeFile            `json:"file_doc,omitempty"`
	DirDoc           *TreeFile            `json:"dir_doc,omitempty"`
	VersionDoc       *Version             `json:"version_doc,omitempty"`
	IsFile           bool                 `json:"is_file"`
	IsVersion        bool                 `json:"is_version"`
	ContentMismatch  *FsckContentMismatch `json:"content_mismatch,omitempty"`
	ExpectedFullpath string               `json:"expected_fullpath,omitempty"`
}

// String returns a string describing the FsckLog
func (f *FsckLog) String() string {
	switch f.Type {
	case IndexMissingRoot:
		return "the root directory is not present in the index"
	case IndexMissingTrash:
		return "the trash directory is not present in the index"
	case IndexOrphanTree:
		if f.IsFile {
			return "the file's parent is missing from the index: orphan file"
		}
		return "the directory's parent is missing from the index: orphan tree"
	case IndexBadFullpath:
		return "the directory does not have the correct path information given its position in the index"
	case FSMissing:
		if f.IsFile {
			return "the file is present in the index but not on the filesystem"
		}
		return "the directory is present in the index but not on the filesystem"
	case IndexMissing:
		return "the document is present on the local filesystem but not in the index"
	case TypeMismatch:
		if f.IsFile {
			return "it's a file in the index but a directory on the filesystem"
		}
		return "it's a directory in the index but a file on the filesystem"
	case ContentMismatch:
		return "the document content does not match the store content checksum"
	case FileMissing:
		return "the document is a version whose file is not present in the index"
	case IndexFileWithPath:
		return "the file document in CouchDB has a path field"
	case IndexDuplicateName:
		return "two documents have the same name inside the same folder"
	case TrashedNotInTrash:
		return "a file document has trashed set to true but its parent is not in the trash"
	case NotTrashedInTrash:
		return "a file document has trashed set tot false but its parent is in the trash"
	case ConflictInIndex:
		return "this document has a conflict in CouchDB between two branches of revisions"
	}
	panic("bad FsckLog type")
}

// FsckContentMismatch is a struct used by the FSCK where CouchDB and Swift
// haven't the same information about a file content (md5sum and size).
type FsckContentMismatch struct {
	SizeIndex   int64  `json:"size_index"`
	SizeFile    int64  `json:"size_file"`
	MD5SumIndex []byte `json:"md5sum_index"`
	MD5SumFile  []byte `json:"md5sum_file"`
}

// Tree is returned by the BuildTree method on the indexes. It contains a
// pointer to the root element of the tree, a map of directories indexed by
// their ID, and a map of a potential list of orphan file or directories
// indexed by their DirID.
type Tree struct {
	Root    *TreeFile
	DirsMap map[string]*TreeFile
	Orphans map[string][]*TreeFile
	Files   map[string]struct{}
}

// TreeFile represent a subset of a file/directory structure that can be used
// in a tree-like representation of the index.
type TreeFile struct {
	DirOrFileDoc
	FilesChildren     []*TreeFile `json:"children,omitempty"`
	FilesChildrenSize int64       `json:"children_size,omitempty"`
	DirsChildren      []*TreeFile `json:"directories,omitempty"`

	IsDir    bool `json:"is_dir"`
	IsOrphan bool `json:"is_orphan"`
	HasCycle bool `json:"has_cycle"`
	HasPath  bool `json:"-"`
}

// AsFile returns the FileDoc part from this more complex struct
func (t *TreeFile) AsFile() *FileDoc {
	if t.IsDir {
		panic("calling AsFile on a directory")
	}
	_, fileDoc := t.DirOrFileDoc.Refine()
	return fileDoc
}

// AsDir returns the DirDoc part from this more complex struct
func (t *TreeFile) AsDir() *DirDoc {
	if !t.IsDir {
		panic("calling AsDir on a file")
	}
	return t.DirDoc.Clone().(*DirDoc)
}

// Clone is part of the couchdb.Doc interface
func (t *TreeFile) Clone() couchdb.Doc {
	panic("TreeFile must not be cloned")
}

var _ couchdb.Doc = &TreeFile{}
