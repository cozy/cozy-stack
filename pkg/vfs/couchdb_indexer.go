package vfs

import (
	"encoding/json"
	"os"
	"path"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
)

type couchdbIndexer struct {
	db couchdb.Database
}

// NewCouchdbIndexer creates an Indexer instance based on couchdb to store
// files and directories metadata and index them.
func NewCouchdbIndexer(db couchdb.Database) Indexer {
	return &couchdbIndexer{
		db: db,
	}
}

func (c *couchdbIndexer) InitIndex() error {
	err := couchdb.CreateNamedDocWithDB(c.db, &DirDoc{
		DocName:  "",
		Type:     consts.DirType,
		DocID:    consts.RootDirID,
		Fullpath: "/",
		DirID:    "",
	})
	if err != nil {
		return err
	}

	err = couchdb.CreateNamedDocWithDB(c.db, &DirDoc{
		DocName:  path.Base(TrashDirName),
		Type:     consts.DirType,
		DocID:    consts.TrashDirID,
		Fullpath: TrashDirName,
		DirID:    consts.RootDirID,
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
	return couchdb.UpdateDoc(c.db, newdoc)
}

func (c *couchdbIndexer) UpdateFileDocs(docs []*FileDoc) error {
	if len(docs) == 0 {
		return nil
	}
	// Ensure that fullpath is filled because it's used in realtime/@events
	couchdocs := make([]interface{}, len(docs))
	for i, doc := range docs {
		if _, err := doc.Path(c); err != nil {
			return err
		}
		couchdocs[i] = doc
	}
	return couchdb.BulkUpdateDocs(c.db, consts.Files, couchdocs)
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
	if newdoc.Fullpath != olddoc.Fullpath {
		if err := c.moveDir(olddoc.Fullpath, newdoc.Fullpath); err != nil {
			return err
		}
	}
	return couchdb.UpdateDoc(c.db, newdoc)
}

func (c *couchdbIndexer) DeleteDirDoc(doc *DirDoc) error {
	return couchdb.DeleteDoc(c.db, doc)
}

func (c *couchdbIndexer) moveDir(oldpath, newpath string) error {
	var children []*DirDoc
	sel := mango.StartWith("path", oldpath+"/")
	req := &couchdb.FindRequest{
		UseIndex: "dir-by-path",
		Selector: sel,
	}
	err := couchdb.FindDocs(c.db, consts.Files, req, &children)
	if err != nil || len(children) == 0 {
		return err
	}

	couchdocs := make([]interface{}, len(children))
	for i, child := range children {
		child.Fullpath = path.Join(newpath, child.Fullpath[len(oldpath)+1:])
		couchdocs[i] = child
	}
	return couchdb.BulkUpdateDocs(c.db, consts.Files, couchdocs)
}

func (c *couchdbIndexer) DirByID(fileID string) (*DirDoc, error) {
	doc := &DirDoc{}
	err := couchdb.GetDoc(c.db, consts.Files, fileID, doc)
	if couchdb.IsNotFoundError(err) {
		err = os.ErrNotExist
	}
	if err != nil {
		if fileID == consts.RootDirID {
			panic("Root directory is not in database")
		}
		if fileID == consts.TrashDirID {
			panic("Trash directory is not in database")
		}
		return nil, err
	}
	if doc.Type != consts.DirType {
		return nil, os.ErrNotExist
	}
	return doc, err
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
			panic("Root directory is not in database")
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
	err = json.Unmarshal(*res.Rows[0].Doc, &fdoc)
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
		err := json.Unmarshal(*row.Doc, &doc)
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
