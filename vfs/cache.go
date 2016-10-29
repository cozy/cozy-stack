package vfs

// Cache is a type used to provide a common interface for VFS data
// layer. The VFS will always go through the specified cache to access
// file or directory attributes.
//
// It can implement a simple local wrapper of the CouchDB package, a
// simple abstraction to avoid using CouchDB or a more complex package
// to handle VFS synchronization in a horizontally scaled
// architecture.
type Cache interface {
	CreateDir(c Context, doc *DirDoc) error
	UpdateDir(c Context, olddoc, newdoc *DirDoc) error
	DirByID(c Context, fileID string) (*DirDoc, error)
	DirByPath(c Context, name string) (*DirDoc, error)
	DirFiles(c Context, doc *DirDoc) (files []*FileDoc, dirs []*DirDoc, err error)

	CreateFile(c Context, doc *FileDoc) error
	UpdateFile(c Context, doc *FileDoc) error
	FileByID(c Context, fileID string) (*FileDoc, error)
	FileByPath(c Context, name string) (*FileDoc, error)

	DirOrFileByID(c Context, fileID string) (*DirDoc, *FileDoc, error)

	Len() int
}
