package sharings

import (
	"encoding/json"
	"net/url"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
)

// Member contains the information about a recipient (or the sharer) for a sharing
type Member struct {
	Status string `json:"status,omitempty"`
	URL    string `json:"url,omitempty"`

	// Only a reference on the contact is persisted in the sharing document
	RefContact couchdb.DocReference `json:"contact,omitempty"`
	contact    *contacts.Contact

	// Information needed to send data to the member
	Client      auth.Client      `json:"client"`
	AccessToken auth.AccessToken `json:"access_token"`

	// The OAuth ClientID used for authentifying incoming requests from the member
	InboundClientID string `json:"inbound_client_id,omitempty"`
}

// Contact get the actual contact of a Member
func (m *Member) Contact(db couchdb.Database) *contacts.Contact {
	if m.contact == nil {
		if db == nil {
			return nil
		}
		c, err := contacts.Find(db, m.RefContact.ID)
		if err != nil {
			return nil
		}
		m.contact = c
	}
	return m.contact
}

// CreateOrUpdateRecipient inserts a Recipient document in database. Email and URL must
// not be empty.
// TODO use an ID to find the contact and kill the SharingRecipientView
func CreateOrUpdateRecipient(db couchdb.Database, doc *contacts.Contact) error {
	if len(doc.Cozy) == 0 && len(doc.Email) == 0 {
		return ErrRecipientBadParams
	}

	var res couchdb.ViewResponse
	if len(doc.Email) > 0 {
		err := couchdb.ExecView(db, consts.SharingRecipientView, &couchdb.ViewRequest{
			Key:         []string{doc.Email[0].Address, "email"},
			IncludeDocs: true,
			Limit:       1,
		}, &res)
		if err == nil && len(res.Rows) > 0 {
			if len(doc.Cozy) == 0 {
				return json.Unmarshal(res.Rows[0].Doc, &doc)
			}
			cozy := doc.Cozy[0]
			doc.Cozy = nil
			if err = json.Unmarshal(res.Rows[0].Doc, &doc); err != nil {
				return err
			}
			for _, c := range doc.Cozy {
				if c.URL == cozy.URL {
					return nil
				}
			}
			doc.Cozy = append(doc.Cozy, cozy)
			return couchdb.UpdateDoc(db, doc)
		}
	}

	if len(doc.Cozy) > 0 {
		err := couchdb.ExecView(db, consts.SharingRecipientView, &couchdb.ViewRequest{
			Key:         []string{doc.Cozy[0].URL, "cozy"},
			IncludeDocs: true,
			Limit:       1,
		}, &res)
		if err == nil && len(res.Rows) > 0 {
			if len(doc.Email) == 0 {
				return json.Unmarshal(res.Rows[0].Doc, &doc)
			}
			email := doc.Email[0]
			doc.Email = nil
			if err = json.Unmarshal(res.Rows[0].Doc, &doc); err != nil {
				return err
			}
			for _, e := range doc.Email {
				if e.Address == email.Address {
					return nil
				}
			}
			doc.Email = append(doc.Email, email)
			return couchdb.UpdateDoc(db, doc)
		}
	}

	return couchdb.CreateDoc(db, doc)
}

// GetAccessToken sends an "access_token" request to the recipient using the
// given authorization code.
func (m *Member) GetAccessToken(code string) (*auth.AccessToken, error) {
	if m.URL == "" {
		return nil, ErrRecipientHasNoURL
	}
	if m.Client.ClientID == "" {
		return nil, ErrNoOAuthClient
	}

	u, err := url.Parse(m.URL)
	if err != nil {
		return nil, err
	}

	req := &auth.Request{
		Domain: u.Host,
		Scheme: u.Scheme,
	}
	return req.GetAccessToken(&m.Client, code)
}

// RegisterClient asks the Cozy of the member to register a new OAuth client.
func (m *Member) RegisterClient(i *instance.Instance, u *url.URL) error {
	req := &auth.Request{
		Domain: u.Host,
		Scheme: u.Scheme,
	}

	publicName, err := i.PublicName()
	if err != nil {
		publicName = i.Domain
	}
	redirectURI := i.PageURL("/sharings/answer", nil)
	clientURI := i.PageURL("", nil)
	authClient := &auth.Client{
		RedirectURIs: []string{redirectURI},
		ClientName:   publicName,
		ClientKind:   "sharing",
		SoftwareID:   "github.com/cozy/cozy-stack",
		ClientURI:    clientURI,
	}

	resClient, err := req.RegisterClient(authClient)
	if err != nil {
		return err
	}
	m.URL = u.String()
	m.Client = *resClient
	return nil
}
