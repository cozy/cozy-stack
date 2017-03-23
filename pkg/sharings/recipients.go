package sharings

import (
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

// Recipient is a struct describing a sharing recipient
type Recipient struct {
	RID    string `json:"_id,omitempty"`
	RRev   string `json:"_rev,omitempty"`
	Email  string `json:"email"`
	URL    string `json:"url"`
	Client *auth.Client
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

// Relationships implements jsonapi.Doc
func (r *Recipient) Relationships() jsonapi.RelationshipMap { return nil }

// Included implements jsonapi.Doc
func (r *Recipient) Included() []jsonapi.Object { return nil }

// Links implements jsonapi.Doc
func (r *Recipient) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/recipients/" + r.RID}
}

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

// GetAccessToken sends an access_token requests to the recipient, using the
// given authorization code
func (r *Recipient) GetAccessToken(code string) (*auth.AccessToken, error) {
	client := new(http.Client)
	rURL, err := r.ExtractDomain()
	if err != nil {
		return nil, err
	}
	req := &auth.Request{
		Domain:     rURL,
		HTTPClient: client,
	}
	return req.GetAccessToken(r.Client, code)
}

// Register creates a OAuth request and register to the Recipient
func (r *Recipient) Register(instance *instance.Instance) error {
	rURL, err := r.ExtractDomain()
	if err != nil {
		return err
	}
	client := new(http.Client)
	req := &auth.Request{
		Domain:     rURL,
		HTTPClient: client,
	}
	redirectURI := instance.Scheme() + "://" + instance.Domain + "/sharings/answer"

	// Get the Cozy's public name
	doc := &couchdb.JSONDoc{}
	err = couchdb.GetDoc(instance, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return err
	}
	sharerPublicName, _ := doc.M["public_name"].(string)
	if sharerPublicName == "" {
		return ErrPublicNameNotDefined
	}

	authClient := &auth.Client{
		RedirectURIs: []string{redirectURI},
		ClientName:   sharerPublicName,
		ClientKind:   "sharing",
		SoftwareID:   "github.com/cozy/cozy-stack",
		ClientURI:    instance.Domain,
	}

	resClient, err := req.RegisterClient(authClient)
	if err != nil {
		return err
	}

	r.Client = resClient
	return couchdb.UpdateDoc(instance, r)
}

// CreateRecipient inserts a Recipient document in database
func CreateRecipient(db couchdb.Database, doc *Recipient) error {
	err := couchdb.CreateDoc(db, doc)
	return err
}

var (
	_ couchdb.Doc    = &Recipient{}
	_ jsonapi.Object = &Recipient{}
)
