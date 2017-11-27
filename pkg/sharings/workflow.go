package sharings

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/labstack/echo"
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

// RevokeSharing revokes the sharing and deletes all the OAuth client and
// triggers associated with it.
//
// Revoking a sharing consists of setting the field `Revoked` to `true`.
// When the sharing is of type "two-way" both recipients and sharer have
// trigger(s) and OAuth client(s) to delete.
// In every other cases only the sharer has trigger(s) to delete and only the
// recipients have an OAuth client to delete.
//
// When this function is called it needs to call either `RevokerSharing` or
// `RevokeRecipient` depending on who initiated the revocation. This is
// represented by the `recursive` boolean parameter. The first call has this set
// to `true` while the subsequent call has it set to `false`.
func RevokeSharing(ins *instance.Instance, sharing *Sharing, recursive bool) error {
	var err error
	if sharing.Owner {
		for _, rs := range sharing.Recipients {
			if recursive {
				err = askToRevokeSharing(ins, sharing, &rs)
				if err != nil {
					continue
				}
			}

			if sharing.SharingType == consts.TwoWaySharing {
				err = deleteOAuthClient(ins, &rs)
				if err != nil {
					continue
				}
			}
		}

		err = removeSharingTriggers(ins, sharing.SID)
		if err != nil {
			return err
		}

	} else {
		if recursive {
			err = askToRevokeRecipient(ins, sharing, &sharing.Sharer)
			if err != nil {
				return err
			}
		}

		err = deleteOAuthClient(ins, &sharing.Sharer)
		if err != nil {
			return err
		}

		if sharing.SharingType == consts.TwoWaySharing {
			err = removeSharingTriggers(ins, sharing.SID)
			if err != nil {
				return err
			}
		}
	}
	sharing.Revoked = true
	ins.Logger().Debugf("[sharings] Setting status of sharing %s to revoked",
		sharing.SID)
	return couchdb.UpdateDoc(ins, sharing)
}

// RevokeRecipientByContactID revokes a recipient from the given sharing. Only the sharer
// can make this action.
func RevokeRecipientByContactID(ins *instance.Instance, sharing *Sharing, contactID string) error {
	if !sharing.Owner {
		return ErrOnlySharerCanRevokeRecipient
	}

	for _, rs := range sharing.Recipients {
		c := rs.Contact(ins)
		if c != nil && c.DocID == contactID {
			// TODO check how askToRevokeRecipient behave when no recipient has accepted the sharing
			if err := askToRevokeSharing(ins, sharing, &rs); err != nil {
				return err
			}
			return RevokeRecipient(ins, sharing, &rs)
		}
	}

	return nil
}

// RevokeRecipientByClientID revokes a recipient from the given sharing. Only the sharer
// can make this action.
func RevokeRecipientByClientID(ins *instance.Instance, sharing *Sharing, clientID string) error {
	if !sharing.Owner {
		return ErrOnlySharerCanRevokeRecipient
	}

	for _, rs := range sharing.Recipients {
		if rs.Client.ClientID == clientID {
			return RevokeRecipient(ins, sharing, &rs)
		}
	}

	return nil
}

// RevokeRecipient revokes a recipient from the given sharing. Only the sharer
// can make this action.
//
// If the sharing is of type "two-way" the sharer also has to remove the
// recipient's OAuth client.
//
// If there are no more recipients the sharing is revoked and the corresponding
// trigger is deleted.
func RevokeRecipient(ins *instance.Instance, sharing *Sharing, recipient *Member) error {
	if sharing.SharingType == consts.TwoWaySharing {
		if err := deleteOAuthClient(ins, recipient); err != nil {
			return err
		}
		recipient.InboundClientID = ""
	}

	recipient.Client = auth.Client{}
	recipient.AccessToken = auth.AccessToken{}
	recipient.Status = consts.SharingStatusRevoked

	toRevoke := true
	for _, recipient := range sharing.Recipients {
		if recipient.Status != consts.SharingStatusRevoked {
			toRevoke = false
		}
	}

	if toRevoke {
		// TODO check how removeSharingTriggers behave when no recipient has accepted the sharing
		if err := removeSharingTriggers(ins, sharing.SID); err != nil {
			ins.Logger().Errorf("[sharings] RevokeRecipient: Could not remove "+
				"triggers for sharing %s: %s", sharing.SID, err)
		}
		sharing.Revoked = true
	}

	return couchdb.UpdateDoc(ins, sharing)
}

