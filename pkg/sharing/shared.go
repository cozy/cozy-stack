package sharing

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/lock"
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
	Verb string          `json:"verb"`
	Doc  couchdb.JSONDoc `json:"doc"`
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
}

// SharedRef is the struct for the documents in io.cozy.shared.
// They are used to track which documents is in which sharings.
type SharedRef struct {
	// SID is the identifier, it is doctype + / + id of the referenced doc
	SID  string `json:"_id,omitempty"`
	SRev string `json:"_rev,omitempty"`

	// Revisions is an array with the last known _rev of the shared object.
	// The revisions are sorted by growing generation (the number before the hyphen).
	// TODO it should be a tree, not an array (conflicts)
	Revisions []string `json:"revisions"`

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
	return &cloned
}

// Match implements the permissions.Matcher interface
func (s *SharedRef) Match(key, value string) bool {
	switch key {
	case "sharing":
		_, ok := s.Infos[value]
		return ok
	}
	return false
}

// RevGeneration returns the number before the hyphen, called the generation of a revision
func RevGeneration(rev string) int {
	parts := strings.SplitN(rev, "-", 2)
	gen, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return gen
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

// UpdateShared updates the io.cozy.shared database when a document is
// created/update/removed
func UpdateShared(inst *instance.Instance, msg TrackMessage, evt TrackEvent) error {
	mu := lock.ReadWrite(inst.Domain + "/shared")
	mu.Lock()
	defer mu.Unlock()

	evt.Doc.Type = msg.DocType
	sid := evt.Doc.Type + "/" + evt.Doc.ID()
	var ref SharedRef
	if err := couchdb.GetDoc(inst, consts.Shared, sid, &ref); err != nil {
		if !couchdb.IsNotFoundError(err) {
			return err
		}
		ref.SID = sid
		ref.Infos = make(map[string]SharedInfo)
	}

	if _, ok := ref.Infos[msg.SharingID]; !ok {
		ref.Infos[msg.SharingID] = SharedInfo{
			Rule:   msg.RuleIndex,
			Binary: evt.Doc.Type == consts.Files && evt.Doc.Get("type") == consts.FileType,
		}
	}
	// TODO detect when a file/folder is trashed
	// TODO detect when a document goes out of a sharing
	if evt.Verb == "DELETED" {
		infos := ref.Infos[msg.SharingID]
		infos.Removed = true
		infos.Binary = false
	}

	// TODO to be improved when we will work on conflicts
	rev := evt.Doc.Rev()
	if len(ref.Revisions) == 0 || ref.Revisions[len(ref.Revisions)-1] != rev {
		ref.Revisions = append(ref.Revisions, rev)
	}

	if ref.Rev() == "" {
		return couchdb.CreateNamedDoc(inst, &ref)
	}
	return couchdb.UpdateDoc(inst, &ref)
}

// GetSharedDocsBySharingID returns the shared documents related to the
// given sharingID
func GetSharedDocsBySharingID(inst *instance.Instance, sharingID string) ([]couchdb.DocReference, error) {
	var req = &couchdb.ViewRequest{
		Key:         sharingID,
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse

	err := couchdb.ExecView(inst, consts.SharedDocsBySharingID, req, &res)
	if err != nil {
		return nil, err
	}

	var result []couchdb.DocReference

	for _, row := range res.Rows {
		var doc SharedRef
		err := json.Unmarshal(row.Doc, &doc)
		if err != nil {
			return nil, err
		}
		// Filter out the removed docs
		if !doc.Infos[sharingID].Removed {
			var d couchdb.DocReference
			id := doc.ID()
			slice := strings.Split(id, "/")
			if len(slice) != 2 {
				continue
			}
			d.ID = slice[1]
			d.Type = slice[0]
			result = append(result, d)
		}
	}
	return result, nil
}

// GetSharingsByDocType returns the sharingids related to the
// given docType
func GetSharingsByDocType(inst *instance.Instance, docType string) ([]string, error) {
	var req = &couchdb.AllDocsRequest{
		StartKey: docType + "/",
		EndKey:   docType + "/\uFFFF",
	}

	var docs []*SharedRef
	err := couchdb.GetAllDocs(inst, consts.Shared, req, &docs)
	if err != nil {
		return nil, err
	}
	// Use a map to easily avoid duplicates
	keys := make(map[string]bool)
	var sharingIDs []string
	for _, doc := range docs {
		for id := range doc.Infos {
			// Do not include removed docs
			if !keys[id] && !doc.Infos[id].Removed {
				keys[id] = true
				sharingIDs = append(sharingIDs, id)
			}
		}
	}
	return sharingIDs, nil
}

var _ couchdb.Doc = &SharedRef{}
