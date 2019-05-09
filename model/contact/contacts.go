package contact

import (
	"encoding/json"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/mail"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Contact is a struct containing all the informations about a contact.
// We are using maps/slices/interfaces instead of structs, as it is a doctype
// that can also be used in front applications and they can add new fields. It
// would be complicated to maintain a up-to-date mapping, and failing to do so
// means that we can lose some data on JSON round-trip.
type Contact struct {
	couchdb.JSONDoc
}

// New returns a new blank contact.
func New() *Contact {
	return &Contact{
		JSONDoc: couchdb.JSONDoc{
			M: make(map[string]interface{}),
		},
	}
}

// DocType returns the contact document type
func (c *Contact) DocType() string { return consts.Contacts }

// ToMailAddress returns a struct that can be used by cozy-stack to send an
// email to this contact
func (c *Contact) ToMailAddress() (*mail.Address, error) {
	emails, ok := c.Get("email").([]interface{})
	if !ok || len(emails) == 0 {
		return nil, ErrNoMailAddress
	}
	var email string
	for i := range emails {
		obj, ok := emails[i].(map[string]interface{})
		if !ok {
			continue
		}
		address, ok := obj["address"].(string)
		if !ok {
			continue
		}
		if primary, ok := obj["primary"].(bool); ok && primary {
			email = address
		}
		if email == "" {
			email = address
		}
	}
	name := c.PrimaryName()
	if name == "" {
		name = email
	}
	return &mail.Address{Name: name, Email: email}, nil
}

// PrimaryName returns the name of the contact
func (c *Contact) PrimaryName() string {
	if fullname, ok := c.Get("fullname").(string); ok && fullname != "" {
		return fullname
	}
	name, ok := c.Get("name").(map[string]interface{})
	if !ok {
		return ""
	}
	var primary string
	if given, ok := name["givenName"].(string); ok && given != "" {
		primary = given
	}
	if family, ok := name["familyName"].(string); ok && family != "" {
		if primary != "" {
			primary += " "
		}
		primary += family
	}
	return primary
}

// PrimaryCozyURL returns the URL of the primary cozy,
// or a blank string if the contact has no known cozy.
func (c *Contact) PrimaryCozyURL() string {
	cozys, ok := c.Get("cozy").([]interface{})
	if !ok || len(cozys) == 0 {
		return ""
	}
	var url string
	for i := range cozys {
		cozy, ok := cozys[i].(map[string]interface{})
		if !ok {
			continue
		}
		u, ok := cozy["url"].(string)
		if !ok {
			continue
		}
		if primary, ok := cozy["primary"].(bool); ok && primary {
			url = u
		}
		if url == "" {
			url = u
		}
	}
	return url
}

// AddCozyURL adds a cozy URL to this contact (unless the contact has already
// this cozy URL) and saves the contact.
func (c *Contact) AddCozyURL(db prefixer.Prefixer, cozyURL string) error {
	cozys, ok := c.Get("cozy").([]interface{})
	if !ok {
		cozys = []interface{}{}
	}
	for i := range cozys {
		cozy, ok := cozys[i].(map[string]interface{})
		if !ok {
			continue
		}
		u, ok := cozy["url"].(string)
		if ok && cozyURL == u {
			return nil
		}
	}
	cozy := map[string]interface{}{"url": cozyURL}
	c.M["cozy"] = append([]interface{}{cozy}, cozys...)
	return couchdb.UpdateDoc(db, c)
}

// Find returns the contact stored in database from a given ID
func Find(db prefixer.Prefixer, contactID string) (*Contact, error) {
	doc := &Contact{}
	err := couchdb.GetDoc(db, consts.Contacts, contactID, doc)
	return doc, err
}

// FindByEmail returns the contact with the given email address, when possible
func FindByEmail(db couchdb.Database, email string) (*Contact, error) {
	var res couchdb.ViewResponse
	err := couchdb.ExecView(db, couchdb.ContactByEmail, &couchdb.ViewRequest{
		Key:         email,
		IncludeDocs: true,
		Limit:       1,
	}, &res)
	if err != nil {
		return nil, err
	}
	if len(res.Rows) == 0 {
		return nil, ErrNotFound
	}
	doc := &Contact{}
	err = json.Unmarshal(res.Rows[0].Doc, &doc)
	return doc, err
}

var _ couchdb.Doc = &Contact{}
