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

// FindContactByShareCode returns the contact that is linked to a sharing by
// the given shareCode
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

func AcceptSharingRequest(i *instance.Instance, answerURL string) error {
	return nil

	// TODO
	// - call the answerURL
	// - from the response, create the sharing and the permissions doc
	// - if one-way sharing, add a trigger for deletes (see the code below)
	// - if two-way sharing, setup the replication in the other way

	// Code from SharingRequest
	// sharing, err := sharings.CreateSharingRequest(instance, desc, state,
	// 	sharingType, scope, clientID, appSlug)
	// if err == sharings.ErrSharingAlreadyExist {
	// 	redirectAuthorize := instance.PageURL("/auth/authorize", c.QueryParams())
	// 	return c.Redirect(http.StatusSeeOther, redirectAuthorize)
	// }
	// if err != nil {
	// 	return wrapErrors(err)
	// }
	// // Particular case for two-way: register the sharer
	// if sharingType == consts.TwoWaySharing {
	// 	if err = sharings.RegisterSharer(instance, sharing); err != nil {
	// 		return wrapErrors(err)
	// 	}
	// 	if err = sharings.SendClientID(sharing); err != nil {
	// 		return wrapErrors(err)
	// 	}
	// } else if sharing.SharingType == consts.OneWaySharing {
	// 	// The recipient listens deletes for a one-way sharing
	// 	sharingPerms, err := sharing.PermissionsSet(instance)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	for _, rule := range *sharingPerms {
	// 		err = sharings.AddTrigger(instance, rule, sharing.SID, true)
	// 		if err != nil {
	// 			return err
	// 		}
	// 	}
	// }
}

// SharingAccepted handles an accepted sharing on the sharer side and returns
// the redirect url.
func SharingAccepted(i *instance.Instance, shareCode, clientID, accessCode string) (string, error) {
	perms, err := permissions.GetForShareCode(i, shareCode)
	if err != nil {
		return "", err
	}

	s, err := GetSharingFromPermissions(i, perms)
	if err != nil {
		return "", err
	}

	m, err := s.GetMemberFromClientID(i, clientID)
	if err != nil {
		return "", err
	}

	// Update the sharing status and asks the recipient for access
	token, err := m.getAccessToken(i, accessCode)
	if err != nil {
		return "", err
	}
	m.Status = consts.SharingStatusAccepted
	m.AccessToken = *token
	if err = couchdb.UpdateDoc(i, s); err != nil {
		return "", err
	}

	// Particular case for two-way sharing: the recipient needs credentials
	if s.SharingType == consts.TwoWaySharing {
		// TODO
		// err = SendCode(instance, sharing, recStatus)
		// if err != nil {
		// 	return "", err
		// }
	}

	// Share all the documents with the recipient
	err = ShareDoc(i, s, m)
	return "", err
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
