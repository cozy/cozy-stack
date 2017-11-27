package sharings

import (
	"io"
	"net/url"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

// SharingRequestParams contains the basic information required to request
// a sharing party
type SharingRequestParams struct {
	SharingID       string `json:"state"`
	ClientID        string `json:"client_id"`
	InboundClientID string `json:"inbound_client_id"`
	Code            string `json:"code"`
}

// GenerateOAuthQueryString takes care of creating a correct OAuth request for
// the given sharing and recipient.
func GenerateOAuthURL(i *instance.Instance, s *Sharing, m *Member, code string) (string, error) {
	// Check that we have an OAuth client that we can use
	if m.URL == "" || m.Client.ClientID == "" || len(m.Client.RedirectURIs) < 1 {
		return "", ErrNoOAuthClient
	}

	u, err := url.Parse(m.URL)
	if err != nil {
		return "", err
	}
	u.Path = "/auth/authorize"

	// Convert the local permissions doc to an OAuth scope
	if _, err = s.PermissionsSet(i); err != nil {
		return "", err
	}
	cloned := s.permissions.Clone().(*permissions.Permission)
	permSet := cloned.Permissions
	for _, rule := range permSet {
		rule.Verbs = permissions.ALL
	}
	scope, err := permSet.MarshalScopeString()
	if err != nil {
		return "", err
	}

	q := url.Values{
		consts.QueryParamAppSlug: {s.AppSlug},
		"client_id":              {m.Client.ClientID},
		"redirect_uri":           {m.Client.RedirectURIs[0]},
		"response_type":          {"cozy_sharing"},
		"scope":                  {scope},
		"state":                  {code},
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func FindContactByShareCode(i *instance.Instance, s *Sharing, code string) (*contacts.Contact, error) {
	// XXX hack to fill s.permissions
	if _, err := s.PermissionsSet(i); err != nil {
		return nil, err
	}
	contactID, ok := s.permissions.Codes[code]
	if !ok {
		return nil, ErrRecipientDoesNotExist
	}
	return GetContact(i, contactID)
}

// SharingAccepted handles an accepted sharing on the sharer side and returns
// the redirect url.
func SharingAccepted(instance *instance.Instance, state, clientID, accessCode string) (string, error) {
	sharing, recStatus, err := FindSharingRecipient(instance, state, clientID)
	if err != nil {
		return "", err
	}
	// Update the sharing status and asks the recipient for access
	recStatus.Status = consts.SharingStatusAccepted
	err = ExchangeCodeForToken(instance, sharing, recStatus, accessCode)
	if err != nil {
		return "", err
	}

	// Particular case for two-way sharing: the recipients needs credentials
	if sharing.SharingType == consts.TwoWaySharing {
		err = SendCode(instance, sharing, recStatus)
		if err != nil {
			return "", err
		}
	}
	// Share all the documents with the recipient
	err = ShareDoc(instance, sharing, recStatus)

	// Redirect the recipient after acceptation
	redirect := recStatus.contact.Cozy[0].URL
	return redirect, err
}

// CreateSharingRequest checks fields integrity and creates a sharing document
// for an incoming sharing request
func CreateSharingRequest(i *instance.Instance, desc, state, sharingType, scope, clientID, appSlug string) (*Sharing, error) {
	if state == "" {
		return nil, ErrMissingState
	}
	if err := CheckSharingType(sharingType); err != nil {
		return nil, err
	}
	if scope == "" {
		return nil, ErrMissingScope
	}
	if clientID == "" {
		return nil, ErrNoOAuthClient
	}
	permsSet, err := permissions.UnmarshalScopeString(scope)
	if err != nil {
		return nil, err
	}

	sharerClient := &oauth.Client{}
	err = couchdb.GetDoc(i, consts.OAuthClients, clientID, sharerClient)
	if err != nil {
		return nil, ErrNoOAuthClient
	}

	var res []Sharing
	// TODO don't use the by-sharing index
	err = couchdb.FindDocs(i, consts.Sharings, &couchdb.FindRequest{
		UseIndex: "by-sharing-id",
		Selector: mango.Equal("sharing_id", state),
	}, &res)
	if err == nil && len(res) > 0 {
		return nil, ErrSharingAlreadyExist
	}

	sharer := Member{
		URL:             sharerClient.ClientURI,
		InboundClientID: clientID,
		contact: &contacts.Contact{
			Cozy: []contacts.Cozy{
				contacts.Cozy{
					URL: sharerClient.ClientURI,
				},
			},
		},
	}

	sharing := &Sharing{
		AppSlug:     appSlug,
		SharingType: sharingType,
		// TODO force the ID
		// SharingID:   state,
		Owner:       false,
		Description: desc,
		Sharer:      sharer,
		Revoked:     false,
	}

	perms, err := permissions.CreateSharedWithMeSet(i, permsSet)
	if err != nil {
		return nil, err
	}
	sharing.RefPermissions = couchdb.DocReference{
		Type: perms.DocType(),
		ID:   perms.ID(),
	}
	sharing.permissions = perms

	err = couchdb.CreateDoc(i, sharing)
	return sharing, err
}

// RegisterClientOnTheRecipient is called on the owner to register its-self as
// an OAuth Client on the cozy instance of the recipient
func RegisterClientOnTheRecipient(i *instance.Instance, s *Sharing, m *Member, u *url.URL) error {
	err := m.RegisterClient(i, u)
	if err != nil {
		i.Logger().Errorf("[sharing] Could not register at %s: %v", u, err)
		return err
	}
	m.Status = consts.SharingStatusMailNotSent
	return couchdb.UpdateDoc(i, s)
}

// RegisterSharer registers the sharer for two-way sharing
func RegisterSharer(instance *instance.Instance, sharing *Sharing) error {
	// Register the sharer as a recipient
	sharer := sharing.Sharer
	doc := &contacts.Contact{
		Cozy: []contacts.Cozy{
			contacts.Cozy{
				URL: sharer.URL,
			},
		},
	}
	err := CreateOrUpdateRecipient(instance, doc)
	if err != nil {
		return err
	}
	ref := couchdb.DocReference{
		ID:   doc.ID(),
		Type: consts.Contacts,
	}
	sharer.RefContact = ref
	// TODO err = sharer.RegisterClient(instance)
	if err != nil {
		instance.Logger().Error("[sharing] Could not register at "+sharer.URL+" ", err)
		return err
	}
	return couchdb.UpdateDoc(instance, sharing)
}

// SendClientID sends the registered clientId to the sharer
func SendClientID(sharing *Sharing) error {
	domain, scheme, err := ExtractDomainAndScheme(sharing.Sharer.contact)
	if err != nil {
		return nil
	}
	path := "/sharings/access/client"
	newClientID := sharing.Sharer.Client.ClientID
	params := SharingRequestParams{
		SharingID:       sharing.SID,
		ClientID:        sharing.Sharer.InboundClientID,
		InboundClientID: newClientID,
	}
	return Request("POST", domain, scheme, path, params)
}

// SendCode generates and sends an OAuth code to a recipient
func SendCode(instance *instance.Instance, sharing *Sharing, recStatus *Member) error {
	permSet, err := sharing.PermissionsSet(instance)
	if err != nil {
		return err
	}
	// TODO check if changing the HTTP verbs to ALL is needed
	scope, err := permSet.MarshalScopeString()
	if err != nil {
		return err
	}
	clientID := recStatus.Client.ClientID
	access, err := oauth.CreateAccessCode(instance, clientID, scope)
	if err != nil {
		return err
	}
	domain, scheme, err := ExtractDomainAndScheme(recStatus.contact)
	if err != nil {
		return nil
	}
	path := "/sharings/access/code"
	params := SharingRequestParams{
		SharingID: sharing.SID,
		Code:      access.Code,
	}
	return Request("POST", domain, scheme, path, params)
}

// ExchangeCodeForToken asks for an AccessToken based on an AccessCode
func ExchangeCodeForToken(instance *instance.Instance, sharing *Sharing, recStatus *Member, code string) error {
	// Fetch the access and refresh tokens.
	access, err := recStatus.getAccessToken(instance, code)
	if err != nil {
		return err
	}
	recStatus.AccessToken = *access
	return couchdb.UpdateDoc(instance, sharing)
}

// Request is a utility method to send request to remote sharing party
func Request(method, domain, scheme, path string, params interface{}) error {
	var body io.Reader
	var err error
	if params != nil {
		body, err = request.WriteJSON(params)
		if err != nil {
			return nil
		}
	}
	_, err = request.Req(&request.Options{
		Domain: domain,
		Scheme: scheme,
		Method: method,
		Path:   path,
		Headers: request.Headers{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Body: body,
	})
	return err
}
