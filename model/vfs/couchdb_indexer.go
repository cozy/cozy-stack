package vfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/prefixer"
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
	err := couchdb.ExecView(c.db, couchdb.DiskUsageView, &couchdb.ViewRequest{
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
	err = couchdb.ExecView(c.db, couchdb.FilesByParentView, &couchdb.ViewRequest{
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
	err := couchdb.ExecView(c.db, couchdb.FilesByParentView, &req, &res)
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
	err := couchdb.ExecView(c.db, couchdb.FilesByParentView, &req, &res)
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
	err := couchdb.ExecView(c.db, couchdb.FilesByParentView, &couchdb.ViewRequest{
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

func (c *couchdbIndexer) CheckIndexIntegrity(accumulate func(*FsckLog)) (err error) {
	tree, err := c.BuildTree()
	if err != nil {
		return
	}

	// cleanDirsMap browse the given root tree recursively into its children
	// directories, removing them from the dirsmap table along the way. In the
	// end, only trees with cycles should stay in the dirsmap.
	cleanDirsMap(tree.Root, tree.DirsMap, accumulate)
	for _, entries := range tree.Orphans {
		for _, f := range entries {
			if f.IsDir {
				cleanDirsMap(f, tree.DirsMap, accumulate)
			}
		}
	}

	for _, orphanCycle := range tree.DirsMap {
		orphanCycle.HasCycle = true
		tree.Orphans[orphanCycle.DirID] = append(tree.Orphans[orphanCycle.DirID], orphanCycle)
	}

	for _, orphansTree := range tree.Orphans {
		for _, orphan := range orphansTree {
			if !orphan.IsDir {
				accumulate(&FsckLog{
					Type:    IndexOrphanTree,
					IsFile:  true,
					FileDoc: orphan,
				})
			} else {
				accumulate(&FsckLog{
					Type:   IndexOrphanTree,
					IsFile: false,
					DirDoc: orphan,
				})
			}
		}
	}

	return
}

func (c *couchdbIndexer) BuildTree(eaches ...func(*TreeFile)) (t *Tree, err error) {
	t = &Tree{
		Root:    nil,
		Orphans: make(map[string][]*TreeFile, 32), // DirID -> *FileDoc
		DirsMap: make(map[string]*TreeFile, 256),  // DocID -> *FileDoc
	}

	// NOTE: the each method is called with objects in no particular order. The
	// only enforcement is that either the Fullpath of the objet is informed or
	// the IsOrphan flag is precised. It may be useful to gather along the way
	// the files without having to browse the whole tree structure.
	var each func(*TreeFile)
	if len(eaches) > 0 {
		each = eaches[0]
	} else {
		each = func(*TreeFile) {}
	}

	err = couchdb.ForeachDocs(c.db, consts.Files, func(_ string, data json.RawMessage) error {
		var f *TreeFile
		if erru := json.Unmarshal(data, &f); erru != nil {
			return erru
		}
		f.IsDir = f.Type == consts.DirType
		if f.DocID == consts.RootDirID {
			t.Root = f
			each(f)
		} else if parent, ok := t.DirsMap[f.DirID]; ok {
			if f.IsDir {
				parent.DirsChildren = append(parent.DirsChildren, f)
			} else {
				parent.FilesChildren = append(parent.FilesChildren, f)
				parent.FilesChildrenSize += f.ByteSize
				f.Fullpath = path.Join(parent.Fullpath, f.DocName)
			}
			each(f)
		} else {
			t.Orphans[f.DirID] = append(t.Orphans[f.DirID], f)
		}
		if f.IsDir {
			if bucket, ok := t.Orphans[f.DocID]; ok {
				for _, child := range bucket {
					if child.IsDir {
						f.DirsChildren = append(f.DirsChildren, child)
					} else {
						f.FilesChildren = append(f.FilesChildren, child)
						f.FilesChildrenSize += child.ByteSize
						child.Fullpath = path.Join(f.Fullpath, child.DocName)
					}
					each(child)
				}
				delete(t.Orphans, f.DocID)
			}
			t.DirsMap[f.DocID] = f
		}
		return nil
	})
	if t.Root == nil {
		return nil, fmt.Errorf("could not find root file")
	}
	for _, bucket := range t.Orphans {
		for _, child := range bucket {
			child.IsOrphan = true
			each(child)
		}
	}
	return
}

func cleanDirsMap(parent *TreeFile, dirsmap map[string]*TreeFile, accumulate func(*FsckLog)) {
	delete(dirsmap, parent.DocID)
	for _, child := range parent.DirsChildren {
		expected := path.Join(parent.Fullpath, child.DocName)
		if expected != child.Fullpath {
			accumulate(&FsckLog{
				Type:             IndexBadFullpath,
				DirDoc:           child,
				ExpectedFullpath: expected,
			})
		}
		cleanDirsMap(child, dirsmap, accumulate)
	}
}
