package vfs

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// FsckLogType is the type of a FsckLog
type FsckLogType string

const (
	// IndexMissingRoot is used when the index does not have a root object
	IndexMissingRoot FsckLogType = "index_missing_root"
	// IndexOrphanTree used when a part of the tree is detached from the main
	// root of the index.
	IndexOrphanTree FsckLogType = "index_orphan_tree"
	// IndexBadFullpath used when a directory does not have the correct path
	// field given its position in the index.
	IndexBadFullpath FsckLogType = "index_bad_fullpath"
	// FileMissing used when a file data is missing from its index entry.
	FileMissing FsckLogType = "file_missing"
	// IndexMissing is used when the index entry is missing from a file data.
	IndexMissing FsckLogType = "index_missing"
	// TypeMismatch is used when a document type does not match in the index and
	// underlying filesystem.
	TypeMismatch FsckLogType = "type_mismatch"
	// ContentMismatch is used when a document content checksum does not match
	// with the one in the underlying fs.
	ContentMismatch FsckLogType = "content_mismatch"
)

// FsckLog is a struct for an inconsistency in the VFS
type FsckLog struct {
	Type             FsckLogType          `json:"type"`
	FileDoc          *TreeFile            `json:"file_doc,omitempty"`
	DirDoc           *TreeFile            `json:"dir_doc,omitempty"`
	IsFile           bool                 `json:"is_file"`
	ContentMismatch  *FsckContentMismatch `json:"content_mismatch,omitempty"`
	ExpectedFullpath string               `json:"expected_fullpath,omitempty"`
}

// String returns a string describing the FsckLog
func (f *FsckLog) String() string {
	switch f.Type {
	case IndexMissingRoot:
		return "the root directory is not present in the index"
	case IndexOrphanTree:
		if f.IsFile {
			return "the file's parent is missing from the index: orphan file"
		}
		return "the directory's parent is missing from the index: orphan tree"
	case IndexBadFullpath:
		return "the directory does not have the correct path information given its position in the index"
	case FileMissing:
		if f.IsFile {
			return "the file is present in the index but not on the filesystem"
		}
		return "the directory is present in the index but not on the filesystem"
	case TypeMismatch:
		if f.IsFile {
			return "it's a file in the index but a directory on the filesystem"
		}
		return "it's a directory in the index but a file on the filesystem"
	case IndexMissing:
		return "the document is present on the local filesystem but not in the index"
	case ContentMismatch:
		return "the document content does not match the store content checksum"
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
