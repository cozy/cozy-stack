package move

import (
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
)

const (
	// ExportStateExporting is the state used when the export document is being
	// created.
	ExportStateExporting = "exporting"
	// ExportStateDone is used when the export document is finished, without
	// error.
	ExportStateDone = "done"
	// ExportStateError is used when the export document is finshed with error.
	ExportStateError = "error"
)

// ExportDoc is a documents storing the metadata of an export.
type ExportDoc struct {
	DocID     string `json:"_id,omitempty"`
	DocRev    string `json:"_rev,omitempty"`
	Domain    string `json:"domain"`
	PartsSize int64  `json:"parts_size,omitempty"`

	PartsCursors     []string      `json:"parts_cursors,omitempty"`
	WithDoctypes     []string      `json:"with_doctypes,omitempty"`
	WithoutFiles     bool          `json:"without_files,omitempty"`
	State            string        `json:"state"`
	CreatedAt        time.Time     `json:"created_at"`
	ExpiresAt        time.Time     `json:"expires_at"`
	TotalSize        int64         `json:"total_size,omitempty"`
	CreationDuration time.Duration `json:"creation_duration,omitempty"`
	Error            string        `json:"error,omitempty"`
}

// DocType implements the couchdb.Doc interface
func (e *ExportDoc) DocType() string { return consts.Exports }

// ID implements the couchdb.Doc interface
func (e *ExportDoc) ID() string { return e.DocID }

// Rev implements the couchdb.Doc interface
func (e *ExportDoc) Rev() string { return e.DocRev }

// SetID implements the couchdb.Doc interface
func (e *ExportDoc) SetID(id string) { e.DocID = id }

// SetRev implements the couchdb.Doc interface
func (e *ExportDoc) SetRev(rev string) { e.DocRev = rev }

// Clone implements the couchdb.Doc interface
func (e *ExportDoc) Clone() couchdb.Doc {
	clone := *e

	clone.PartsCursors = make([]string, len(e.PartsCursors))
	copy(clone.PartsCursors, e.PartsCursors)

	clone.WithDoctypes = make([]string, len(e.WithDoctypes))
	copy(clone.WithDoctypes, e.WithDoctypes)

	return &clone
}

// Links implements the jsonapi.Object interface
func (e *ExportDoc) Links() *jsonapi.LinksList { return nil }

// Relationships implements the jsonapi.Object interface
func (e *ExportDoc) Relationships() jsonapi.RelationshipMap { return nil }

// Included implements the jsonapi.Object interface
func (e *ExportDoc) Included() []jsonapi.Object { return nil }

// HasExpired returns whether or not the export document has expired.
func (e *ExportDoc) HasExpired() bool {
	return time.Until(e.ExpiresAt) <= 0
}

var _ jsonapi.Object = &ExportDoc{}

// GenerateAuthMessage generates a MAC authentificating the access to the
// export data.
func (e *ExportDoc) GenerateAuthMessage(i *instance.Instance) []byte {
	mac, err := crypto.EncodeAuthMessage(archiveMACConfig, i.SessionSecret(), []byte(e.ID()), nil)
	if err != nil {
		panic(fmt.Errorf("could not generate archive auth message: %s", err))
	}
	return mac
}

// verifyAuthMessage verifies the given MAC to authenticate and grant the
// access to the export data.
func verifyAuthMessage(i *instance.Instance, mac []byte) (string, bool) {
	exportID, err := crypto.DecodeAuthMessage(archiveMACConfig, i.SessionSecret(), mac, nil)
	return string(exportID), err == nil
}

// GetExport returns an Export document associated with the given instance and
// with the given MAC message.
func GetExport(inst *instance.Instance, mac []byte) (*ExportDoc, error) {
	exportID, ok := verifyAuthMessage(inst, mac)
	if !ok {
		return nil, ErrMACInvalid
	}
	var exportDoc ExportDoc
	if err := couchdb.GetDoc(couchdb.GlobalDB, consts.Exports, exportID, &exportDoc); err != nil {
		if couchdb.IsNotFoundError(err) || couchdb.IsNoDatabaseError(err) {
			return nil, ErrExportNotFound
		}
		return nil, err
	}
	return &exportDoc, nil
}

// GetExports returns the list of exported documents.
func GetExports(domain string) ([]*ExportDoc, error) {
	var docs []*ExportDoc
	req := &couchdb.FindRequest{
		UseIndex: "by-domain",
		Selector: mango.Equal("domain", domain),
		Sort: mango.SortBy{
			{Field: "domain", Direction: mango.Desc},
			{Field: "created_at", Direction: mango.Desc},
		},
		Limit: 256,
	}
	err := couchdb.FindDocs(couchdb.GlobalDB, consts.Exports, req, &docs)
	if err != nil && !couchdb.IsNoDatabaseError(err) {
		return nil, err
	}
	return docs, nil
}
