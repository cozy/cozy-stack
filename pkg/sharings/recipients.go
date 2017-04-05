package sharings

import (
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
)

// Recipient is a struct describing a sharing recipient
type Recipient struct {
	RID   string `json:"_id,omitempty"`
	RRev  string `json:"_rev,omitempty"`
	Email string `json:"email"`
	URL   string `json:"url"`
}

// RecipientStatus contains the information about a recipient for a sharing
type RecipientStatus struct {
	Status string `json:"status,omitempty"`

	// Reference on the recipient.
	RefRecipient couchdb.DocReference `json:"recipient,omitempty"`
	recipient    *Recipient

	// The sharer is the "client", in the OAuth2 protocol, we keep here the
	// information she needs to send to authenticate.
	Client      *auth.Client
	AccessToken *auth.AccessToken
}

// ID returns the recipient qualified identifier
func (r *Recipient) ID() string { return r.RID }

// Rev returns the recipient revision
func (r *Recipient) Rev() string { return r.RRev }

// DocType returns the recipient document type
func (r *Recipient) DocType() string { return consts.Recipients }

// SetID changes the recipient qualified identifier
func (r *Recipient) SetID(id string) { r.RID = id }

// SetRev changes the recipient revision
func (r *Recipient) SetRev(rev string) { r.RRev = rev }

// ExtractDomain returns the recipient's domain without the scheme
func (r *Recipient) ExtractDomain() (string, error) {
	if r.URL == "" {
		return "", ErrRecipientHasNoURL
	}
	if tokens := strings.Split(r.URL, "://"); len(tokens) > 1 {
		return tokens[1], nil
	}
	return r.URL, nil
}

// CreateRecipient inserts a Recipient document in database. Email and URL must
// not be empty.
func CreateRecipient(db couchdb.Database, doc *Recipient) error {
	if doc.Email == "" {
		return ErrRecipientHasNoEmail
	}
	if doc.URL == "" {
		return ErrRecipientHasNoURL
	}

	err := couchdb.CreateDoc(db, doc)
	return err
}

// GetRecipient returns the Recipient stored in database from a given ID
func GetRecipient(db couchdb.Database, recID string) (*Recipient, error) {
	doc := &Recipient{}
	err := couchdb.GetDoc(db, consts.Recipients, recID, doc)
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

	if rs.recipient.URL == "" {
		return nil, ErrRecipientHasNoURL
	}
	if rs.Client.ClientID == "" {
		return nil, ErrNoOAuthClient
	}

	// The structure `auth.Request` expects a domain WITHOUT a scheme (i.e.
	// without "http://" or "https://") so we parse it.
	recipientDomain, err := rs.recipient.ExtractDomain()
	if err != nil {
		return nil, err
	}

	req := &auth.Request{
		Domain:     recipientDomain,
		HTTPClient: new(http.Client),
	}

	return req.GetAccessToken(rs.Client, code)
}

// Register asks the recipient to register the sharer as a new OAuth client.
//
// To register the sharer must provide the following information:
// - redirect uri: where the recipient must answer the sharing request. Our
//		protocol forces this field to be: "sharerdomain/sharings/answer".
// - client name: the sharer's public name.
// - client kind: "sharing" since this will be a sharing oriented OAuth client.
// - software id: the link to the github repository of the stack.
// - client URI: the domain of the sharer's Cozy.
func (rs *RecipientStatus) Register(instance *instance.Instance) error {
	// We require the recipient to be persisted in the database.
	if rs.recipient.RID == "" {
		return ErrRecipientDoesNotExist
	}

	// If the recipient has no URL there is no point in registering.
	if rs.recipient.URL == "" {
		return ErrRecipientHasNoURL
	}

	// We get the instance document to extract the public name.
	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(instance, consts.Settings, consts.InstanceSettingsID,
		doc)
	if err != nil {
		return err
	}

	sharerPublicName, _ := doc.M["public_name"].(string)
	if sharerPublicName == "" {
		return ErrPublicNameNotDefined
	}

	redirectURI := instance.PageURL("/sharings/answer", nil)
	clientURI := instance.PageURL("", nil)

	// We have all we need to register an OAuth client.
	authClient := &auth.Client{
		RedirectURIs: []string{redirectURI},
		ClientName:   sharerPublicName,
		ClientKind:   "sharing",
		SoftwareID:   "github.com/cozy/cozy-stack",
		ClientURI:    clientURI,
	}

	recipientURL, err := rs.recipient.ExtractDomain()
	if err != nil {
		return err
	}

	req := &auth.Request{
		Domain:     recipientURL,
		HTTPClient: new(http.Client),
	}

	// We launch the register process.
	resClient, err := req.RegisterClient(authClient)
	if err != nil {
		return err
	}

	rs.Client = resClient
	return nil
}

var (
	_ couchdb.Doc = &Recipient{}
)
