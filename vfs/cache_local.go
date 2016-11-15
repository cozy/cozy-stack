package vfs

import (
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/cozy/cozy-stack/lru"
	"golang.org/x/sync/errgroup"
)

// LocalCache implements the VFS Cache interface and should be used
// for mono-stack usage, where only one cozy-stack is accessing to the
// VFS at a time.
//
// Internally it provides some optimisations to cache file attributes
// and avoid having multiple useless RTTs with CouchDB.
type LocalCache struct {
	mud  sync.RWMutex       // mutex for directories data-structures
	lrud *lru.Cache         // lru cache for directories
	pthd map[string]*string // path directory to id map

	muf  sync.RWMutex       // mutex for files data-structures
	lruf *lru.Cache         // lru cache for files
	pthf map[string]*string // (folderID, name) file pair to id map
}

// NewLocalCache creates an new LocalCache. The maxEntries parameter
// is used to specified the cache size: how many files and directories
// elements are kept in-memory
func NewLocalCache(maxEntries int) *LocalCache {
	lc := new(LocalCache)
	lc.init(maxEntries)
	return lc
}

func (lc *LocalCache) init(maxEntries int) {
	dirEviction := func(key string, value interface{}) {
		if doc, ok := value.(*DirDoc); ok {
			delete(lc.pthd, doc.Fullpath)
		}
	}

	fileEviction := func(key string, value interface{}) {
		if doc, ok := value.(*FileDoc); ok {
			delete(lc.pthf, genFilePathID(doc.FolderID, doc.Name))
		}
	}

	lc.pthd = make(map[string]*string)
	lc.pthf = make(map[string]*string)
	lc.lrud = &lru.Cache{MaxEntries: maxEntries, OnEvicted: dirEviction}
	lc.lruf = &lru.Cache{MaxEntries: maxEntries, OnEvicted: fileEviction}
}

// CreateDir is be used to persist a directory document
func (lc *LocalCache) CreateDir(c Context, doc *DirDoc) error {
	var err error
	if err = doc.calcPath(c); err != nil {
		return err
	}
	err = couchdb.CreateDoc(c, doc)
	if err != nil {
		return err
	}
	lc.tapDir(doc)
	return nil
}

// UpdateDir is used to update a persisted directory document
func (lc *LocalCache) UpdateDir(c Context, olddoc, newdoc *DirDoc) error {
	oldpath, err := olddoc.Path(c)
	if err != nil {
		return err
	}

	if err = lc.updateDirDoc(c, newdoc); err != nil {
		return err
	}

	newpath, err := newdoc.Path(c)
	if err != nil {
		return err
	}

	if oldpath == newpath {
		return nil
	}

	var children []*DirDoc
	req := &couchdb.FindRequest{
		Selector: mango.StartWith("path", oldpath+"/"),
	}

	err = couchdb.FindDocs(c, FsDocType, req, &children)
	if err != nil || len(children) == 0 {
		return err
	}

	newchildren := make([]*DirDoc, len(children))
	for i, child := range children {
		newchild, err := NewDirDoc(
			child.Name,
			child.FolderID,
			child.Tags,
		)
		if err != nil {
			return err
		}

		newchild.SetID(child.ID())
		newchild.SetRev(child.Rev())
		newchild.Fullpath = path.Join(newpath, child.Fullpath[len(oldpath)+1:])
		newchildren[i] = newchild
	}

	// @TODO use couchdb bulk updates instead
	var g errgroup.Group
	for _, child := range newchildren {
		newchild := child
		g.Go(func() error {
			return lc.updateDirDoc(c, newchild)
		})
	}
	return g.Wait()
}

func (lc *LocalCache) updateDirDoc(c Context, doc *DirDoc) error {
	if err := doc.calcPath(c); err != nil {
		return err
	}
	if err := couchdb.UpdateDoc(c, doc); err != nil {
		lc.rmDir(doc)
		return err
	}
	lc.tapDir(doc)
	return nil
}

// DirByID is used to fetch a directory given its ID
func (lc *LocalCache) DirByID(c Context, fileID string) (doc *DirDoc, err error) {
	var ok bool
	if doc, ok = lc.dirCachedByID(fileID); ok {
		return
	}

	doc = &DirDoc{}
	err = couchdb.GetDoc(c, FsDocType, fileID, doc)
	if couchdb.IsNotFoundError(err) {
		err = ErrParentDoesNotExist
	} else if err == nil && doc.Type != DirType {
		err = os.ErrNotExist
	}
	if err != nil {
		return
	}

	lc.tapDir(doc)
	return
}

// DirByPath is used to fetch a directory given its path
func (lc *LocalCache) DirByPath(c Context, name string) (doc *DirDoc, err error) {
	var ok bool
	if doc, ok = lc.dirCachedByPath(name); ok {
		return
	}

	var docs []*DirDoc
	sel := mango.Equal("path", path.Clean(name))
	req := &couchdb.FindRequest{Selector: sel, Limit: 1}
	err = couchdb.FindDocs(c, FsDocType, req, &docs)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		if name == "/" {
			panic("Root folder is not in database")
		}
		return nil, os.ErrNotExist
	}
	doc = docs[0]

	lc.tapDir(doc)
	return
}

// DirFiles is used to fetch directory children (files and
// directories)
func (lc *LocalCache) DirFiles(c Context, doc *DirDoc) (files []*FileDoc, dirs []*DirDoc, err error) {
	var docs []*DirOrFileDoc
	sel := mango.Equal("folder_id", doc.ID())
	req := &couchdb.FindRequest{Selector: sel, Limit: 10}
	err = couchdb.FindDocs(c, FsDocType, req, &docs)
	if err != nil {
		return
	}

	for _, doc := range docs {
		dir, file := doc.Refine()
		if dir != nil {
			lc.tapDir(dir)
			dirs = append(dirs, dir)
		} else {
			lc.tapFile(file)
			files = append(files, file)
		}
	}

	return
}

