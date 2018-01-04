package sharings

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

// FindContactByShareCode returns the contact that is linked to a sharing by
// the given shareCode
func FindContactByShareCode(i *instance.Instance, s *Sharing, code string) (*contacts.Contact, error) {
	perms, err := s.Permissions(i)
	if err != nil {
		return nil, err
	}
	for contactID, c := range perms.Codes {
		if c == code {
			return contacts.Find(i, contactID)
		}
	}
	return nil, ErrRecipientDoesNotExist
}

// extractScopeFromPermissions returns a scope string from a permissions doc
// XXX we force the HTTP verbs to ALL for the needs of the replication between
// the two cozy instances
func extractScopeFromPermissions(p *permissions.Permission) (string, error) {
	cloned := p.Clone().(*permissions.Permission)
	permSet := cloned.Permissions
	for _, rule := range permSet {
		rule.Verbs = permissions.ALL
	}
	return permSet.MarshalScopeString()
}

// GenerateOAuthURL takes care of creating a correct OAuth request for
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
	perms, err := s.Permissions(i)
	if err != nil {
		return "", err
	}
	scope, err := extractScopeFromPermissions(perms)
	if err != nil {
		return "", err
	}

	q := url.Values{
		consts.QueryParamAppSlug: {s.AppSlug},
		"client_id":              {m.Client.ClientID},
		"redirect_uri":           {m.Client.RedirectURIs[0]},
		"response_type":          {consts.SharingResponseType},
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
	return couchdb.UpdateDoc(i, s)
}

// AcceptSharingRequest is called on the recipient when the permissions for the
// sharing are accepted. It calls the cozy of the owner to start the sharing,
// and then create the sharing (and other stuff) in its couchdb.
func AcceptSharingRequest(i *instance.Instance, answerURL *url.URL, scope string) error {
	res, err := request.Req(&request.Options{
		Method:  http.MethodPost,
		Scheme:  answerURL.Scheme,
		Domain:  answerURL.Host,
		Path:    answerURL.Path,
		Queries: answerURL.Query(),
		Headers: request.Headers{"Accept": "application/json"},
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	sharing := &Sharing{}
	if err = json.NewDecoder(res.Body).Decode(sharing); err != nil {
		return err
	}

	if err = CheckSharingType(sharing.SharingType); err != nil {
		return err
	}
	sharing.Owner = false
	// TODO sharing.Sharer.RefContact = ...
	if err = couchdb.CreateNamedDoc(i, sharing); err != nil {
		return err
	}
	permsSet, err := permissions.UnmarshalScopeString(scope)
	if err != nil {
		return err
	}
	perms, err := permissions.CreateSharedWithMeSet(i, sharing.SID, permsSet)
	if err != nil {
		return err
	}
	sharing.permissions = perms

	// Add triggers on the recipient side for each rule
	if sharing.SharingType != consts.OneShotSharing {
		for _, rule := range perms.Permissions {
			// On a one-way sharing, we observe deletions because we assume
			// that deleting the main shared folder means that we should revoke
			// the sharing
			delTrigger := sharing.SharingType == consts.OneWaySharing
			if err = AddTrigger(i, rule, sharing.SID, delTrigger); err != nil {
				return err
			}
		}
	}

	return nil
}

// SharingAccepted handles an accepted sharing on the sharer side and returns
// the sharing.
func SharingAccepted(i *instance.Instance, shareCode, clientID, accessCode string) (*Sharing, error) {
	if shareCode == "" {
		return nil, ErrMissingCode
	}

	perms, err := permissions.GetForShareCode(i, shareCode)
	if err != nil {
		return nil, ErrForbidden
	}

	s, err := GetSharingFromPermissions(i, perms)
	if err != nil {
		return nil, err
	}

	m, err := s.GetMemberFromClientID(i, clientID)
	if err != nil {
		return nil, err
	}
	if m.Status != consts.SharingStatusPending {
		return nil, ErrSharingAlreadyExist
	}

	// Update the sharing status and asks the recipient for access
	token, err := m.GetAccessToken(accessCode)
	if err != nil {
		return nil, err
	}
	m.Status = consts.SharingStatusAccepted
	m.AccessToken = *token
	if err = couchdb.UpdateDoc(i, s); err != nil {
		return nil, err
	}

	res := &Sharing{
		SID:         s.SID,
		SharingType: s.SharingType,
		Sharer: &Member{
			Status:          consts.SharingStatusAccepted,
			URL:             i.PageURL("", nil),
			InboundClientID: clientID,
		},
		Description: s.Description,
		AppSlug:     s.AppSlug,
	}

	// Particular case for two-way sharing: the recipient needs credentials
	if s.SharingType == consts.TwoWaySharing {
		// Create an OAuth client
		name := m.URL
		if contact := m.Contact(i); contact != nil {
			if addr, errc := contact.ToMailAddress(); errc != nil {
				name = addr.Name
			}
		}
		redirectURI := fmt.Sprintf("%s/%s", m.URL, "/sharings/answer")
		c := oauth.Client{
			RedirectURIs: []string{redirectURI},
			ClientName:   name,
			ClientKind:   "sharing",
			SoftwareID:   "github.com/cozy/cozy-stack",
			ClientURI:    m.URL,
		}
		if err := c.Create(i); err != nil {
			return nil, ErrNoOAuthClient
		}
		res.Sharer.Client = auth.Client{
			ClientID:          c.ClientID,
			ClientSecret:      c.ClientSecret,
			SecretExpiresAt:   c.SecretExpiresAt,
			RegistrationToken: c.RegistrationToken,
			RedirectURIs:      c.RedirectURIs,
			ClientName:        c.ClientName,
			ClientKind:        c.ClientKind,
			ClientURI:         c.ClientURI,
			SoftwareID:        c.SoftwareID,
		}

		// And an OAuth access token
		scope, errb := extractScopeFromPermissions(perms)
		if errb != nil {
			return nil, errb
		}
		c.CouchID = c.ClientID
		access, errb := c.CreateJWT(i, permissions.AccessTokenAudience, scope)
		if errb != nil {
			return nil, errb
		}
		refresh, errb := c.CreateJWT(i, permissions.RefreshTokenAudience, scope)
		if errb != nil {
			return nil, errb
		}
		res.Sharer.AccessToken = auth.AccessToken{
			TokenType:    "bearer",
			AccessToken:  access,
			RefreshToken: refresh,
			Scope:        scope,
		}
	}

	// Share all the documents with the recipient
	go func() {
		time.Sleep(1 * time.Second)
		if errs := ShareDoc(i, s, m); errs != nil {
			i.Logger().Infof("[sharings] Cannot setup the initial share docs: %s", errs)
		}
	}()
	return res, nil
}
