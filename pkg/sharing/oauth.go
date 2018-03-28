package sharing

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

// CreateSharingRequest sends information about the sharing to the recipient's cozy
func (m *Member) CreateSharingRequest(inst *instance.Instance, s *Sharing, u *url.URL) error {
	// TODO translate ids of files/folders in the rules sent to the recipients
	// TODO skip local rules
	sh := APISharing{
		&Sharing{
			SID:         s.SID,
			Active:      false,
			Owner:       false,
			Open:        s.Open,
			Description: s.Description,
			AppSlug:     s.AppSlug,
			PreviewPath: s.PreviewPath,
			CreatedAt:   s.CreatedAt,
			UpdatedAt:   s.UpdatedAt,
			Rules:       s.Rules,
			Members:     s.Members,
		},
		nil,
	}
	data, err := jsonapi.MarshalObject(&sh)
	if err != nil {
		return err
	}
	body, err := json.Marshal(jsonapi.Document{Data: &data})
	if err != nil {
		return err
	}
	res, err := request.Req(&request.Options{
		Method: http.MethodPut,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/" + s.SID,
		Headers: request.Headers{
			"Accept":       "application/vnd.api+json",
			"Content-Type": "application/vnd.api+json",
		},
		Body: bytes.NewReader(body),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		return ErrRequestFailed
	}

	return nil
}

// RegisterCozyURL saves a new Cozy URL for a member
func (s *Sharing) RegisterCozyURL(inst *instance.Instance, m *Member, cozyURL string) error {
	if !s.Owner {
		return ErrInvalidSharing
	}

	u, err := url.Parse(strings.TrimSpace(cozyURL))
	if err != nil || u.Host == "" {
		return ErrInvalidURL
	}
	if u.Scheme == "" {
		u.Scheme = "https" // Set https as the default scheme
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	m.Instance = u.String()

	if err = m.CreateSharingRequest(inst, s, u); err != nil {
		inst.Logger().Warnf("[sharing] Error on sharing request: %s", err)
		return ErrRequestFailed
	}
	return couchdb.UpdateDoc(inst, s)
}

// GenerateOAuthURL takes care of creating a correct OAuth request for
// the given member of the sharing.
func (m *Member) GenerateOAuthURL(s *Sharing) (string, error) {
	if !s.Owner || len(s.Members) != len(s.Credentials)+1 {
		return "", ErrInvalidSharing
	}
	var creds *Credentials
	for i, member := range s.Members {
		if *m == member {
			creds = &s.Credentials[i-1]
		}
	}
	if creds == nil {
		return "", ErrInvalidSharing
	}
	if m.Instance == "" {
		return "", ErrNoOAuthClient
	}

	u, err := url.Parse(m.Instance)
	if err != nil {
		return "", err
	}
	u.Path = "/auth/authorize/sharing"

	q := url.Values{
		"sharing_id": {s.SID},
		"state":      {creds.State},
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// CreateOAuthClient creates an OAuth client for a recipient of the given sharing
func CreateOAuthClient(inst *instance.Instance, m *Member) (*oauth.Client, error) {
	if m.Instance == "" {
		return nil, ErrInvalidURL
	}
	cli := oauth.Client{
		RedirectURIs: []string{m.Instance + "/sharings/answer"},
		ClientName:   "Sharing " + m.Name,
		ClientKind:   "sharing",
		SoftwareID:   "github.com/cozy/cozy-stack",
		ClientURI:    m.Instance + "/",
	}
	if err := cli.Create(inst); err != nil {
		return nil, ErrInternalServerError
	}
	return &cli, nil
}

// ConvertOAuthClient converts an OAuth client from one type (pkg/oauth.Client)
// to another (client/auth.Client)
func ConvertOAuthClient(c *oauth.Client) *auth.Client {
	return &auth.Client{
		ClientID:          c.ClientID,
		ClientSecret:      c.ClientSecret,
		SecretExpiresAt:   c.SecretExpiresAt,
		RegistrationToken: c.RegistrationToken,
		RedirectURIs:      c.RedirectURIs,
		ClientName:        c.ClientName,
		ClientKind:        c.ClientKind,
		ClientURI:         c.ClientURI,
		LogoURI:           c.LogoURI,
		PolicyURI:         c.PolicyURI,
		SoftwareID:        c.SoftwareID,
		SoftwareVersion:   c.SoftwareVersion,
	}
}

// CreateAccessToken creates an access token for the given OAuth client,
// with a scope on this sharing.
func CreateAccessToken(inst *instance.Instance, cli *oauth.Client, sharingID string) (*auth.AccessToken, error) {
	scope := consts.Sharings + ":ALL:" + sharingID
	cli.CouchID = cli.ClientID // XXX CouchID is required by CreateJWT
	refresh, err := cli.CreateJWT(inst, permissions.RefreshTokenAudience, scope)
	if err != nil {
		return nil, err
	}
	access, err := cli.CreateJWT(inst, permissions.AccessTokenAudience, scope)
	if err != nil {
		return nil, err
	}
	return &auth.AccessToken{
		TokenType:    "bearer",
		AccessToken:  access,
		RefreshToken: refresh,
		Scope:        scope,
	}, nil
}

// SendAnswer says to the sharer's Cozy that the sharing has been accepted, and
// materialize that by an exchange of credentials.
func (s *Sharing) SendAnswer(inst *instance.Instance, state string) error {
	if s.Owner || len(s.Members) < 2 || len(s.Credentials) != 1 {
		return ErrInvalidSharing
	}
	u, err := url.Parse(s.Members[0].Instance)
	if s.Members[0].Instance == "" || err != nil {
		return ErrInvalidSharing
	}
	cli, err := CreateOAuthClient(inst, &s.Members[0])
	if err != nil {
		return err
	}
	token, err := CreateAccessToken(inst, cli, s.SID)
	if err != nil {
		return err
	}
	ac := APICredentials{
		Credentials: &Credentials{
			State:       state,
			Client:      ConvertOAuthClient(cli),
			AccessToken: token,
		},
		CID: s.SID,
	}
	data, err := jsonapi.MarshalObject(&ac)
	if err != nil {
		return err
	}
	body, err := json.Marshal(jsonapi.Document{Data: &data})
	if err != nil {
		return err
	}
	res, err := request.Req(&request.Options{
		Method: http.MethodPost,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/" + s.SID + "/answer",
		Headers: request.Headers{
			"Accept":       "application/vnd.api+json",
			"Content-Type": "application/vnd.api+json",
		},
		Body: bytes.NewReader(body),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return ErrRequestFailed
	}

	if !s.ReadOnly() {
		var creds Credentials
		if _, err = jsonapi.Bind(res.Body, &creds); err != nil {
			return ErrRequestFailed
		}
		s.Credentials[0].AccessToken = creds.AccessToken
		s.Credentials[0].Client = creds.Client
		return couchdb.UpdateDoc(inst, s)
	}

	return nil
}

// ProcessAnswer takes somes credentials and update the sharing with those.
func (s *Sharing) ProcessAnswer(inst *instance.Instance, creds *Credentials) (*APICredentials, error) {
	if !s.Owner || len(s.Members) != len(s.Credentials)+1 {
		return nil, ErrInvalidSharing
	}
	for i, c := range s.Credentials {
		if c.State == creds.State {
			s.Members[i+1].Status = MemberStatusReady
			s.Credentials[i].Client = creds.Client
			s.Credentials[i].AccessToken = creds.AccessToken
			if err := couchdb.UpdateDoc(inst, s); err != nil {
				return nil, err
			}
			ac := APICredentials{CID: s.SID, Credentials: &Credentials{}}
			if !s.ReadOnly() {
				cli, err := CreateOAuthClient(inst, &s.Members[i+1])
				if err != nil {
					return &ac, nil
				}
				ac.Credentials.Client = ConvertOAuthClient(cli)
				token, err := CreateAccessToken(inst, cli, s.SID)
				if err != nil {
					return &ac, nil
				}
				ac.Credentials.AccessToken = token
			}
			go s.Setup(inst, &s.Members[i+1])
			return &ac, nil
		}
	}
	return nil, ErrMemberNotFound
}
