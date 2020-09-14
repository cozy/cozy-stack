package vfs

import (
	"encoding/json"
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

// DiskUsage returns the total size of the files (current + old versions).
func (c *couchdbIndexer) DiskUsage() (int64, error) {
	used, err := c.FilesUsage()
	if err != nil {
		return 0, err
	}

	if versions, err := c.VersionsUsage(); err == nil {
		used += versions
	}

	return used, nil
}

// FilesUsage returns the files total size (without versions)
func (c *couchdbIndexer) FilesUsage() (int64, error) {
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
	used, ok := doc.Rows[0].Value.(float64)
	if !ok {
		return 0, ErrWrongCouchdbState
	}

	return int64(used), nil
}

// VersionsUsage returns the total size for old file versions (not including
// current version).
func (c *couchdbIndexer) VersionsUsage() (int64, error) {
	var doc couchdb.ViewResponse
	err := couchdb.ExecView(c.db, couchdb.OldVersionsDiskUsageView, &couchdb.ViewRequest{
		Reduce: true,
	}, &doc)
	if err != nil {
		return 0, err
	}
	if len(doc.Rows) == 0 {
		return 0, nil
	}
	// Reduce of _count should give us a number value
	used, ok := doc.Rows[0].Value.(float64)
	if !ok {
		return 0, ErrWrongCouchdbState
	}

	return int64(used), nil
}

// TrashUsage returns the space taken by the files in the trash.
func (c *couchdbIndexer) TrashUsage() (int64, error) {
	var size int64
	trash, err := c.DirByID(consts.TrashDirID)
	if err != nil {
		return 0, err
	}
	err = walk(c, trash.Name(), trash, nil, func(_ string, _ *DirDoc, file *FileDoc, err error) error {
		if file != nil {
			size += file.ByteSize
		}
		return err
	}, 0)
	return size, err
}

func (c *couchdbIndexer) prepareFileDoc(doc *FileDoc) error {
	// Ensure that fullpath is filled because it's used in realtime/@events
	if _, err := doc.Path(c); err != nil {
		return err
	}
	// If a valid datetime is extracted from the EXIF metadata, use it as the
	// created_at of the file. By valid, we mean that we filter out photos
	// taken on camera were the clock was never configured (e.g. 1970-01-01).
	if date, ok := doc.Metadata["datetime"].(time.Time); ok && date.Year() > 1990 {
		doc.CreatedAt = date
		if doc.UpdatedAt.Before(date) {
			doc.UpdatedAt = date
		}
	}
	return nil
}

func (c *couchdbIndexer) CreateFileDoc(doc *FileDoc) error {
	if err := c.prepareFileDoc(doc); err != nil {
		return err
	}
	return couchdb.CreateDoc(c.db, doc)
}

func (c *couchdbIndexer) CreateNamedFileDoc(doc *FileDoc) error {
	if err := c.prepareFileDoc(doc); err != nil {
		return err
	}
	return couchdb.CreateNamedDoc(c.db, doc)
}

func (c *couchdbIndexer) UpdateFileDoc(olddoc, newdoc *FileDoc) error {
	if err := c.prepareFileDoc(newdoc); err != nil {
		return err
	}

	fullpath, err := olddoc.Path(c)
	if err != nil {
		return err
	}
	if strings.HasPrefix(fullpath, TrashDirName) {
		c.checkTrashedFileIsShared(newdoc)
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
		c.checkTrashedDirIsShared(newdoc)
		if err := c.setTrashedForFilesInsideDir(olddoc, true); err != nil {
			return err
		}
	}

	if newdoc.Fullpath != olddoc.Fullpath {
		if err := c.MoveDir(olddoc.Fullpath, newdoc.Fullpath); err != nil {
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

func (c *couchdbIndexer) DeleteDirDocAndContent(doc *DirDoc, onlyContent bool) (files []*FileDoc, n int64, err error) {
	var docs []couchdb.Doc
	if !onlyContent {
		docs = append(docs, doc)
	}
	err = walk(c, doc.Name(), doc, nil, func(name string, dir *DirDoc, file *FileDoc, err error) error {
		if err != nil {
			return err
		}
		if dir != nil {
			if dir.ID() == doc.ID() {
				return nil
			}
			docs = append(docs, dir.Clone())
		} else {
			cloned := file.Clone()
			docs = append(docs, cloned)
			files = append(files, cloned.(*FileDoc))
			n += file.ByteSize
		}
		return err
	}, 0)
	if err == nil {
		err = c.BatchDelete(docs)
	}
	return
}

func (c *couchdbIndexer) BatchDelete(docs []couchdb.Doc) error {
	remaining := docs[:]
	for len(remaining) > 0 {
		n := 1000
		if len(remaining) < n {
			n = len(remaining)
		}
		toDelete := remaining[:n]
		remaining = remaining[n:]
		if err := couchdb.BulkDeleteDocs(c.db, consts.Files, toDelete); err != nil {
			// If it fails once, try again
			if err := couchdb.BulkDeleteDocs(c.db, consts.Files, toDelete); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *couchdbIndexer) MoveDir(oldpath, newpath string) error {
	if oldpath+"/" == newpath {
		return nil
	}

	limit := 256
	var children []*DirDoc
	docs := make([]interface{}, 0, limit)
	olddocs := make([]interface{}, 0, limit)
	isTrashed := strings.HasPrefix(newpath, TrashDirName)

	// We limit the stack to 128 bulk updates to avoid infinite loops, as we
	// had a case in the past.
	start := oldpath + "/"
	stop := oldpath + "0" // 0 is the next ascii character after /
	for i := 0; i < 128; i++ {
		// The simple selector mango.StartWith can have some issues when
		// renaming a folder to the same name, but with a different unicode
		// normalization (like NFC to NFD). In that case, CouchDB would always
		// return the same documents with this selector, as it does the
		// comparison on a normalized string.
		sel := mango.And(
			mango.Gt("path", start),
			mango.Lt("path", stop),
		)
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
		start = children[len(children)-1].Fullpath
		for _, child := range children {
			// XXX In theory, only the directories have a path, but in
			// practice, it's safer to check it as we had already some bugs in
			// the VFS.
			if child.Type != consts.DirType {
				continue
			}
			// XXX We can have documents that are not a child of the moved dir
			// because of the comparison of strings used by CouchDB:
			// /Photos/ < /PHOTOS/AAA < /Photos/bbb < /Photos0
			// So, we need to skip the documents that are not really the children.
			// Cf http://docs.couchdb.org/en/stable/ddocs/views/collation.html#collation-specification
			if !strings.HasPrefix(child.Fullpath, oldpath) {
				continue
			}
			cloned := child.Clone()
			if isTrashed {
				c.checkTrashedDirIsShared(cloned.(*DirDoc))
			}
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
		return nil, os.ErrNotExist
	}
	if err != nil {
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
		return nil, os.ErrNotExist
	}
	if docs[0].Type != consts.DirType {
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
	if fdoc.Type != consts.FileType {
		return nil, os.ErrNotExist
	}
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
	if err == nil {
		return dirDoc, nil, nil
	}
	if !os.IsNotExist(err) {
		return nil, nil, err
	}
	fileDoc, err := c.FileByPath(name)
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
				c.checkTrashedFileIsShared(cloned)
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

func (c *couchdbIndexer) CreateVersion(v *Version) error {
	return couchdb.CreateNamedDocWithDB(c.db, v)
}

func (c *couchdbIndexer) DeleteVersion(v *Version) error {
	return couchdb.DeleteDoc(c.db, v)
}

func (c *couchdbIndexer) AllVersions() ([]*Version, error) {
	var versions []*Version
	err := couchdb.ForeachDocs(c.db, consts.FilesVersions, func(_id string, doc json.RawMessage) error {
		var v Version
		if err := json.Unmarshal(doc, &v); err != nil {
			return err
		}
		versions = append(versions, &v)
		return nil
	})
	return versions, err
}

func (c *couchdbIndexer) BatchDeleteVersions(versions []*Version) error {
	remaining := make([]couchdb.Doc, len(versions))
	for i, v := range versions {
		remaining[i] = v
	}
	for len(remaining) > 0 {
		n := 1000
		if len(remaining) < n {
			n = len(remaining)
		}
		toDelete := remaining[:n]
		remaining = remaining[n:]
		if err := couchdb.BulkDeleteDocs(c.db, consts.Files, toDelete); err != nil {
			// If it fails once, try again
			if err := couchdb.BulkDeleteDocs(c.db, consts.Files, toDelete); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkTrashedDirIsShared will look for a dir going to the trash if it was the
// main dir of a sharing. If it is the case, the sharing is revoked and the
// reference to the sharing is removed.
func (c *couchdbIndexer) checkTrashedDirIsShared(doc *DirDoc) {
	refs := doc.ReferencedBy[:0]
	for _, ref := range doc.ReferencedBy {
		if ref.Type == consts.Sharings {
			c.revokeSharing(ref.ID)
		} else {
			refs = append(refs, ref)
		}
	}
	doc.ReferencedBy = refs
}

// checkTrashedFileIsShared will look for a file going to the trash if it was
// the main file of a sharing. If it is the case, the sharing is revoked and
// the reference to the sharing is removed.
func (c *couchdbIndexer) checkTrashedFileIsShared(doc *FileDoc) {
	refs := doc.ReferencedBy[:0]
	for _, ref := range doc.ReferencedBy {
		if ref.Type == consts.Sharings {
			c.revokeSharing(ref.ID)
		} else {
			refs = append(refs, ref)
		}
	}
	doc.ReferencedBy = refs
}

// RevokeSharingFunc does nothing. It will will be overridden from the sharing
// package.
var RevokeSharingFunc = func(db prefixer.Prefixer, sharingID string) {}

// revokeSharing is called when the main file/dir of a sharing is trashed, in
// order to revoke the sharing.
//
// Note: we don't want to rely on triggers/jobs, as they are asynchronous and
// if they are executed a bit late, the trashing of the files inside the shared
// directory can be replicated to the other members before the sharing is
// revoked.
//
// Technically, it isn't straightforward, as we are in the VFS and we can't
// call functions in the sharing module (it would cause cyclic import error).
// And, we don't have an instance, just a CouchDB indexer. So, it is a bit
// hackish.
func (c *couchdbIndexer) revokeSharing(sharingID string) {
	RevokeSharingFunc(c.db, sharingID)
}

func (c *couchdbIndexer) CheckIndexIntegrity(accumulate func(*FsckLog), failFast bool) error {
	tree, err := c.BuildTree()
	if err != nil {
		return err
	}
	if err := c.CheckTreeIntegrity(tree, accumulate, failFast); err != nil && err != ErrFsckFailFast {
		return err
	}
	return nil
}

func (c *couchdbIndexer) CheckTreeIntegrity(tree *Tree, accumulate func(*FsckLog), failFast bool) error {
	if err := c.checkNoConflicts(accumulate, failFast); err != nil {
		if err == ErrFsckFailFast {
			return nil
		}
		return err
	}

	if tree.Root == nil {
		accumulate(&FsckLog{Type: IndexMissingRoot})
		return nil
	}

	if _, ok := tree.DirsMap[consts.TrashDirID]; !ok {
		accumulate(&FsckLog{Type: IndexMissingTrash})
		if failFast {
			return nil
		}
	}

	// cleanDirsMap browse the given root tree recursively into its children
	// directories, removing them from the dirsmap table along the way. In the
	// end, only trees with cycles should stay in the dirsmap.
	if ok := cleanDirsMap(tree.Root, tree.DirsMap, accumulate, failFast); !ok {
		return nil
	}
	for _, entries := range tree.Orphans {
		for _, f := range entries {
			if f.IsDir {
				if ok := cleanDirsMap(f, tree.DirsMap, accumulate, failFast); !ok {
					return nil
				}
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
			if failFast {
				return nil
			}
		}
	}

	return couchdb.ForeachDocs(c.db, consts.FilesVersions, func(_ string, data json.RawMessage) error {
		v := &Version{}
		if erru := json.Unmarshal(data, v); erru != nil {
			return erru
		}
		fileID := strings.SplitN(v.DocID, "/", 2)[0]
		if _, ok := tree.Files[fileID]; !ok {
			accumulate(&FsckLog{
				Type:       FileMissing,
				IsVersion:  true,
				VersionDoc: v,
			})
		}
		return nil
	})
}

func (c *couchdbIndexer) BuildTree(eaches ...func(*TreeFile)) (t *Tree, err error) {
	t = &Tree{
		Root:    nil,
		Orphans: make(map[string][]*TreeFile, 32), // DirID -> *FileDoc
		DirsMap: make(map[string]*TreeFile, 256),  // DocID -> *FileDoc
		Files:   make(map[string]struct{}, 1024),  // DocID -> ∅
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
				if f.Fullpath == "" {
					f.Fullpath = path.Join(parent.Fullpath, f.DocName)
				} else {
					f.HasPath = true
				}
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
						if child.Fullpath == "" {
							child.Fullpath = path.Join(f.Fullpath, child.DocName)
						} else {
							child.HasPath = true
						}
					}
					each(child)
				}
				delete(t.Orphans, f.DocID)
			}
			t.DirsMap[f.DocID] = f
		} else {
			t.Files[f.DocID] = struct{}{}
		}
		return nil
	})
	for _, bucket := range t.Orphans {
		for _, child := range bucket {
			child.IsOrphan = true
			each(child)
		}
	}
	return
}

func cleanDirsMap(
	parent *TreeFile,
	dirsmap map[string]*TreeFile,
	accumulate func(*FsckLog),
	failFast bool,
) bool {
	delete(dirsmap, parent.DocID)
	names := make(map[string]struct{})
	inTrash := strings.HasPrefix(parent.Fullpath, TrashDirName)

	for _, file := range parent.FilesChildren {
		if inTrash != file.Trashed {
			if file.Trashed {
				accumulate(&FsckLog{
					Type:    TrashedNotInTrash,
					FileDoc: file,
				})
			} else {
				accumulate(&FsckLog{
					Type:    NotTrashedInTrash,
					FileDoc: file,
				})
			}
			if failFast {
				return false
			}
		}

		if file.HasPath {
			accumulate(&FsckLog{
				Type:    IndexFileWithPath,
				FileDoc: file,
			})
			if failFast {
				return false
			}
		}
		if _, ok := names[file.DocName]; ok {
			accumulate(&FsckLog{
				Type:    IndexDuplicateName,
				FileDoc: file,
			})
			if failFast {
				return false
			}
		}
		names[file.DocName] = struct{}{}
	}

	for _, child := range parent.DirsChildren {
		if _, ok := names[child.DocName]; ok {
			accumulate(&FsckLog{
				Type:   IndexDuplicateName,
				DirDoc: child,
			})
			if failFast {
				return false
			}
		}
		names[child.DocName] = struct{}{}
		expected := path.Join(parent.Fullpath, child.DocName)
		if expected != child.Fullpath {
			accumulate(&FsckLog{
				Type:             IndexBadFullpath,
				DirDoc:           child,
				ExpectedFullpath: expected,
			})
			if failFast {
				return false
			}
		}
		if ok := cleanDirsMap(child, dirsmap, accumulate, failFast); !ok {
			return false
		}
	}
	return true
}

func (c *couchdbIndexer) checkNoConflicts(accumulate func(*FsckLog), failFast bool) error {
	var docs []DirOrFileDoc
	req := &couchdb.FindRequest{
		UseIndex:  "with-conflicts",
		Selector:  mango.Exists("_conflicts"),
		Limit:     1000,
		Conflicts: true,
	}
	_, err := couchdb.FindDocsRaw(c.db, consts.Files, req, &docs)
	if err != nil {
		return err
	}
	for _, doc := range docs {
		if doc.Type == consts.DirType {
			accumulate(&FsckLog{
				Type:   ConflictInIndex,
				DirDoc: &TreeFile{DirOrFileDoc: doc},
			})
		} else {
			accumulate(&FsckLog{
				Type:    ConflictInIndex,
				FileDoc: &TreeFile{DirOrFileDoc: doc},
				IsFile:  true,
			})
		}
		if failFast {
			return ErrFsckFailFast
		}
	}
	return nil
}
