package sharing

import (
	"encoding/json"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// TrackMessage is used for jobs on the share-track worker.
// It's the same for all the jobs of a trigger.
type TrackMessage struct {
	SharingID string `json:"sharing_id"`
	RuleIndex int    `json:"rule_index"`
	DocType   string `json:"doctype"`
}

// TrackEvent is used for jobs on the share-track worker.
// It's unique per job.
type TrackEvent struct {
	Verb   string           `json:"verb"`
	Doc    couchdb.JSONDoc  `json:"doc"`
	OldDoc *couchdb.JSONDoc `json:"old,omitempty"`
}

// SharedInfo gives informations about how to apply the sharing to the shared
// document
type SharedInfo struct {
	// Rule is the index of the rule inside the sharing rules
	Rule int `json:"rule"`

	// Removed is true for a deleted document, a trashed file, or if the
	// document does no longer match the sharing rule
	Removed bool `json:"removed,omitempty"`

	// Binary is a boolean flag that is true only for files (and not even
	// folders) with `removed: false`
	Binary bool `json:"binary,omitempty"`

	// Dissociated is a boolean flag that can be true only for files and
	// folders when they have been removed from the sharing but can be put
	// again (only on the Cozy instance of the owner)
	Dissociated bool `json:"dissociated,omitempty"`
}

// SharedRef is the struct for the documents in io.cozy.shared.
// They are used to track which documents is in which sharings.
type SharedRef struct {
	// SID is the identifier, it is doctype + / + id of the referenced doc
	SID  string `json:"_id,omitempty"`
	SRev string `json:"_rev,omitempty"`

	// Revisions is a tree with the last known _rev of the shared object.
	Revisions *RevsTree `json:"revisions"`

	// Infos is a map of sharing ids -> informations
	Infos map[string]SharedInfo `json:"infos"`
}

// ID returns the sharing qualified identifier
func (s *SharedRef) ID() string { return s.SID }

// Rev returns the sharing revision
func (s *SharedRef) Rev() string { return s.SRev }

// DocType returns the sharing document type
func (s *SharedRef) DocType() string { return consts.Shared }

// SetID changes the sharing qualified identifier
func (s *SharedRef) SetID(id string) { s.SID = id }

// SetRev changes the sharing revision
func (s *SharedRef) SetRev(rev string) { s.SRev = rev }

// Clone implements couchdb.Doc
func (s *SharedRef) Clone() couchdb.Doc {
	cloned := *s
	revs := s.Revisions.Clone()
	cloned.Revisions = &revs
	cloned.Infos = make(map[string]SharedInfo, len(s.Infos))
	for k, v := range s.Infos {
		cloned.Infos[k] = v
	}
	return &cloned
}

// Fetch implements the permission.Fetcher interface
func (s *SharedRef) Fetch(field string) []string {
	switch field {
	case "sharing":
		var keys []string
		for key := range s.Infos {
			keys = append(keys, key)
		}
		return keys
	}
	return nil
}

// FindReferences returns the io.cozy.shared references to the given identifiers
func FindReferences(inst *instance.Instance, ids []string) ([]*SharedRef, error) {
	var refs []*SharedRef
	req := &couchdb.AllDocsRequest{Keys: ids}
	if err := couchdb.GetAllDocs(inst, consts.Shared, req, &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

// extractReferencedBy extracts the referenced_by slice from the given doc
// and cast it to the right type
func extractReferencedBy(doc *couchdb.JSONDoc) []couchdb.DocReference {
	slice, _ := doc.Get(couchdb.SelectorReferencedBy).([]interface{})
	refs := make([]couchdb.DocReference, len(slice))
	for i, ref := range slice {
		switch r := ref.(type) {
		case couchdb.DocReference:
			refs[i] = r
		case map[string]interface{}:
			id, _ := r["id"].(string)
			typ, _ := r["type"].(string)
			refs[i] = couchdb.DocReference{ID: id, Type: typ}
		}
	}
	return refs
}

// isNoLongerShared returns true for a document/file/folder that has matched a
// rule of a sharing, but no longer does.
func isNoLongerShared(inst *instance.Instance, msg TrackMessage, evt TrackEvent) (bool, error) {
	switch msg.DocType {
	case consts.Files:
		return isFileNoLongerShared(inst, msg, evt)
	case consts.BitwardenCiphers:
		return isCipherNoLongerShared(inst, msg, evt)
	default:
		return false, nil
	}
}

func isCipherNoLongerShared(inst *instance.Instance, msg TrackMessage, evt TrackEvent) (bool, error) {
	if evt.OldDoc == nil {
		return false, nil
	}
	oldOrg := evt.OldDoc.Get("organization_id")
	newOrg := evt.Doc.Get("organization_id")
	if oldOrg != newOrg {
		s, err := FindSharing(inst, msg.SharingID)
		if err != nil {
			return false, err
		}
		rule := s.Rules[msg.RuleIndex]
		if rule.Selector == "organization_id" {
			for _, val := range rule.Values {
				if val == newOrg {
					return false, nil
				}
			}
			return true, nil
		}
	}
	return false, nil
}

func isFileNoLongerShared(inst *instance.Instance, msg TrackMessage, evt TrackEvent) (bool, error) {
	// Optim: if dir_id and referenced_by have not changed, the file can't have
	// been removed from the sharing. Same if it has no old doc.
	if evt.OldDoc == nil {
		return false, nil
	}
	if evt.Doc.Get("type") == consts.FileType {
		if evt.OldDoc.Get("dir_id") == evt.Doc.Get("dir_id") {
			olds := extractReferencedBy(evt.OldDoc)
			news := extractReferencedBy(&evt.Doc)
			if vfs.SameReferences(olds, news) {
				return false, nil
			}
		}
	} else {
		// For a directory, we have to check the path, as it can be a subfolder
		// of a folder moved from inside the sharing to outside, and we will
		// have an event for that (path is updated by the VFS).
		if evt.OldDoc.Get("path") == evt.Doc.Get("path") {
			olds := extractReferencedBy(evt.OldDoc)
			news := extractReferencedBy(&evt.Doc)
			if vfs.SameReferences(olds, news) {
				return false, nil
			}
		}
	}

	s, err := FindSharing(inst, msg.SharingID)
	if err != nil {
		return false, err
	}
	rule := s.Rules[msg.RuleIndex]
	if rule.Selector == couchdb.SelectorReferencedBy {
		refs := extractReferencedBy(&evt.Doc)
		for _, ref := range refs {
			if rule.hasReferencedBy(ref) {
				return false, nil
			}
		}
		return true, nil
	}
	if rule.Selector == "" || rule.Selector == "id" {
		docID := evt.Doc.ID()
		for _, id := range rule.Values {
			if id == docID {
				return false, nil
			}
		}
	}

	var docPath string
	if evt.Doc.Get("type") == consts.FileType {
		dirID, ok := evt.Doc.Get("dir_id").(string)
		if !ok {
			return false, ErrInternalServerError
		}
		var parent *vfs.DirDoc
		parent, err = inst.VFS().DirByID(dirID)
		if err != nil {
			return false, err
		}
		docPath = parent.Fullpath
	} else {
		p, ok := evt.Doc.Get("path").(string)
		if !ok {
			return false, ErrInternalServerError
		}
		docPath = p
	}
	sharingDir, err := inst.VFS().DirByID(rule.Values[0])
	if err != nil {
		return false, err
	}
	return !strings.HasPrefix(docPath+"/", sharingDir.Fullpath+"/"), nil
}

// isTheSharingDirectory returns true if the event was for the directory that
// is the root of the sharing: we don't want to track it in io.cozy.shared.
func isTheSharingDirectory(inst *instance.Instance, msg TrackMessage, evt TrackEvent) (bool, error) {
	if evt.Doc.Type != consts.Files || evt.Doc.Get("type") != consts.DirType {
		return false, nil
	}
	s, err := FindSharing(inst, msg.SharingID)
	if err != nil {
		return false, err
	}
	rule := s.Rules[msg.RuleIndex]
	if rule.Selector == couchdb.SelectorReferencedBy {
		return false, nil
	}
	id := evt.Doc.ID()
	for _, val := range rule.Values {
		if val == id {
			return true, nil
		}
	}
	return false, nil
}

// updateRemovedForFiles updates the removed flag for files inside a directory
// that was moved.
func updateRemovedForFiles(inst *instance.Instance, sharingID, dirID string, rule int, removed bool) error {
	dir := &vfs.DirDoc{DocID: dirID}
	cursor := couchdb.NewSkipCursor(100, 0)
	var docs []interface{}
	for cursor.HasMore() {
		children, err := inst.VFS().DirBatch(dir, cursor)
		if err != nil {
			return err
		}
		for _, child := range children {
			_, file := child.Refine()
			if file == nil {
				continue
			}
			sid := consts.Files + "/" + file.ID()
			var ref SharedRef
			if err := couchdb.GetDoc(inst, consts.Shared, sid, &ref); err != nil {
				if !couchdb.IsNotFoundError(err) {
					return err
				}
				ref.SID = sid
				ref.Infos = make(map[string]SharedInfo)
			}
			ref.Infos[sharingID] = SharedInfo{
				Rule:    rule,
				Removed: removed,
				Binary:  !removed,
			}
			rev := file.Rev()
			if ref.Rev() == "" {
				ref.Revisions = &RevsTree{Rev: rev}
			}
			docs = append(docs, ref)
		}
	}
	if len(docs) == 0 {
		return nil
	}
	olds := make([]interface{}, len(docs))
	return couchdb.BulkUpdateDocs(inst, consts.Shared, docs, olds)
}

// UpdateShared updates the io.cozy.shared database when a document is
// created/update/removed
func UpdateShared(inst *instance.Instance, msg TrackMessage, evt TrackEvent) error {
	evt.Doc.Type = msg.DocType
	sid := evt.Doc.Type + "/" + evt.Doc.ID()

	mu := lock.ReadWrite(inst, "shared/"+sid)
	if err := mu.Lock(); err != nil {
		return err
	}
	defer mu.Unlock()

	var ref SharedRef
	if err := couchdb.GetDoc(inst, consts.Shared, sid, &ref); err != nil {
		if !couchdb.IsNotFoundError(err) {
			return err
		}
		ref.SID = sid
		ref.Infos = make(map[string]SharedInfo)
	}

	rev := evt.Doc.Rev()
	// XXX this optimization only works for files
	if _, ok := ref.Infos[msg.SharingID]; ok && msg.DocType == consts.Files {
		if sub, _ := ref.Revisions.Find(rev); sub != nil {
			return nil
		}
	}

	// If a document was in a sharing, was removed of the sharing, and comes
	// back inside it, we need to clear the Removed flag.
	needToUpdateFiles := false
	removed := false
	wasRemoved := true
	ruleIndex := msg.RuleIndex
	if rule, ok := ref.Infos[msg.SharingID]; ok {
		wasRemoved = rule.Removed
		ruleIndex = ref.Infos[msg.SharingID].Rule
	}
	ref.Infos[msg.SharingID] = SharedInfo{
		Rule:    ruleIndex,
		Binary:  evt.Doc.Type == consts.Files && evt.Doc.Get("type") == consts.FileType,
		Removed: false,
	}

	if evt.Verb == "DELETED" || isTrashed(evt.Doc) {
		// Do not create a shared doc for a deleted document: it's useless and
		// it can have some side effects!
		if ref.Rev() == "" {
			return nil
		}
		ref.Infos[msg.SharingID] = SharedInfo{
			Rule:    ruleIndex,
			Removed: true,
			Binary:  false,
		}
	} else {
		if skip, err := isTheSharingDirectory(inst, msg, evt); err != nil || skip {
			return err
		}
		var err error
		removed, err = isNoLongerShared(inst, msg, evt)
		if err != nil {
			return err
		}
		if removed {
			if ref.Rev() == "" {
				return nil
			}
			ref.Infos[msg.SharingID] = SharedInfo{
				Rule:    ruleIndex,
				Removed: true,
				Binary:  false,
			}
		}
		if evt.Doc.Type == consts.Files && evt.Doc.Get("type") == consts.DirType {
			needToUpdateFiles = removed != wasRemoved
		}
	}

	if ref.Rev() == "" {
		ref.Revisions = &RevsTree{Rev: rev}
		if err := couchdb.CreateNamedDoc(inst, &ref); err != nil {
			return err
		}
	} else {
		if evt.OldDoc == nil {
			inst.Logger().WithNamespace("sharing").
				Warnf("Updating an io.cozy.shared with no previous revision: %v %v", evt, ref)
			if subtree, _ := ref.Revisions.Find(rev); subtree == nil {
				ref.Revisions.Add(rev)
			}
		} else {
			chain, err := addMissingRevsToChain(inst, &ref, []string{rev})
			if err != nil {
				return err
			}
			ref.Revisions.InsertChain(chain)
		}
		if err := couchdb.UpdateDoc(inst, &ref); err != nil {
			return err
		}
	}

	// For a directory, we have to update the Removed flag for the files inside
	// it, as we won't have any events for them.
	if needToUpdateFiles {
		err := updateRemovedForFiles(inst, msg.SharingID, evt.Doc.ID(), ruleIndex, removed)
		if err != nil {
			inst.Logger().WithNamespace("sharing").
				Warnf("Error on updateRemovedForFiles for %v: %s", evt, err)
		}
	}

	return nil
}

// UpdateFileShared creates or updates the io.cozy.shared for a file with
// possibly multiple revisions.
func UpdateFileShared(db prefixer.Prefixer, ref *SharedRef, revs RevsStruct) error {
	chain := revsStructToChain(revs)
	if ref.Rev() == "" {
		ref.Revisions = &RevsTree{Rev: chain[0]}
		ref.Revisions.InsertChain(chain)
		return couchdb.CreateNamedDoc(db, ref)
	}
	chain, err := addMissingRevsToChain(db, ref, chain)
	if err != nil {
		return err
	}
	ref.Revisions.InsertChain(chain)
	return couchdb.UpdateDoc(db, ref)
}

// RemoveSharedRefs deletes the references containing the sharingid
func RemoveSharedRefs(inst *instance.Instance, sharingID string) error {
	// We can have CouchDB conflicts if another instance is synchronizing files
	// to this instance
	maxRetries := 5
	var err error
	for i := 0; i < maxRetries; i++ {
		err = doRemoveSharedRefs(inst, sharingID)
		if !couchdb.IsConflictError(err) {
			return err
		}
	}
	return err
}

func doRemoveSharedRefs(inst *instance.Instance, sharingID string) error {
	req := &couchdb.ViewRequest{
		Key:         sharingID,
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse
	err := couchdb.ExecView(inst, couchdb.SharedDocsBySharingID, req, &res)
	if err != nil {
		return err
	}

	for _, row := range res.Rows {
		var doc SharedRef
		if err = json.Unmarshal(row.Doc, &doc); err != nil {
			return err
		}
		// Remove the ref if there are others sharings; remove the doc otherwise
		if len(doc.Infos) > 1 {
			delete(doc.Infos, sharingID)
			if err = couchdb.UpdateDoc(inst, &doc); err != nil {
				return err
			}
		} else {
			if err = couchdb.DeleteDoc(inst, &doc); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetSharedDocsBySharingIDs returns a map associating each given sharingID
// to a list of DocReference, which are the shared documents
func GetSharedDocsBySharingIDs(inst *instance.Instance, sharingIDs []string) (map[string][]couchdb.DocReference, error) {
	keys := make([]interface{}, len(sharingIDs))
	for i, id := range sharingIDs {
		keys[i] = id
	}
	req := &couchdb.ViewRequest{
		Keys:        keys,
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse

	err := couchdb.ExecView(inst, couchdb.SharedDocsBySharingID, req, &res)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]couchdb.DocReference, len(res.Rows))

	for _, row := range res.Rows {
		var doc SharedRef
		err := json.Unmarshal(row.Doc, &doc)
		if err != nil {
			return nil, err
		}
		sID := row.Key.(string)
		// Filter out the removed docs
		if !doc.Infos[sID].Removed {
			docRef := extractDocReferenceFromID(doc.ID())
			if docRef != nil {
				result[sID] = append(result[sID], *docRef)
			}
		}
	}
	return result, nil
}

// extractDocReferenceFromID takes a string formatted as doctype/docid and
// returns a DocReference
func extractDocReferenceFromID(id string) *couchdb.DocReference {
	var ref couchdb.DocReference
	slice := strings.SplitN(id, "/", 2)
	if len(slice) != 2 {
		return nil
	}
	ref.ID = slice[1]
	ref.Type = slice[0]
	return &ref
}

// CheckShared will scan all the io.cozy.shared documents and check their
// revision tree for inconsistencies.
func CheckShared(inst *instance.Instance) ([]*CheckSharedError, error) {
	checks := []*CheckSharedError{}
	err := couchdb.ForeachDocs(inst, consts.Shared, func(id string, data json.RawMessage) error {
		s := &SharedRef{}
		if err := json.Unmarshal(data, s); err != nil {
			checks = append(checks, &CheckSharedError{Type: "invalid_json", ID: id})
			return nil
		}
		if check := s.Revisions.check(); check != nil {
			check.ID = s.SID
			checks = append(checks, check)
		}
		return nil
	})
	return checks, err
}

var _ couchdb.Doc = &SharedRef{}
var _ permission.Fetcher = &SharedRef{}