// CreateFile is be used to persist a file document
func (lc *LocalCache) CreateFile(c Context, doc *FileDoc) error {
	err := couchdb.CreateDoc(c, doc)
	if err != nil {
		return err
	}
	lc.tapFile(doc)
	return nil
}

// UpdateFile is used to update a persisted file document
func (lc *LocalCache) UpdateFile(c Context, doc *FileDoc) error {
	err := couchdb.UpdateDoc(c, doc)
	if err != nil {
		lc.rmFile(doc)
		return err
	}
	lc.tapFile(doc)
	return nil
}

// FileByID is used to fetch a file given its ID
func (lc *LocalCache) FileByID(c Context, fileID string) (doc *FileDoc, err error) {
	var ok bool
	if doc, ok = lc.fileCachedByID(fileID); ok {
		return
	}

	doc = &FileDoc{}
	err = couchdb.GetDoc(c, FsDocType, fileID, doc)
	if err != nil {
		return nil, err
	}

	if doc.Type != FileType {
		return nil, os.ErrNotExist
	}

	return doc, nil
}

// FileByPath is used to fetch a file given its path
func (lc *LocalCache) FileByPath(c Context, name string) (doc *FileDoc, err error) {
	dirpath := path.Dir(name)
	parent, err := lc.DirByPath(c, dirpath)
	if err != nil {
		return
	}

	folderID, filename := parent.ID(), path.Base(name)

	var ok bool
	if doc, ok = lc.fileCachedByFolderID(folderID, filename); ok {
		return
	}

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
		return
	}
	if len(docs) == 0 {
		if name == "/" {
			panic("Root folder is not in database")
		}
		err = os.ErrNotExist
		return
	}

	doc = docs[0]
	lc.tapFile(doc)
	return
}

// DirOrFileByID is used to fetch a directory or file given its ID
func (lc *LocalCache) DirOrFileByID(c Context, fileID string) (dirDoc *DirDoc, fileDoc *FileDoc, err error) {
	var ok bool
	if dirDoc, ok = lc.dirCachedByID(fileID); ok {
		return
	}

	if fileDoc, ok = lc.fileCachedByID(fileID); ok {
		return
	}

	dirOrFile := &DirOrFileDoc{}
	err = couchdb.GetDoc(c, FsDocType, fileID, dirOrFile)
	if err != nil {
		return
	}

	dirDoc, fileDoc = dirOrFile.Refine()
	return
}

// Len returns the total number of elements currently cached
func (lc *LocalCache) Len() int {
	lc.mud.RLock()
	lc.muf.RLock()
	defer lc.mud.RUnlock()
	defer lc.muf.RUnlock()
	return lc.lrud.Len() + lc.lruf.Len()
}

func (lc *LocalCache) tapDir(doc *DirDoc) {
	lc.mud.Lock()
	defer lc.mud.Unlock()
	key := doc.DocID
	if olddoc, ok := lc.lrud.Get(key); ok {
		delete(lc.pthd, olddoc.(*DirDoc).Fullpath)
	}
	lc.lrud.Add(key, doc)
	lc.pthd[doc.Fullpath] = &doc.DocID
}

func (lc *LocalCache) tapFile(doc *FileDoc) {
	lc.muf.Lock()
	defer lc.muf.Unlock()
	key := doc.DocID
	if olddoc, ok := lc.lruf.Get(key); ok {
		f := olddoc.(*FileDoc)
		delete(lc.pthf, genFilePathID(f.FolderID, f.Name))
	}
	lc.lruf.Add(key, doc)
	lc.pthf[genFilePathID(doc.FolderID, doc.Name)] = &doc.DocID
}

func (lc *LocalCache) rmDir(doc *DirDoc) {
	lc.mud.Lock()
	defer lc.mud.Unlock()
	lc.lrud.Remove(doc.DocID)
}

func (lc *LocalCache) rmFile(doc *FileDoc) {
	lc.muf.Lock()
	defer lc.muf.Unlock()
	lc.lruf.Remove(doc.DocID)
}

func (lc *LocalCache) dirCachedByID(fileID string) (*DirDoc, bool) {
	lc.mud.Lock()
	defer lc.mud.Unlock()
	if v, ok := lc.lrud.Get(fileID); ok {
		return v.(*DirDoc), true
	}
	return nil, false
}

func (lc *LocalCache) dirCachedByPath(name string) (*DirDoc, bool) {
	lc.mud.Lock()
	defer lc.mud.Unlock()
	pid, ok := lc.pthd[name]
	if ok {
		v, _ := lc.lrud.Get(*pid)
		return v.(*DirDoc), true
	}
	return nil, false
}

func (lc *LocalCache) fileCachedByID(fileID string) (*FileDoc, bool) {
	lc.muf.Lock()
	defer lc.muf.Unlock()
	if v, ok := lc.lruf.Get(fileID); ok {
		return v.(*FileDoc), true
	}
	return nil, false
}

func (lc *LocalCache) fileCachedByFolderID(folderID, name string) (*FileDoc, bool) {
	lc.muf.Lock()
	defer lc.muf.Unlock()
	pid, ok := lc.pthf[genFilePathID(folderID, name)]
	if ok {
		v, _ := lc.lruf.Get(*pid)
		return v.(*FileDoc), true
	}
	return nil, false
}

func genFilePathID(folderID, name string) string {
	return fmt.Sprintf("%s/%s", folderID, name)
}

// check if LocalCache implements the Cache interface
var _ Cache = &LocalCache{}
