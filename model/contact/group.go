package contact

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Group is a struct containing all the information about a group of contacts.
type Group struct {
	couchdb.JSONDoc
}

// DocType returns the contacts groups document type
func (g *Group) DocType() string { return consts.ContactsGroups }

// Name returns the name of the group
func (g *Group) Name() string {
	name, _ := g.Get("name").(string)
	return name
}

// ListContacts returns the list of contacts in this group.
func (g *Group) ListContacts(inst *instance.Instance) ([]*Contact, error) {
	var docs []*Contact
	req := &couchdb.FindRequest{
		Selector: mango.ElemMatch("relationships.groups.data", mango.And(
			mango.Equal("_type", consts.ContactsGroups),
			mango.Equal("_id", g.ID()),
		)),
		Limit: 100,
	}
	err := couchdb.FindDocsUnoptimized(inst, consts.Contacts, req, &docs)
	return docs, err
}

// FindGroup returns the group stored in database from a given ID
func FindGroup(db prefixer.Prefixer, groupID string) (*Group, error) {
	doc := &Group{}
	err := couchdb.GetDoc(db, consts.ContactsGroups, groupID, doc)
	return doc, err
}
