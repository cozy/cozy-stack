package sharing

import (
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
	multierror "github.com/hashicorp/go-multierror"
)

// isTrashed returns true for a file or folder inside the trash
func isTrashed(doc couchdb.JSONDoc) bool {
	if doc.Type != consts.Files {
		return false
	}
	if doc.Get("type") == consts.FileType {
		return doc.Get("trashed") == true
	}
	return strings.HasPrefix(doc.Get("path").(string), vfs.TrashDirName+"/")
}

// MakeXorKey generates a key for transforming the file identifiers
func MakeXorKey() []byte {
	random := crypto.GenerateRandomBytes(8)
	result := make([]byte, 2*len(random))
	for i, val := range random {
		result[2*i] = val & 0xf
		result[2*i+1] = val >> 4
	}
	return result
}

// XorID transforms the identifier of a file to a new identifier, in a
// reversible way: it makes a XOR on the hexadecimal characters
func XorID(id string, key []byte) string {
	l := len(key)
	buf := []byte(id)
	for i, c := range buf {
		switch {
		case '0' <= c && c <= '9':
			c = (c - '0') ^ key[i%l]
		case 'a' <= c && c <= 'f':
			c = (c - 'a' + 10) ^ key[i%l]
		case 'A' <= c && c <= 'F':
			c = (c - 'A' + 10) ^ key[i%l]
		default:
			continue
		}
		if c < 10 {
			buf[i] = c + '0'
		} else {
			buf[i] = (c - 10) + 'a'
		}
	}
	return string(buf)
}

// SortFilesToSent sorts the files slice that will be sent in bulk_docs:
// - directories must come before files (if a file is created in a new
//   directory, we must create directory before the file)
// - directories are sorted by increasing depth (if a sub-folder is created
//   in a new directory, we must create the parent before the child)
func (s *Sharing) SortFilesToSent(files []map[string]interface{}) {
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if a["type"] == consts.FileType {
			return false
		}
		if b["type"] == consts.FileType {
			return true
		}
		if removed, ok := a["_deleted"].(bool); ok && removed {
			return true
		}
		if removed, ok := b["_deleted"].(bool); ok && removed {
			return false
		}
		p, ok := a["path"].(string)
		if !ok {
			return true
		}
		q, ok := b["path"].(string)
		if !ok {
			return false
		}
		return strings.Count(p, "/") < strings.Count(q, "/")
	})
}

// TransformFileToSent transforms an io.cozy.files document before sending it
// to another cozy instance:
// - its identifier is XORed
// - its dir_id is XORed or removed
// - the path is removed (directory only)
//
// ruleIndexes is a map of "doctype-docid" -> rule index
// TODO keep referenced_by that are relevant to this sharing
func (s *Sharing) TransformFileToSent(doc map[string]interface{}, xorKey []byte, ruleIndex int) map[string]interface{} {
	if doc["type"] == consts.DirType {
		delete(doc, "path")
	}
	id := doc["_id"].(string)
	doc["_id"] = XorID(id, xorKey)
	dir, ok := doc["dir_id"].(string)
	if !ok {
		return doc
	}
	delete(doc, "referenced_by")
	rule := s.Rules[ruleIndex]
	noDirID := rule.Selector == "referenced_by"
	if !noDirID {
		for _, v := range rule.Values {
			if v == dir {
				noDirID = true
				break
			}
		}
	}
	if noDirID {
		delete(doc, "dir_id")
	} else {
		doc["dir_id"] = XorID(dir, xorKey)
	}
	return doc
}

// EnsureSharedWithMeDir returns the shared-with-me directory, and create it if
// it doesn't exist
func EnsureSharedWithMeDir(inst *instance.Instance) (*vfs.DirDoc, error) {
	fs := inst.VFS()
	dir, _, err := fs.DirOrFileByID(consts.SharedWithMeDirID)
	if err != nil && err != os.ErrNotExist {
		return nil, err
	}

	if dir == nil {
		name := inst.Translate("Tree Shared with me")
		dir, err = vfs.NewDirDocWithPath(name, consts.RootDirID, "/", nil)
		dir.DocID = consts.SharedWithMeDirID
		if err != nil {
			return nil, err
		}
		if err = fs.CreateDir(dir); err != nil {
			return nil, err
		}
		return dir, nil
	}

	if dir.RestorePath != "" {
		_, err = vfs.RestoreDir(fs, dir)
		if err != nil {
			return nil, err
		}
		children, err := fs.DirBatch(dir, &couchdb.SkipCursor{})
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			d, f := child.Refine()
			if d != nil {
				_, err = vfs.TrashDir(fs, d)
			} else {
				_, err = vfs.TrashFile(fs, f)
			}
			if err != nil {
				return nil, err
			}
		}
	}

	return dir, nil
}

