package vfs

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	multierror "github.com/hashicorp/go-multierror"
)

type couchdbIndexer struct {
	db prefixer.Prefixer
}

// NewCouchdbIndexer creates an Indexer instance based on couchdb to store
// files and directories metadata and index them.
func NewCouchdbIndexer(db prefixer.Prefixer) Indexer {
	return &couchdbIndexer{
		db: db,
	}
}

func (c *couchdbIndexer) InitIndex() error {
	createDate := time.Now()
	err := couchdb.CreateNamedDocWithDB(c.db, &DirDoc{
		DocName:   "",
		Type:      consts.DirType,
		DocID:     consts.RootDirID,
		Fullpath:  "/",
		DirID:     "",
		CreatedAt: createDate,
		UpdatedAt: createDate,
	})
	if err != nil {
		return err
	}

	err = couchdb.CreateNamedDocWithDB(c.db, &DirDoc{
		DocName:   path.Base(TrashDirName),
		Type:      consts.DirType,
		DocID:     consts.TrashDirID,
		Fullpath:  TrashDirName,
		DirID:     consts.RootDirID,
		CreatedAt: createDate,
		UpdatedAt: createDate,
	})
	if err != nil && !couchdb.IsConflictError(err) {
		return err
	}
	return nil
}

func (c *couchdbIndexer) DiskUsage() (int64, error) {
	var doc couchdb.ViewResponse
	err := couchdb.ExecView(c.db, consts.DiskUsageView, &couchdb.ViewRequest{
		Reduce: true,
	}, &doc)
	if err != nil {
		return 0, err
	}
	if len(doc.Rows) == 0 {
		return 0, nil
	}
	// Reduce of _count should give us a number value
	f64, ok := doc.Rows[0].Value.(float64)
	if !ok {
		return 0, ErrWrongCouchdbState
	}
	return int64(f64), nil
}

func (c *couchdbIndexer) CreateFileDoc(doc *FileDoc) error {
	// Ensure that fullpath is filled because it's used in realtime/@events
	if _, err := doc.Path(c); err != nil {
		return err
	}
	return couchdb.CreateDoc(c.db, doc)
}

func (c *couchdbIndexer) CreateNamedFileDoc(doc *FileDoc) error {
	// Ensure that fullpath is filled because it's used in realtime/@events
	if _, err := doc.Path(c); err != nil {
		return err
	}
	return couchdb.CreateNamedDoc(c.db, doc)
}

func (c *couchdbIndexer) UpdateFileDoc(olddoc, newdoc *FileDoc) error {
	// Ensure that fullpath is filled because it's used in realtime/@events
	if _, err := olddoc.Path(c); err != nil {
		return err
	}
	if _, err := newdoc.Path(c); err != nil {
		return err
	}
	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	return couchdb.UpdateDocWithOld(c.db, newdoc, olddoc)
}

func (c *couchdbIndexer) DeleteFileDoc(doc *FileDoc) error {
	// Ensure that fullpath is filled because it's used in realtime/@events
	if _, err := doc.Path(c); err != nil {
		return err
	}
	return couchdb.DeleteDoc(c.db, doc)
}

func (c *couchdbIndexer) CreateDirDoc(doc *DirDoc) error {
	return couchdb.CreateDoc(c.db, doc)
}

func (c *couchdbIndexer) CreateNamedDirDoc(doc *DirDoc) error {
	return couchdb.CreateNamedDoc(c.db, doc)
}

func (c *couchdbIndexer) UpdateDirDoc(olddoc, newdoc *DirDoc) error {
	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())

	oldTrashed := strings.HasPrefix(olddoc.Fullpath, TrashDirName)
	newTrashed := strings.HasPrefix(newdoc.Fullpath, TrashDirName)

	isRestored := oldTrashed && !newTrashed
	isTrashed := !oldTrashed && newTrashed

	if isTrashed {
		if err := c.setTrashedForFilesInsideDir(olddoc, true); err != nil {
			return err
		}
	}

	if newdoc.Fullpath != olddoc.Fullpath {
		if err := c.moveDir(olddoc.Fullpath, newdoc.Fullpath); err != nil {
			return err
		}
	}

	if err := couchdb.UpdateDocWithOld(c.db, newdoc, olddoc); err != nil {
		return err
	}

	if isRestored {
		if err := c.setTrashedForFilesInsideDir(newdoc, false); err != nil {
			return err
		}
	}

	return nil
}

