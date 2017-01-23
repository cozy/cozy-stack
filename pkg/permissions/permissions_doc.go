package permissions

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

// Permission is a storable object containing a set of rules and
// several codes
type Permission struct {
	pID           string
	pRev          string
	ApplicationID string            `json:"application_id"`
	Permissions   Set               `json:"permissions, omitempty"`
	ExpiresAt     int               `json:"expires_at, omitempty"`
	Codes         map[string]string `json:"codes, omitempty"`
}

// Index is the necessary index for this package
// used in instance creation
var Index = mango.IndexOnFields("application_id")

// ID implements jsonapi.Doc
func (p *Permission) ID() string { return p.pID }

// Rev implements jsonapi.Doc
func (p *Permission) Rev() string { return p.pRev }

// DocType implements jsonapi.Doc
func (p *Permission) DocType() string { return consts.Permissions }

// SetID implements jsonapi.Doc
func (p *Permission) SetID(id string) { p.pID = id }

// SetRev implements jsonapi.Doc
func (p *Permission) SetRev(rev string) { p.pRev = rev }

// Relationships implements jsonapi.Doc
func (p *Permission) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{
		"application": jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Related: "/apps/" + p.ApplicationID,
			},
			Data: jsonapi.ResourceIdentifier{
				ID:   p.ApplicationID,
				Type: consts.Manifests,
			},
		},
	}
}

// Included implements jsonapi.Doc
func (p *Permission) Included() []jsonapi.Object { return nil }

// SelfLink implements jsonapi.Doc
func (p *Permission) SelfLink() string { return "/permissions/" + p.pID }

// GetForApp retrieves the Permission doc for a given app
func GetForApp(db couchdb.Database, slug string) (*Permission, error) {
	var res []*Permission
	err := couchdb.FindDocs(db, consts.Permissions, &couchdb.FindRequest{
		Selector: mango.Equal("application_id", consts.Manifests+"/"+slug),
	}, res)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("no permission doc for %v", slug)
	}
	return res[0], nil
}

// Create creates a Permission doc for a given app
func Create(db couchdb.Database, slug string, set Set) (*Permission, error) {
	existing, _ := GetForApp(db, slug)
	if existing != nil {
		return nil, fmt.Errorf("There is already a permission doc for %v", slug)
	}

	doc := &Permission{
		ApplicationID: consts.Manifests + "/" + slug,
		Permissions:   set, // @TODO some validation?
	}

	err := couchdb.CreateDoc(db, doc)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// Destroy removes Permission doc for a given app
func Destroy(db couchdb.Database, slug string) error {
	existing, err := GetForApp(db, slug)
	if err != nil {
		return err
	}
	return couchdb.DeleteDoc(db, existing)
}