// CreateDirForSharing creates the directory where files for this sharing will
// be put. This directory will be initially inside the Shared with me folder.
func (s *Sharing) CreateDirForSharing(inst *instance.Instance, rule *Rule) error {
	parent, err := EnsureSharedWithMeDir(inst)
	if err != nil {
		return err
	}
	fs := inst.VFS()
	dir, err := vfs.NewDirDocWithParent(rule.Title, parent, []string{"from-sharing-" + s.SID})
	dir.DocID = rule.Values[0]
	if err != nil {
		return err
	}
	dir.AddReferencedBy(couchdb.DocReference{
		ID:   s.SID,
		Type: consts.Sharings,
	})
	return fs.CreateDir(dir)
}

// AddReferenceForSharingDir adds a reference to the sharing on the sharing directory
func (s *Sharing) AddReferenceForSharingDir(inst *instance.Instance, rule *Rule) error {
	fs := inst.VFS()
	dir, err := fs.DirByID(rule.Values[0])
	if err != nil {
		return err
	}
	for _, ref := range dir.ReferencedBy {
		if ref.Type == consts.Sharings && ref.ID == s.SID {
			return nil
		}
	}
	olddoc := dir.Clone().(*vfs.DirDoc)
	dir.AddReferencedBy(couchdb.DocReference{
		ID:   s.SID,
		Type: consts.Sharings,
	})
	return fs.UpdateDirDoc(olddoc, dir)
}