func (c *couchdbIndexer) DeleteDirDoc(doc *DirDoc) error {
	return couchdb.DeleteDoc(c.db, doc)
}

func (c *couchdbIndexer) DeleteDirDocAndContent(doc *DirDoc, onlyContent bool) (n int64, ids []string, err error) {
	var files []couchdb.Doc
	if !onlyContent {
		files = append(files, doc)
	}
	err = walk(c, doc.Name(), doc, nil, func(name string, dir *DirDoc, file *FileDoc, err error) error {
		if err != nil {
			return err
		}
		if dir != nil {
			if dir.ID() == doc.ID() {
				return nil
			}
			files = append(files, dir)
		} else {
			files = append(files, file)
			ids = append(ids, file.ID())
			n += file.ByteSize
		}
		return err
	}, 0)
	if err == nil {
		err = c.BatchDelete(files)
	}
	return
}

func (c *couchdbIndexer) BatchDelete(docs []couchdb.Doc) error {
	return couchdb.BulkDeleteDocs(c.db, consts.Files, docs)
}

func (c *couchdbIndexer) moveDir(oldpath, newpath string) error {
	limit := 256
	var children []*DirDoc
	docs := make([]interface{}, 0, limit)
	olddocs := make([]interface{}, 0, limit)

	for {
		sel := mango.StartWith("path", oldpath+"/")
		req := &couchdb.FindRequest{
			UseIndex: "dir-by-path",
			Selector: sel,
			Skip:     0,
			Limit:    limit,
		}
		err := couchdb.FindDocs(c.db, consts.Files, req, &children)
		if err != nil {
			return err
		}
		if len(children) == 0 {
			break
		}
		for _, child := range children {
			cloned := child.Clone()
			olddocs = append(olddocs, cloned)
			child.Fullpath = path.Join(newpath, child.Fullpath[len(oldpath)+1:])
			docs = append(docs, child)
		}
		if err = couchdb.BulkUpdateDocs(c.db, consts.Files, docs, olddocs); err != nil {
			return err
		}
		if len(children) < limit {
			break
		}
		children = children[:0]
		docs = docs[:0]
		olddocs = olddocs[:0]
	}

	return nil
}

func (c *couchdbIndexer) DirByID(fileID string) (*DirDoc, error) {
	doc := &DirDoc{}
	err := couchdb.GetDoc(c.db, consts.Files, fileID, doc)
	if couchdb.IsNotFoundError(err) {
		err = os.ErrNotExist
	}
	if err != nil {
		if fileID == consts.RootDirID {
			return nil, errors.New("Root directory is not in database")
		}
		if fileID == consts.TrashDirID {
			return nil, errors.New("Trash directory is not in database")
		}
		return nil, err
	}
	if doc.Type != consts.DirType {
		return nil, os.ErrNotExist
	}
	return doc, nil
}

func (c *couchdbIndexer) DirByPath(name string) (*DirDoc, error) {
	if !path.IsAbs(name) {
		return nil, ErrNonAbsolutePath
	}
	var docs []*DirDoc
	sel := mango.Equal("path", path.Clean(name))
	req := &couchdb.FindRequest{
		UseIndex: "dir-by-path",
		Selector: sel,
		Limit:    1,
	}
	err := couchdb.FindDocs(c.db, consts.Files, req, &docs)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		if name == "/" {
			return nil, errors.New("Root directory is not in database")
		}
		return nil, os.ErrNotExist
	}
	return docs[0], nil
}

func (c *couchdbIndexer) FileByID(fileID string) (*FileDoc, error) {
	doc := &FileDoc{}
	err := couchdb.GetDoc(c.db, consts.Files, fileID, doc)
	if couchdb.IsNotFoundError(err) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	if doc.Type != consts.FileType {
		return nil, os.ErrNotExist
	}
	return doc, nil
}

func (c *couchdbIndexer) FileByPath(name string) (*FileDoc, error) {
	if !path.IsAbs(name) {
		return nil, ErrNonAbsolutePath
	}
	parent, err := c.DirByPath(path.Dir(name))
	if err != nil {
		return nil, err
	}

	// consts.FilesByParentView keys are [parentID, type, name]
	var res couchdb.ViewResponse
	err = couchdb.ExecView(c.db, consts.FilesByParentView, &couchdb.ViewRequest{
		Key:         []string{parent.DocID, consts.FileType, path.Base(name)},
		IncludeDocs: true,
	}, &res)
	if err != nil {
		return nil, err
	}

	if len(res.Rows) == 0 {
		return nil, os.ErrNotExist
	}

	var fdoc FileDoc
	err = json.Unmarshal(res.Rows[0].Doc, &fdoc)
	return &fdoc, err
}

