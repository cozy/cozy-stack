package sharings

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

// Sharing is a struct containing all the information about a sharing
type Sharing struct {
	SID       string `json:"_id,omitempty"`
	SRev      string `json:"_rev,omitempty"`
	Type      string `json:"type"`
	Owner     bool   `json:"owner,omitempty"`
	Desc      string `json:"desc,omitempty"`
	SharingID string `json:"sharing_id,omitempty"`

	Permissions permissions.Set `json:"permissions,omitempty"`
	Recipients  Recipients      `json:"recipients,omitempty"`
}

// ID returns the sharing qualified identifier
func (s *Sharing) ID() string { return s.SID }

// Rev returns the sharing revision
func (s *Sharing) Rev() string { return s.SRev }

// DocType returns the sharing document type
func (s *Sharing) DocType() string { return consts.Sharings }

// SetID changes the sharing qualified identifier
func (s *Sharing) SetID(id string) { s.SID = id }

// SetRev changes the sharing revision
func (s *Sharing) SetRev(rev string) { s.SRev = rev }

// Relationships implements jsonapi.Doc
func (s *Sharing) Relationships() jsonapi.RelationshipMap { return nil }

// Included implements jsonapi.Doc
func (s *Sharing) Included() []jsonapi.Object { return nil }

// Links implements jsonapi.Doc
func (s *Sharing) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/sharing/" + s.SID}
}

// Create creates a Sharing document
func Create(db couchdb.Database, doc *Sharing) (*Sharing, error) {

	fmt.Println("let's create doc")

	/*doc := &Sharing{
		SID:         sharingID,
		Type:        sharingType,
		Owner:       owner,
		Desc:        desc,
		Permissions: perm,
		Recipients:  rec,
	}*/

	err := couchdb.CreateDoc(db, doc)
	if err != nil {
		return nil, err
	}

	return doc, nil
}
