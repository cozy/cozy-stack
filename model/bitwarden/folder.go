package bitwarden

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/metadata"
)

// Folder is a space to organize ciphers. Its name is encrypted on client-side.
type Folder struct {
	CouchID  string                 `json:"_id,omitempty"`
	CouchRev string                 `json:"_rev,omitempty"`
	Name     string                 `json:"name"`
	Metadata *metadata.CozyMetadata `json:"cozyMetadata,omitempty"`
}

// ID returns the folder qualified identifier
func (f *Folder) ID() string { return f.CouchID }

// Rev returns the folder revision
func (f *Folder) Rev() string { return f.CouchRev }

// DocType returns the folder document type
func (f *Folder) DocType() string { return consts.BitwardenFolders }

// Clone implements couchdb.Doc
func (f *Folder) Clone() couchdb.Doc {
	cloned := *f
	if cloned.Metadata != nil {
		cloned.Metadata = f.Metadata.Clone()
	}
	return &cloned
}

// SetID changes the folder qualified identifier
func (f *Folder) SetID(id string) { f.CouchID = id }

// SetRev changes the folder revision
func (f *Folder) SetRev(rev string) { f.CouchRev = rev }

var _ couchdb.Doc = &Folder{}
