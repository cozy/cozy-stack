package sharings

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
)

// RecipientStatus contains the information about a recipient for a sharing
type RecipientStatus struct {
	Status string `json:"status,omitempty"`

	// Reference on the recipient.
	RefRecipient couchdb.DocReference `json:"recipient,omitempty"`
	recipient    *contacts.Contact

	// The sharer is the "client", in the OAuth2 protocol, we keep here the
	// information she needs to send to authenticate.
	Client      auth.Client      `json:"client"`
	AccessToken auth.AccessToken `json:"access_token"`

	// The OAuth ClientID refering to the host's client stored in its db
	InboundClientID string `json:"inbound_client_id,omitempty"`
}

// ExtractDomainAndScheme returns the recipient's domain and the scheme
func ExtractDomainAndScheme(r *contacts.Contact) (string, string, error) {
	if len(r.Cozy) == 0 {
		return "", "", ErrRecipientHasNoURL
	}
	u, err := url.Parse(r.Cozy[0].URL)
	if err != nil {
		return "", "", err
	}
	host := u.Host
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return host, scheme, nil
}

// GetRecipient get the actual recipient of a RecipientStatus
func (rs *RecipientStatus) GetRecipient(db couchdb.Database) error {
	if rs.recipient == nil {
		recipient, err := GetRecipient(db, rs.RefRecipient.ID)
		if err != nil {
			return err
		}
		rs.recipient = recipient
	}
	return nil
}

// CreateOrUpdateRecipient inserts a Recipient document in database. Email and URL must
// not be empty.
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

// ForceRecipient forces the recipient. It is useful when testing the URL of
// the cozy instances of the recipient before saving the recipient if
// successful.
func (rs *RecipientStatus) ForceRecipient(r *contacts.Contact) {
	rs.recipient = r
}

// GetRecipient returns the Recipient stored in database from a given ID
func GetRecipient(db couchdb.Database, recID string) (*contacts.Contact, error) {
	doc := &contacts.Contact{}
	err := couchdb.GetDoc(db, consts.Contacts, recID, doc)
	if couchdb.IsNotFoundError(err) {
		err = ErrRecipientDoesNotExist
	}
	return doc, err
}

// GetCachedRecipient returns the recipient cached in memory within
// the RecipientStatus. CAN BE NIL, Used by jsonapi
func (rs *RecipientStatus) GetCachedRecipient() *contacts.Contact {
	return rs.recipient
}

// getAccessToken sends an "access_token" request to the recipient using the
// given authorization code.
func (rs *RecipientStatus) getAccessToken(db couchdb.Database, code string) (*auth.AccessToken, error) {
	// Sanity check: `recipient` being a private attribute it can be nil if the
	// sharing document that references it was extracted from the database.
	if rs.recipient == nil {
		recipient, err := GetRecipient(db, rs.RefRecipient.ID)
		if err != nil {
			return nil, err
		}
		rs.recipient = recipient
	}

	if len(rs.recipient.Cozy) == 0 {
		return nil, ErrRecipientHasNoURL
	}
	if rs.Client.ClientID == "" {
		return nil, ErrNoOAuthClient
	}

	recipientDomain, scheme, err := ExtractDomainAndScheme(rs.recipient)
	if err != nil {
		return nil, err
	}

	req := &auth.Request{
		Domain:     recipientDomain,
		Scheme:     scheme,
		HTTPClient: new(http.Client),
	}

	return req.GetAccessToken(&rs.Client, code)
}

// Register asks the recipient to register the sharer as a new OAuth client.
//
// The following information must be provided to register:
// - redirect uri: where the recipient must answer the sharing request. Our
//		protocol forces this field to be: "sharerdomain/sharings/answer".
// - client name: the sharer's public name.
// - client kind: "sharing" since this will be a sharing oriented OAuth client.
// - software id: the link to the github repository of the stack.
// - client URI: the domain of the sharer's Cozy.
func (rs *RecipientStatus) Register(instance *instance.Instance) error {
	if rs.recipient == nil {
		r, err := GetRecipient(instance, rs.RefRecipient.ID)
		if err != nil {
			return err
		}
		rs.recipient = r
	}

	// If the recipient has no URL there is no point in registering.
	if len(rs.recipient.Cozy) == 0 {
		return ErrRecipientHasNoURL
	}

	publicName, err := instance.PublicName()
	if err != nil {
		return err
	}

	redirectURI := instance.PageURL("/sharings/answer", nil)
	clientURI := instance.PageURL("", nil)

	// We have all we need to register an OAuth client.
	authClient := &auth.Client{
		RedirectURIs: []string{redirectURI},
		ClientName:   publicName,
		ClientKind:   "sharing",
		SoftwareID:   "github.com/cozy/cozy-stack",
		ClientURI:    clientURI,
	}

	recipientURL, scheme, err := ExtractDomainAndScheme(rs.recipient)
	if err != nil {
		return err
	}

	req := &auth.Request{
		Domain:     recipientURL,
		Scheme:     scheme,
		HTTPClient: new(http.Client),
	}

	// We launch the register process.
	resClient, err := req.RegisterClient(authClient)
	if err != nil {
		return err
	}

	rs.Client = *resClient
	return nil
}