// GetSharingDir returns the directory used by this sharing for putting files
// and folders that have no dir_id.
func (s *Sharing) GetSharingDir(inst *instance.Instance) (*vfs.DirDoc, error) {
	key := []string{consts.Sharings, s.SID}
	end := []string{key[0], key[1], couchdb.MaxString}
	req := &couchdb.ViewRequest{
		StartKey:    key,
		EndKey:      end,
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse
	err := couchdb.ExecView(inst, consts.FilesReferencedByView, req, &res)
	if err != nil || len(res.Rows) == 0 {
		inst.Logger().WithField("nspace", "sharing").
			Warnf("Sharing dir not found: %v (%s)", err, s.SID)
		return nil, ErrInternalServerError
	}
	return inst.VFS().DirByID(res.Rows[0].ID)
}

// ApplyBulkFiles takes a list of documents for the io.cozy.files doctype and
// will apply changes to the VFS according to those documents.
func (s *Sharing) ApplyBulkFiles(inst *instance.Instance, docs DocsList) error {
	var errm error
	fs := inst.VFS()

	for _, target := range docs {
		id, ok := target["_id"].(string)
		if !ok {
			errm = multierror.Append(errm, ErrMissingID)
			continue
		}
		ref := &SharedRef{}
		err := couchdb.GetDoc(inst, consts.Shared, consts.Files+"/"+id, ref)
		if err != nil {
			if !couchdb.IsNotFoundError(err) {
				inst.Logger().WithField("nspace", "replicator").
					Debugf("Error on finding doc of bulk files: %s", err)
				errm = multierror.Append(errm, err)
				continue
			}
			ref = nil
		}
		dir, file, err := fs.DirOrFileByID(id)
		if err != nil && err != os.ErrNotExist {
			inst.Logger().WithField("nspace", "replicator").
				Debugf("Error on finding ref of bulk files: %s", err)
			errm = multierror.Append(errm, err)
			continue
		}
		if _, ok := target["_deleted"]; ok {
			if ref == nil || (dir == nil && file == nil) {
				continue
			}
			if dir != nil {
				err = s.TrashDir(inst, dir)
			} else {
				err = s.TrashFile(inst, file)
			}
			if err != nil {
				errm = multierror.Append(errm, err)
			}
		} else if ref == nil && dir == nil {
			err = s.CreateDir(inst, target)
			if err != nil {
				errm = multierror.Append(errm, err)
			}
			// TODO update the io.cozy.shared reference?
		} else if ref == nil {
			// TODO be safe => return an error
			continue
		} else if dir == nil {
			// TODO manage the conflict: doc was deleted/moved outside the
			// sharing on this cozy and updated on the other cozy
			continue
		} else {
			err = s.UpdateDir(inst, target, dir)
			if err != nil {
				errm = multierror.Append(errm, err)
			}
		}
	}
	return nil
}

func copySafeFieldsToFile(target, file *vfs.FileDoc) {
	file.Tags = make([]string, len(target.Tags))
	copy(file.Tags, target.Tags)
	file.CreatedAt = target.CreatedAt
	file.UpdatedAt = target.UpdatedAt
	file.Mime = target.Mime
	file.Class = target.Class
	file.Executable = target.Executable
}

func copySafeFieldsToDir(target map[string]interface{}, dir *vfs.DirDoc) {
	if tags, ok := target["tags"].([]interface{}); ok {
		dir.Tags = make([]string, 0, len(tags))
		for _, tag := range tags {
			if t, ok := tag.(string); ok {
				dir.Tags = append(dir.Tags, t)
			}
		}
	}
	if created, ok := target["created_at"].(string); ok {
		if at, err := time.Parse(time.RFC3339Nano, created); err == nil {
			dir.CreatedAt = at
		}
	}
	if updated, ok := target["updated_at"].(string); ok {
		if at, err := time.Parse(time.RFC3339Nano, updated); err == nil {
			dir.UpdatedAt = at
		}
	}
}

// CreateDir creates a directory on this cozy to reflect a change on another
// cozy instance of this sharing.
func (s *Sharing) CreateDir(inst *instance.Instance, target map[string]interface{}) error {
	name, ok := target["name"].(string)
	if !ok {
		inst.Logger().WithField("nspace", "replicator").
			Debugf("Missing name for creating dir: %#v", target)
		return ErrInternalServerError
	}
	rev, ok := target["_rev"].(string)
	if !ok {
		inst.Logger().WithField("nspace", "replicator").
			Debugf("Missing _rev for creating dir: %#v", target)
		return ErrInternalServerError
	}
	revisions, ok := target["_revisions"].(map[string]interface{})
	if !ok {
		inst.Logger().WithField("nspace", "replicator").
			Debugf("Missing _revisions for creating dir: %#v", target)
		return ErrInternalServerError
	}
	indexer := NewSharingIndexer(inst, &bulkRevs{
		Rev:       rev,
		Revisions: revisions,
	})
	fs := inst.VFS().UseSharingIndexer(indexer)

	var parent *vfs.DirDoc
	var err error
	if dirID, ok := target["dir_id"].(string); ok {
		parent, err = fs.DirByID(dirID)
		// TODO better handling of this conflict
		if err != nil {
			inst.Logger().WithField("nspace", "replicator").
				Debugf("Conflict for parent on creating dir: %s", err)
			return err
		}
	} else {
		parent, err = s.GetSharingDir(inst)
		if err != nil {
			return err
		}
	}

	dir, err := vfs.NewDirDocWithParent(name, parent, nil)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").
			Warnf("Cannot initialize dir doc: %s", err)
		return err
	}
	dir.SetID(target["_id"].(string))
	copySafeFieldsToDir(target, dir)
	// TODO referenced_by
	// TODO manage conflicts
	if err := fs.CreateDir(dir); err != nil {
		inst.Logger().WithField("nspace", "replicator").
			Debugf("Cannot create dir: %s", err)
		return err
	}
	return nil
}