func (c *couchdbIndexer) FilePath(doc *FileDoc) (string, error) {
	var parentPath string
	if doc.DirID == consts.RootDirID {
		parentPath = "/"
	} else if doc.DirID == consts.TrashDirID {
		parentPath = TrashDirName
	} else {
		parent, err := c.DirByID(doc.DirID)
		if err != nil {
			return "", ErrParentDoesNotExist
		}
		parentPath = parent.Fullpath
	}
	return path.Join(parentPath, doc.DocName), nil
}

func (c *couchdbIndexer) DirOrFileByID(fileID string) (*DirDoc, *FileDoc, error) {
	dirOrFile := &DirOrFileDoc{}
	err := couchdb.GetDoc(c.db, consts.Files, fileID, dirOrFile)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			err = os.ErrNotExist
		}
		return nil, nil, err
	}
	dirDoc, fileDoc := dirOrFile.Refine()
	return dirDoc, fileDoc, nil
}

func (c *couchdbIndexer) DirOrFileByPath(name string) (*DirDoc, *FileDoc, error) {
	dirDoc, err := c.DirByPath(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err == nil {
		return dirDoc, nil, nil
	}
	fileDoc, err := c.FileByPath(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err == nil {
		return nil, fileDoc, nil
	}
	return nil, nil, err
}

func (c *couchdbIndexer) DirIterator(doc *DirDoc, opts *IteratorOptions) DirIterator {
	return NewIterator(c.db, doc, opts)
}

func (c *couchdbIndexer) DirBatch(doc *DirDoc, cursor couchdb.Cursor) ([]DirOrFileDoc, error) {
	// consts.FilesByParentView keys are [parentID, type, name]
	req := couchdb.ViewRequest{
		StartKey:    []string{doc.DocID, ""},
		EndKey:      []string{doc.DocID, couchdb.MaxString},
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse
	cursor.ApplyTo(&req)
	err := couchdb.ExecView(c.db, consts.FilesByParentView, &req, &res)
	if err != nil {
		return nil, err
	}
	cursor.UpdateFrom(&res)

	docs := make([]DirOrFileDoc, len(res.Rows))
	for i, row := range res.Rows {
		var doc DirOrFileDoc
		err := json.Unmarshal(row.Doc, &doc)
		if err != nil {
			return nil, err
		}
		docs[i] = doc
	}

	return docs, nil
}

func (c *couchdbIndexer) DirLength(doc *DirDoc) (int, error) {
	req := couchdb.ViewRequest{
		StartKey:   []string{doc.DocID, ""},
		EndKey:     []string{doc.DocID, couchdb.MaxString},
		Reduce:     true,
		GroupLevel: 1,
	}
	var res couchdb.ViewResponse
	err := couchdb.ExecView(c.db, consts.FilesByParentView, &req, &res)
	if err != nil {
		return 0, err
	}

	if len(res.Rows) == 0 {
		return 0, nil
	}

	// Reduce of _count should give us a number value
	f64, ok := res.Rows[0].Value.(float64)
	if !ok {
		return 0, ErrWrongCouchdbState
	}
	return int(f64), nil
}

func (c *couchdbIndexer) DirChildExists(dirID, name string) (bool, error) {
	var res couchdb.ViewResponse

	// consts.FilesByParentView keys are [parentID, type, name]
	err := couchdb.ExecView(c.db, consts.FilesByParentView, &couchdb.ViewRequest{
		Keys: []interface{}{
			[]string{dirID, consts.FileType, name},
			[]string{dirID, consts.DirType, name},
		},
		Reduce: true,
		Group:  true,
	}, &res)
	if err != nil {
		return false, err
	}

	if len(res.Rows) == 0 {
		return false, nil
	}

	// Reduce of _count should give us a number value
	f64, ok := res.Rows[0].Value.(float64)
	if !ok {
		return false, ErrWrongCouchdbState
	}
	return int(f64) > 0, nil
}

func (c *couchdbIndexer) setTrashedForFilesInsideDir(doc *DirDoc, trashed bool) error {
	var files, olddocs []interface{}
	parent := doc
	err := walk(c, doc.Name(), doc, nil, func(name string, dir *DirDoc, file *FileDoc, err error) error {
		if dir != nil {
			parent = dir
		}
		if file != nil && file.Trashed != trashed {
			// Fullpath is used by event triggers and should be pre-filled here
			cloned := file.Clone().(*FileDoc)
			fullpath := path.Join(parent.Fullpath, file.DocName)
			fullpath = strings.TrimPrefix(fullpath, TrashDirName)
			if trashed {
				cloned.fullpath = fullpath
				file.fullpath = TrashDirName + fullpath
			} else {
				cloned.fullpath = TrashDirName + fullpath
				file.fullpath = fullpath
			}
			file.Trashed = trashed
			files = append(files, file)
			olddocs = append(olddocs, cloned)
		}
		return err
	}, 0)
	if err != nil {
		return err
	}
	return couchdb.BulkUpdateDocs(c.db, consts.Files, files, olddocs)
}

// TreeFile represent a subset of a file/directory structure that can be used
// in a tree-like representation of the index.
type TreeFile struct {
	DirOrFileDoc
	FilesChildren     []*TreeFile `json:"children,omitempty"`
	FilesChildrenSize int64       `json:"children_size,omitempty"`
	DirsChildren      []*TreeFile `json:"directories,omitempty"`

	isDir    bool
	hasCycle bool
	visited  bool
	errs     []*treeError
}

// Clone is part of the couchdb.Doc interface
func (t *TreeFile) Clone() couchdb.Doc {
	panic("TreeFile must not be cloned")
}

var _ couchdb.Doc = &TreeFile{}

type treeError struct {
	file *TreeFile
	path string
}

func (c *couchdbIndexer) CheckIndexIntegrity() (logs []*FsckLog, err error) {
	root, orphans, err := checkIndexIntegrity(func(cb func(entry *TreeFile)) error {
		return couchdb.ForeachDocs(c.db, consts.Files, func(_ string, data json.RawMessage) error {
			var f TreeFile
			if err = json.Unmarshal(data, &f); err == nil {
				cb(&f)
			}
			return err
		})
	})
	if err != nil {
		return
	}
	if root == nil {
		logs = append(logs, &FsckLog{Type: IndexMissingRoot})
		return
	}

	logs, err = getLocalTreeLogs(c, root)
	if err != nil {
		return
	}

	for dirID, orphansTree := range orphans {
		for _, entry := range orphansTree {
			var log *FsckLog
			log, err = getOrphanTreeLog(c, dirID, entry)
			if err != nil {
				return
			}
			logs = append(logs, log)
		}
	}

	return
}

func (c *couchdbIndexer) BuildTree() (root *TreeFile, err error) {
	orphans := make(map[string][]*TreeFile, 32)
	dirsmap := make(map[string]*TreeFile, 256)

	err = couchdb.ForeachDocs(c.db, consts.Files, func(_ string, data json.RawMessage) error {
		var f TreeFile
		if erru := json.Unmarshal(data, &f); erru != nil {
			return erru
		}
		f.isDir = f.Type == consts.DirType
		if f.DocID == consts.RootDirID {
			root = &f
		} else if parent, ok := dirsmap[f.DirID]; ok {
			if f.isDir {
				parent.DirsChildren = append(parent.DirsChildren, &f)
			} else {
				parent.FilesChildren = append(parent.FilesChildren, &f)
				parent.FilesChildrenSize += f.ByteSize
				f.Fullpath = path.Join(parent.Fullpath, f.DocName)
			}
		} else {
			orphans[f.DirID] = append(orphans[f.DirID], &f)
		}
		if f.isDir {
			if bucket, ok := orphans[f.DocID]; ok {
				for _, child := range bucket {
					if child.isDir {
						f.DirsChildren = append(f.DirsChildren, child)
					} else {
						f.FilesChildren = append(f.FilesChildren, child)
						f.FilesChildrenSize += child.ByteSize
						child.Fullpath = path.Join(f.Fullpath, child.DocName)
					}
				}
				delete(orphans, f.DocID)
			}
			dirsmap[f.DocID] = &f
		}
		return nil
	})
	return
}

func checkIndexIntegrity(generator func(func(entry *TreeFile)) error) (root *TreeFile, orphans map[string][]*TreeFile, err error) {
	orphans = make(map[string][]*TreeFile, 32)
	dirsmap := make(map[string]*TreeFile, 256)

	err = generator(func(f *TreeFile) {
		f.isDir = f.Type == consts.DirType
		if f.DocID == consts.RootDirID {
			root = f
		} else if parent, ok := dirsmap[f.DirID]; ok {
			if f.isDir {
				parent.DirsChildren = append(parent.DirsChildren, f)
			} else {
				parent.FilesChildren = append(parent.FilesChildren, f)
				parent.FilesChildrenSize += f.ByteSize
			}
		} else {
			orphans[f.DirID] = append(orphans[f.DirID], f)
		}
		if f.isDir {
			if bucket, ok := orphans[f.DocID]; ok {
				for _, child := range bucket {
					if child.isDir {
						f.DirsChildren = append(f.DirsChildren, child)
					} else {
						f.FilesChildren = append(f.FilesChildren, child)
						f.FilesChildrenSize += child.ByteSize
					}
				}
				delete(orphans, f.DocID)
			}
			dirsmap[f.DocID] = f
		}
	})
	if err != nil || root == nil {
		return
	}

	root.errs = reduceTree(root, dirsmap, nil)
	delete(dirsmap, consts.RootDirID)

	for _, entries := range orphans {
		for _, f := range entries {
			if f.isDir {
				f.errs = reduceTree(f, dirsmap, nil)
				delete(dirsmap, f.DocID)
			}
		}
	}

	for _, orphanCycle := range dirsmap {
		orphanCycle.hasCycle = true
		orphans[orphanCycle.DirID] = append(orphans[orphanCycle.DirID], orphanCycle)
	}

	return
}

func reduceTree(parent *TreeFile, dirsmap map[string]*TreeFile, errs []*treeError) []*treeError {
	delete(dirsmap, parent.DocID)
	for _, child := range parent.DirsChildren {
		expected := path.Join(parent.Fullpath, child.DocName)
		if expected != child.Fullpath {
			errs = append(errs, &treeError{
				file: child,
				path: expected,
			})
		}
		errs = reduceTree(child, dirsmap, errs)
	}
	return errs
}

func getLocalTreeLogs(c *couchdbIndexer, root *TreeFile) (logs []*FsckLog, errm error) {
	for _, e := range root.errs {
		if e.path != "" {
			olddoc, err := c.DirByID(e.file.DocID)
			if err != nil {
				errm = multierror.Append(errm, err)
				continue
			}
			newdoc := olddoc.Clone().(*DirDoc)
			newdoc.Fullpath = e.path
			logs = append(logs, &FsckLog{
				Type:      IndexBadFullpath,
				OldDirDoc: olddoc,
				DirDoc:    newdoc,
				Filename:  e.path,
			})
		}
	}
	return
}

func getOrphanTreeLog(c *couchdbIndexer, dirID string, orphan *TreeFile) (log *FsckLog, err error) {
	// TODO: For now, we re-attach the orphan trees at the root of the user's
	// directory. We may use the dirID to infer more precisely where we should
	// re-attach this orphan tree. However we should be careful and check if this
	// directory is actually attached to the root.
	log = &FsckLog{}
	log.Type = IndexOrphanTree
	if !orphan.isDir {
		var olddoc *FileDoc
		olddoc, err = c.FileByID(orphan.DocID)
		if err != nil {
			return
		}
		log.IsFile = true
		log.FileDoc = olddoc
	} else {
		var olddoc *DirDoc
		olddoc, err = c.DirByID(orphan.DocID)
		if err != nil {
			return
		}
		log.DirDoc = olddoc
		log.Filename = olddoc.Fullpath
		if orphan.hasCycle || strings.HasPrefix(olddoc.Fullpath, TrashDirName) {
			log.Deletions = listChildren(orphan, nil)
			return
		}
	}
	return
}

func listChildren(root *TreeFile, files []couchdb.Doc) []couchdb.Doc {
	if !root.visited {
		files = append(files, root)
		// avoid stackoverflow on cycles
		root.visited = true
		for _, child := range root.DirsChildren {
			files = listChildren(child, files)
		}
		for _, child := range root.FilesChildren {
			files = append(files, child)
		}
	}
	return files
}
