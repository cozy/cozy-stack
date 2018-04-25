package sharing

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

type bulkRevs struct {
	Rev       string
	Revisions map[string]interface{}
}

type sharingIndexer struct {
	db       couchdb.Database
	indexer  vfs.Indexer
	bulkRevs *bulkRevs
}

// NewSharingIndexer creates an Indexer for the special purpose of the sharing.
// It intercepts some requests to force the id and revisions of some documents,
// and proxifies other requests to the normal couchdbIndexer (reads).
func NewSharingIndexer(inst *instance.Instance, bulkRevs *bulkRevs) vfs.Indexer {
	return &sharingIndexer{
		db:       inst,
		indexer:  vfs.NewCouchdbIndexer(inst),
		bulkRevs: bulkRevs,
	}
}

func (s *sharingIndexer) InitIndex() error {
	return ErrInternalServerError
}

func (s *sharingIndexer) DiskUsage() (int64, error) {
	return s.indexer.DiskUsage()
}

func (s *sharingIndexer) CreateFileDoc(doc *vfs.FileDoc) error {
	return ErrInternalServerError
}

func (s *sharingIndexer) CreateNamedFileDoc(doc *vfs.FileDoc) error {
	return s.UpdateFileDoc(nil, doc)
}

// TODO update io.cozy.shared
func (s *sharingIndexer) UpdateFileDoc(olddoc, doc *vfs.FileDoc) error {
	docs := make([]map[string]interface{}, 1)
	docs[0] = map[string]interface{}{
		"type":       doc.Type,
		"_id":        doc.DocID,
		"name":       doc.DocName,
		"dir_id":     doc.DirID,
		"created_at": doc.CreatedAt,
		"updated_at": doc.UpdatedAt,
		"tags":       doc.Tags,
		"size":       fmt.Sprintf("%d", doc.ByteSize), // XXX size must be serialized as a string, not an int
		"md5Sum":     doc.MD5Sum,
		"mime":       doc.Mime,
		"class":      doc.Class,
		"executable": doc.Executable,
		"trashed":    doc.Trashed,
	}
	if len(doc.ReferencedBy) > 0 {
		docs[0]["referenced_by"] = doc.ReferencedBy
	}
	if s.bulkRevs != nil {
		docs[0]["_rev"] = s.bulkRevs.Rev
		docs[0]["_revisions"] = s.bulkRevs.Revisions
	}
	if err := couchdb.BulkForceUpdateDocs(s.db, consts.Files, docs); err != nil {
		return err
	}
	// Ensure that fullpath is filled because it's used in realtime/@events
	if olddoc != nil {
		if _, err := olddoc.Path(s); err != nil {
			return err
		}
	}
	if _, err := doc.Path(s); err != nil {
		return err
	}
	// TODO the path is missing!
	if olddoc != nil {
		couchdb.RTEvent(s.db, realtime.EventUpdate, doc, olddoc)
	} else {
		couchdb.RTEvent(s.db, realtime.EventUpdate, doc, nil)
	}
	return nil
}

func (s *sharingIndexer) UpdateFileDocs(docs []*vfs.FileDoc) error {
	return ErrInternalServerError
}

func (s *sharingIndexer) DeleteFileDoc(doc *vfs.FileDoc) error {
	return ErrInternalServerError
}

func (s *sharingIndexer) CreateDirDoc(doc *vfs.DirDoc) error {
	return ErrInternalServerError
}

func (s *sharingIndexer) CreateNamedDirDoc(doc *vfs.DirDoc) error {
	return s.UpdateDirDoc(nil, doc)
}

// TODO update io.cozy.shared
func (s *sharingIndexer) UpdateDirDoc(olddoc, doc *vfs.DirDoc) error {
	docs := make([]map[string]interface{}, 1)
	docs[0] = map[string]interface{}{
		"type":       doc.Type,
		"_id":        doc.DocID,
		"name":       doc.DocName,
		"dir_id":     doc.DirID,
		"created_at": doc.CreatedAt,
		"updated_at": doc.UpdatedAt,
		"tags":       doc.Tags,
		"path":       doc.Fullpath,
	}
	if len(doc.ReferencedBy) > 0 {
		docs[0]["referenced_by"] = doc.ReferencedBy
	}
	if s.bulkRevs != nil {
		docs[0]["_rev"] = s.bulkRevs.Rev
		docs[0]["_revisions"] = s.bulkRevs.Revisions
	}
	if err := couchdb.BulkForceUpdateDocs(s.db, consts.Files, docs); err != nil {
		return err
	}
	if olddoc != nil {
		couchdb.RTEvent(s.db, realtime.EventUpdate, doc, olddoc)
	} else {
		couchdb.RTEvent(s.db, realtime.EventUpdate, doc, nil)
	}
	return nil
}

func (s *sharingIndexer) DeleteDirDoc(doc *vfs.DirDoc) error {
	return ErrInternalServerError
}

func (s *sharingIndexer) DeleteDirDocAndContent(doc *vfs.DirDoc, onlyContent bool) (n int64, ids []string, err error) {
	return 0, nil, ErrInternalServerError
}

func (s *sharingIndexer) BatchDelete(docs []couchdb.Doc) error {
	return ErrInternalServerError
}

func (s *sharingIndexer) DirByID(fileID string) (*vfs.DirDoc, error) {
	return s.indexer.DirByID(fileID)
}

func (s *sharingIndexer) DirByPath(name string) (*vfs.DirDoc, error) {
	return s.indexer.DirByPath(name)
}

func (s *sharingIndexer) FileByID(fileID string) (*vfs.FileDoc, error) {
	return s.indexer.FileByID(fileID)
}

func (s *sharingIndexer) FileByPath(name string) (*vfs.FileDoc, error) {
	return s.indexer.FileByPath(name)
}

func (s *sharingIndexer) FilePath(doc *vfs.FileDoc) (string, error) {
	return s.indexer.FilePath(doc)
}

func (s *sharingIndexer) DirOrFileByID(fileID string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	return s.indexer.DirOrFileByID(fileID)
}

func (s *sharingIndexer) DirOrFileByPath(name string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	return s.indexer.DirOrFileByPath(name)
}

func (s *sharingIndexer) DirIterator(doc *vfs.DirDoc, opts *vfs.IteratorOptions) vfs.DirIterator {
	return s.indexer.DirIterator(doc, opts)
}

func (s *sharingIndexer) DirBatch(doc *vfs.DirDoc, cursor couchdb.Cursor) ([]vfs.DirOrFileDoc, error) {
	return s.indexer.DirBatch(doc, cursor)
}

func (s *sharingIndexer) DirLength(doc *vfs.DirDoc) (int, error) {
	return s.indexer.DirLength(doc)
}

func (s *sharingIndexer) DirChildExists(dirID, name string) (bool, error) {
	return s.indexer.DirChildExists(dirID, name)
}

func (s *sharingIndexer) CheckIndexIntegrity() ([]*vfs.FsckLog, error) {
	return nil, ErrInternalServerError
}

var _ vfs.Indexer = (*sharingIndexer)(nil)
