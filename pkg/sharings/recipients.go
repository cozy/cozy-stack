package sharings

import (
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
)

// RecipientEmail is a struct describing an email of a contact
type RecipientEmail struct {
	Address string `json:"address"`
}

// RecipientCozy is a struct describing a cozy instance of the recipient
type RecipientCozy struct {
	URL string `json:"url"`
}

// Recipient is a struct describing a sharing recipient
type Recipient struct {
	RID   string           `json:"_id,omitempty"`
	RRev  string           `json:"_rev,omitempty"`
	Email []RecipientEmail `json:"email,omitempty"`
	Cozy  []RecipientCozy  `json:"cozy,omitempty"`
}

// RecipientStatus contains the information about a recipient for a sharing
type RecipientStatus struct {
	Status string `json:"status,omitempty"`

	// Reference on the recipient.
	RefRecipient couchdb.DocReference `json:"recipient,omitempty"`
	recipient    *Recipient

	// The sharer is the "client", in the OAuth2 protocol, we keep here the
	// information she needs to send to authenticate.
	Client      auth.Client
	AccessToken auth.AccessToken

	// The OAuth ClientID refering to the host's client stored in its db
	HostClientID string
}

// ID returns the recipient qualified identifier
func (r *Recipient) ID() string { return r.RID }

// Rev returns the recipient revision
func (r *Recipient) Rev() string { return r.RRev }

// DocType returns the recipient document type
func (r *Recipient) DocType() string { return consts.Contacts }

// Clone implements couchdb.Doc
func (r *Recipient) Clone() couchdb.Doc { cloned := *r; return &cloned }

// SetID changes the recipient qualified identifier
func (r *Recipient) SetID(id string) { r.RID = id }

// SetRev changes the recipient revision
func (r *Recipient) SetRev(rev string) { r.RRev = rev }

// ExtractDomainAndScheme returns the recipient's domain and the scheme
func (r *Recipient) ExtractDomainAndScheme() (string, string, error) {
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

// CreateRecipient inserts a Recipient document in database. Email and URL must
// not be empty.
func CreateRecipient(db couchdb.Database, doc *Recipient) error {
	if len(doc.Cozy) == 0 && len(doc.Email) == 0 {
		return ErrRecipientBadParams
	}

	err := couchdb.CreateDoc(db, doc)
	return err
}

// GetRecipient returns the Recipient stored in database from a given ID
func GetRecipient(db couchdb.Database, recID string) (*Recipient, error) {
	doc := &Recipient{}
	err := couchdb.GetDoc(db, consts.Contacts, recID, doc)
	if couchdb.IsNotFoundError(err) {
		err = ErrRecipientDoesNotExist
	}
	return doc, err
}

// GetCachedRecipient returns the recipient cached in memory within
// the RecipientStatus. CAN BE NIL, Used by jsonapi
func (rs *RecipientStatus) GetCachedRecipient() *Recipient {
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

	recipientDomain, scheme, err := rs.recipient.ExtractDomainAndScheme()
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

	recipientURL, scheme, err := rs.recipient.ExtractDomainAndScheme()
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

var (
	_ couchdb.Doc = &Recipient{}
)
