package contacts

import (
	"encoding/json"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/workers/mails"
)

// Name is a struct describing a name of a contact
type Name struct {
	FamilyName     string `json:"familyName,omitempty"`
	GivenName      string `json:"givenName,omitempty"`
	AdditionalName string `json:"additionalName,omitempty"`
	NamePrefix     string `json:"namePrefix,omitempty"`
	NameSuffix     string `json:"nameSuffix,omitempty"`
}

// Email is a struct describing an email of a contact
type Email struct {
	Address string `json:"address"`
	Type    string `json:"type,omitempty"`
	Label   string `json:"label,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// Address is a struct describing an address of a contact
type Address struct {
	Street           string `json:"street,omitempty"`
	Pobox            string `json:"pobox,omitempty"`
	City             string `json:"city,omitempty"`
	Region           string `json:"region,omitempty"`
	Postcode         string `json:"postcode,omitempty"`
	Country          string `json:"country,omitempty"`
	Type             string `json:"type,omitempty"`
	Primary          bool   `json:"primary,omitempty"`
	Label            string `json:"label,omitempty"`
	FormattedAddress string `json:"formattedAddress,omitempty"`
}

// Phone is a struct describing a phone of a contact
type Phone struct {
	Number  string `json:"number"`
	Type    string `json:"type,omitempty"`
	Label   string `json:"label,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// Cozy is a struct describing a cozy instance of a contact
type Cozy struct {
	URL     string `json:"url"`
	Label   string `json:"label,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// Contact is a struct containing all the informations about a contact
type Contact struct {
	DocID  string `json:"_id,omitempty"`
	DocRev string `json:"_rev,omitempty"`

	FullName string    `json:"fullname,omitempty"`
	Name     Name      `json:"name,omitempty"`
	Birthday string    `json:"birthday,omitempty"`
	Note     string    `json:"note,omitempty"`
	Email    []Email   `json:"email,omitempty"`
	Address  []Address `json:"address,omitempty"`
	Phone    []Phone   `json:"phone,omitempty"`
	Cozy     []Cozy    `json:"cozy,omitempty"`
}

// ID returns the contact qualified identifier
func (c *Contact) ID() string { return c.DocID }

// Rev returns the contact revision
func (c *Contact) Rev() string { return c.DocRev }

// DocType returns the contact document type
func (c *Contact) DocType() string { return consts.Contacts }

// Clone implements couchdb.Doc
func (c *Contact) Clone() couchdb.Doc {
	cloned := *c
	cloned.FullName = c.FullName
	cloned.Name = c.Name

	cloned.Email = make([]Email, len(c.Email))
	copy(cloned.Email, c.Email)

	cloned.Address = make([]Address, len(c.Address))
	copy(cloned.Address, c.Address)

	cloned.Phone = make([]Phone, len(c.Phone))
	copy(cloned.Phone, c.Phone)

	cloned.Cozy = make([]Cozy, len(c.Cozy))
	copy(cloned.Cozy, c.Cozy)

	return &cloned
}

// SetID changes the contact qualified identifier
func (c *Contact) SetID(id string) { c.DocID = id }

// SetRev changes the contact revision
func (c *Contact) SetRev(rev string) { c.DocRev = rev }

// ToMailAddress returns a struct that can be used by cozy-stack to send an
// email to this contact
func (c *Contact) ToMailAddress() (*mails.Address, error) {
	if len(c.Email) == 0 {
		return nil, ErrNoMailAddress
	}
	mail := c.Email[0].Address
	for _, email := range c.Email {
		if email.Primary {
			mail = email.Address
		}
	}
	name := c.PrimaryName()
	if name == "" {
		name = mail
	}
	return &mails.Address{Name: name, Email: mail}, nil
}

// PrimaryName returns the name of the contact
func (c *Contact) PrimaryName() string {
	if c.FullName != "" {
		return c.FullName
	}
	if c.Name.GivenName != "" || c.Name.FamilyName != "" {
		return c.Name.GivenName + " " + c.Name.FamilyName
	}
	return ""
}

// PrimaryCozyURL returns the URL of the primary cozy,
// or a blank string if the contact has no known cozy.
func (c *Contact) PrimaryCozyURL() string {
	if len(c.Cozy) == 0 {
		return ""
	}
	for _, cozy := range c.Cozy {
		if cozy.Primary {
			return cozy.URL
		}
	}
	return c.Cozy[0].URL
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
	err := couchdb.ExecView(db, consts.ContactByEmail, &couchdb.ViewRequest{
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
