package sharing

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Reference names the object of another app that a drive was provisioned
// for, and the last revision of that object applied to the drive.
type Reference struct {
	Source   string `json:"source"`
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	// Ready is set once the broker confirmed the reply announcing the drive.
	Ready bool `json:"ready,omitempty"`
}

// FindByReference returns the sharings provisioned for this object, the
// revoked ones included.
func FindByReference(db prefixer.Prefixer, source, id string) ([]*Sharing, error) {
	req := &couchdb.FindRequest{
		UseIndex: "by-reference",
		Selector: mango.And(
			mango.Equal("reference.source", source),
			mango.Equal("reference.id", id),
		),
		Limit: 100,
	}
	var res []*Sharing
	err := couchdb.FindDocs(db, consts.Sharings, req, &res)
	if couchdb.IsNoDatabaseError(err) {
		return nil, nil
	}
	return res, err
}
