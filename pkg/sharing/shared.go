package sharing

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// SharedInfo gives informations about how to apply the sharing to the shared
// document
type SharedInfo struct {
	// Rule is the index of the rule inside the sharing rules
	Rule int `json:"rule"`

	// Removed is true for a deleted document, a trashed file, or if the
	// document does no longer match the sharing rule
	Remove bool `json:"removed,omitempty"`

	// Binary is a boolean flag that is true only for files (and not even
	// folders) with `removed: false`
	Binary bool `json:"binary,omitempty"`
}

// SharedDoc is the struct for the documents in io.cozy.shared.
// They are used to track which documents is in which sharings.
type SharedDoc struct {
	SID  string `json:"_id,omitempty"`
	SRev string `json:"_rev,omitempty"`

	// Revisions is an array with the last known _rev of the shared object
	Revisions []string `json:"revisions"`

	// Infos is a map of sharing ids -> informations
	Infos map[string]SharedInfo `json:"infos"`
}

// ID returns the sharing qualified identifier
func (s *SharedDoc) ID() string { return s.SID }

// Rev returns the sharing revision
func (s *SharedDoc) Rev() string { return s.SRev }

// DocType returns the sharing document type
func (s *SharedDoc) DocType() string { return consts.Shared }

// SetID changes the sharing qualified identifier
func (s *SharedDoc) SetID(id string) { s.SID = id }

// SetRev changes the sharing revision
func (s *SharedDoc) SetRev(rev string) { s.SRev = rev }

// Clone implements couchdb.Doc
func (s *SharedDoc) Clone() couchdb.Doc {
	cloned := *s
	return &cloned
}

var _ couchdb.Doc = &SharedDoc{}