func deleteOAuthClient(ins *instance.Instance, rs *Member) error {
	client, err := oauth.FindClient(ins, rs.InboundClientID)
	if err != nil {
		ins.Logger().Errorf("[sharings] deleteOAuthClient: Could not "+
			"find OAuth client %s: %s", rs.InboundClientID, err)
		return err
	}
	crErr := client.Delete(ins)
	if crErr != nil {
		ins.Logger().Errorf("[sharings] deleteOAuthClient: Could not "+
			"delete OAuth client %s: %s", rs.InboundClientID, err)
		return errors.New(crErr.Error)
	}

	ins.Logger().Debugf("[sharings] OAuth client %s deleted", rs.InboundClientID)
	rs.InboundClientID = ""
	return nil
}

func askToRevokeSharing(ins *instance.Instance, sharing *Sharing, rs *Member) error {
	return askToRevoke(ins, sharing, rs, "")
}

func askToRevokeRecipient(ins *instance.Instance, sharing *Sharing, rs *Member) error {
	// TODO: If the recipient revoke a one-way sharing, he  cannot request
	// the sharer yet, as he have no credentials
	if rs.RefContact.ID != "" {
		return askToRevoke(ins, sharing, rs, rs.Client.ClientID)
	}
	return nil

}

// TODO Once we will handle error properly (recipient is disconnected and
// what not) analyze the error returned and take proper actions every time this
// function is called.
func askToRevoke(ins *instance.Instance, sharing *Sharing, rs *Member, recipientClientID string) error {
	sharingID := sharing.SID
	c := rs.Contact(ins)
	if c == nil {
		ins.Logger().Errorf("[sharings] askToRevoke: Could not fetch "+
			"recipient %s from database", rs.RefContact.ID)
		return ErrRecipientDoesNotExist
	}

	var path string
	if recipientClientID == "" {
		path = fmt.Sprintf("/sharings/%s", sharingID)
	} else {
		// From the recipient point of view, only a two-way sharing
		// grants him the rights to request the sharer, as he doesn't have
		// any credentials otherwise.
		if sharing.SharingType == consts.TwoWaySharing {
			path = fmt.Sprintf("/sharings/%s/recipient/%s", sharingID,
				rs.InboundClientID)
		} else {
			return nil
		}
	}
	domain, scheme, err := ExtractDomainAndScheme(rs.contact)
	if err != nil {
		return err
	}

	reqOpts := &request.Options{
		Domain:  domain,
		Scheme:  scheme,
		Path:    path,
		Method:  http.MethodDelete,
		Queries: url.Values{consts.QueryParamRecursive: {"false"}},
		Headers: request.Headers{
			echo.HeaderAuthorization: fmt.Sprintf("Bearer %s",
				rs.AccessToken.AccessToken),
		},
		Body:       nil,
		NoResponse: true,
	}

	_, err = request.Req(reqOpts)

	if err != nil {
		if IsAuthError(err) {
			recInfo, errInfo := ExtractRecipientInfo(ins, rs)
			if errInfo != nil {
				return errInfo
			}
			_, err = RefreshTokenAndRetry(ins, sharingID, recInfo, reqOpts)
		}
		if err != nil {
			ins.Logger().Errorf("[sharings] askToRevoke: Could not ask recipient "+
				"%s to revoke sharing %s: %v", rs.contact.Cozy[0].URL, sharingID, err)
		}
		return err
	}

	rs.Client = auth.Client{}
	rs.AccessToken = auth.AccessToken{}
	return nil
}

// RefreshTokenAndRetry is called after an authentication failure.
// It tries to renew the access_token and request again
func RefreshTokenAndRetry(ins *instance.Instance, sharingID string, rec *RecipientInfo, opts *request.Options) (*http.Response, error) {
	ins.Logger().Errorf("[sharing] The request is not authorized. "+
		"Trying to renew the token for %v", rec.URL)

	req := &auth.Request{
		Domain:     opts.Domain,
		Scheme:     opts.Scheme,
		HTTPClient: new(http.Client),
	}
	sharing, recStatus, err := FindSharingRecipient(ins, sharingID, rec.Client.ClientID)
	if err != nil {
		return nil, err
	}
	refreshToken := rec.AccessToken.RefreshToken
	access, err := req.RefreshToken(&rec.Client, &rec.AccessToken)
	if err != nil {
		ins.Logger().Errorf("[sharing] Refresh token request failed: %v", err)
		return nil, err
	}
	access.RefreshToken = refreshToken
	recStatus.AccessToken = *access
	if err = couchdb.UpdateDoc(ins, sharing); err != nil {
		return nil, err
	}
	opts.Headers["Authorization"] = "Bearer " + access.AccessToken
	res, err := request.Req(opts)
	return res, err
}

// IsAuthError returns true if the given error is an authentication one
func IsAuthError(err error) bool {
	if v, ok := err.(*request.Error); ok {
		return v.Title == "Bad Request" || v.Title == "Unauthorized"
	}
	return false
}