// UpdateDir updates a directory on this cozy to reflect a change on another
// cozy instance of this sharing.
func (s *Sharing) UpdateDir(inst *instance.Instance, target map[string]interface{}, dir *vfs.DirDoc) error {
	rev, ok := target["_rev"].(string)
	if !ok {
		inst.Logger().WithField("nspace", "replicator").
			Warnf("Missing _rev for updating directory %#v", target)
		return ErrInternalServerError
	}
	revisions, ok := target["_revisions"].(map[string]interface{})
	if !ok {
		inst.Logger().WithField("nspace", "replicator").
			Warnf("Missing _revisions for updating directory %#v", target)
		return ErrInternalServerError
	}
	oldDoc := dir.Clone().(*vfs.DirDoc)
	indexer := NewSharingIndexer(inst, &bulkRevs{
		Rev:       rev,
		Revisions: revisions,
	})
	fs := inst.VFS().UseSharingIndexer(indexer)

	name, ok := target["name"].(string)
	if !ok {
		inst.Logger().WithField("nspace", "replicator").
			Warnf("Missing name for updating directory %#v", target)
		return ErrInternalServerError
	}
	dir.DocName = name
	if dirID, ok := target["dir_id"].(string); ok {
		if dirID != dir.DirID {
			parent, err := fs.DirByID(dirID)
			// TODO better handling of this conflict
			if err != nil {
				inst.Logger().WithField("nspace", "replicator").
					Debugf("Conflict for parent on updating dir: %s", err)
				return err
			}
			dir.DirID = parent.DocID
			dir.Fullpath = path.Join(parent.Fullpath, dir.DocName)
		} else {
			dir.Fullpath = path.Join(path.Dir(dir.Fullpath), dir.DocName)
		}
	} else {
		parent, err := s.GetSharingDir(inst)
		if err != nil {
			return err
		}
		dir.DirID = parent.DocID
		dir.Fullpath = path.Join(parent.Fullpath, dir.DocName)
	}

	copySafeFieldsToDir(target, dir)
	inst.Logger().WithField("nspace", "replicator").
		Debugf("Update dir: %#v", dir)
	// TODO referenced_by
	// TODO manage conflicts
	return fs.UpdateDirDoc(oldDoc, dir)
}

// TrashDir puts the directory in the trash
// TODO conflicts
func (s *Sharing) TrashDir(inst *instance.Instance, dir *vfs.DirDoc) error {
	if strings.HasPrefix(dir.Fullpath+"/", vfs.TrashDirName+"/") {
		// nothing to do if the directory is already in the trash
		return nil
	}
	_, err := vfs.TrashDir(inst.VFS(), dir)
	return err
}

// TrashFile puts the file in the trash
// TODO if file has references, we should keep it in a special folder
// TODO conflicts
func (s *Sharing) TrashFile(inst *instance.Instance, file *vfs.FileDoc) error {
	if file.Trashed {
		// nothing to do if the directory is already in the trash
		return nil
	}
	_, err := vfs.TrashFile(inst.VFS(), file)
	return err
}

// TODO referenced_by
func dirToJSONDoc(dir *vfs.DirDoc) couchdb.JSONDoc {
	doc := couchdb.JSONDoc{
		Type: consts.Files,
		M: map[string]interface{}{
			"type":       dir.Type,
			"_id":        dir.DocID,
			"_rev":       dir.DocRev,
			"name":       dir.DocName,
			"created_at": dir.CreatedAt,
			"updated_at": dir.UpdatedAt,
			"tags":       dir.Tags,
			"path":       dir.Fullpath,
		},
	}
	if dir.DirID != "" {
		doc.M["dir_id"] = dir.DirID
	}
	if dir.RestorePath != "" {
		doc.M["restore_path"] = dir.RestorePath
	}
	return doc
}

// TODO referenced_by
func fileToJSONDoc(file *vfs.FileDoc) couchdb.JSONDoc {
	doc := couchdb.JSONDoc{
		Type: consts.Files,
		M: map[string]interface{}{
			"type":       file.Type,
			"_id":        file.DocID,
			"_rev":       file.DocRev,
			"name":       file.DocName,
			"created_at": file.CreatedAt,
			"updated_at": file.UpdatedAt,
			"size":       file.ByteSize,
			"md5Sum":     file.MD5Sum,
			"mime":       file.Mime,
			"class":      file.Class,
			"executable": file.Executable,
			"trashed":    file.Trashed,
			"tags":       file.Tags,
		},
	}
	if file.DirID != "" {
		doc.M["dir_id"] = file.DirID
	}
	if file.RestorePath != "" {
		doc.M["restore_path"] = file.RestorePath
	}
	if len(file.Metadata) > 0 {
		doc.M["metadata"] = file.Metadata
	}
	return doc
}
