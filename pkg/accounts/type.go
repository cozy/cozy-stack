package accounts

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
)

// This file contains the account_type object as defined in
// docs/konnectors_oauth

// Various grant types
// - AuthorizationCode is the server-side grant type.
// - ImplicitGrant is the implicit grant type
// - ImplicitGrantRedirectURL is the implicit grant type but with redirect_url
//    											  instead of redirect_uri
const (
	AuthorizationCode        = "authorization_code"
	ImplicitGrant            = "token"
	ImplicitGrantRedirectURL = "token_redirect_url"
)

// Token Request authentication modes for AuthorizationCode grant type
// normal is through form parameters
// some services requires it as Basic
const (
	FormTokenAuthMode  = "form"
	BasicTokenAuthMode = "basic"
)

// RefreshToken is the refresh grant type
var RefreshToken = "refresh_token"

// ErrUnrefreshable is the error when an account type or information
// within an account does not allow refreshing it.
var ErrUnrefreshable = errors.New("this account can not be refreshed")

// AccountType holds configuration information for
type AccountType struct {
	DocID                 string            `json:"_id,omitempty"`
	DocRev                string            `json:"_rev,omitempty"`
	GrantMode             string            `json:"grant_mode,omitempty"`
	ClientID              string            `json:"client_id,omitempty"`
	ClientSecret          string            `json:"client_secret,omitempty"`
	AuthEndpoint          string            `json:"auth_endpoint,omitempty"`
	TokenEndpoint         string            `json:"token_endpoint,omitempty"`
	TokenAuthMode         string            `json:"token_mode,omitempty"`
	RegisteredRedirectURI string            `json:"redirect_uri,omitempty"`
	ExtraAuthQuery        map[string]string `json:"extras,omitempty"`
}

// ID is used to implement the couchdb.Doc interface
func (at *AccountType) ID() string { return at.DocID }

// Rev is used to implement the couchdb.Doc interface
func (at *AccountType) Rev() string { return at.DocRev }

// SetID is used to implement the couchdb.Doc interface
func (at *AccountType) SetID(id string) { at.DocID = id }

// SetRev is used to implement the couchdb.Doc interface
func (at *AccountType) SetRev(rev string) { at.DocRev = rev }

// DocType implements couchdb.Doc
func (at *AccountType) DocType() string { return consts.AccountTypes }

// Clone implements couchdb.Doc
func (at *AccountType) Clone() couchdb.Doc {
	cloned := *at
	cloned.ExtraAuthQuery = make(map[string]string)
	for k, v := range at.ExtraAuthQuery {
		cloned.ExtraAuthQuery[k] = v
	}
	return &cloned
}

// ensure AccountType implements couchdb.Doc
var _ couchdb.Doc = (*AccountType)(nil)

type tokenEndpointResponse struct {
	RefreshToken     string `json:"refresh_token"`
	AccessToken      string `json:"access_token"`
	IDToken          string `json:"id_token"` // alternative name for access_token
	ExpiresIn        int    `json:"expires_in"`
	TokenType        string `json:"token_type"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// RedirectURI returns the redirectURI for an account,
// it can be either the
func (at *AccountType) RedirectURI(i *instance.Instance) string {
	redirectURI := i.PageURL("/accounts/"+at.ID()+"/redirect", nil)
	if at.RegisteredRedirectURI != "" {
		redirectURI = at.RegisteredRedirectURI
	}
	return redirectURI
}

// MakeOauthStartURL returns the url at which direct the user to start
// the oauth flow
func (at *AccountType) MakeOauthStartURL(i *instance.Instance, scope string, state string) (string, error) {

	u, err := url.Parse(at.AuthEndpoint)
	if err != nil {
		return "", err
	}
	vv := &url.Values{}

	redirectURI := at.RedirectURI(i)

	switch at.GrantMode {
	case AuthorizationCode:
		vv.Add("scope", scope)
		vv.Add("response_type", "code")
		vv.Add("client_id", at.ClientID)
		vv.Add("state", state)
		vv.Add("redirect_uri", redirectURI)
	case ImplicitGrant:
		vv.Add("scope", scope)
		vv.Add("response_type", "token")
		vv.Add("client_id", at.ClientID)
		vv.Add("state", state)
		vv.Add("redirect_uri", redirectURI)
	case ImplicitGrantRedirectURL:
		vv.Add("scope", scope)
		vv.Add("response_type", "token")
		vv.Add("state", state)
		vv.Add("redirect_url", redirectURI)
	default:
		return "", errors.New("Wrong account type")
	}

	for k, v := range at.ExtraAuthQuery {
		vv.Add(k, v)
	}

	u.RawQuery = vv.Encode()
	return u.String(), nil

}

// RefreshAccount requires a new AccessToken using the RefreshToken
// as specified in https://tools.ietf.org/html/rfc6749#section-6
func (at *AccountType) RefreshAccount(a Account) error {

	if a.Oauth == nil || a.Oauth.RefreshToken == "" {
		return ErrUnrefreshable
	}

	res, err := http.PostForm(at.TokenEndpoint, url.Values{
		"grant_type":    []string{RefreshToken},
		"refresh_token": []string{a.Oauth.RefreshToken},
		"client_id":     []string{at.ClientID},
		"client_secret": []string{at.ClientSecret},
	})

	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		return errors.New("oauth services responded with non-200 res")
	}

	var out tokenEndpointResponse
	err = json.NewDecoder(res.Body).Decode(&out)
	if err != nil {
		return err
	}

	if out.Error != "" {
		return fmt.Errorf("OauthError(%s) %s", out.Error, out.ErrorDescription)
	}

	if out.AccessToken != "" {
		a.Oauth.AccessToken = out.AccessToken
	}

	if out.ExpiresIn != 0 {
		a.Oauth.ExpiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	}

	if out.RefreshToken != "" {
		a.Oauth.RefreshToken = out.RefreshToken
	}

	return nil
}

// TypeInfo returns the AccountType document for a given id
func TypeInfo(id string) (*AccountType, error) {
	if id == "" {
		return nil, errors.New("no account type id provided")
	}
	var a AccountType
	err := couchdb.GetDoc(couchdb.GlobalSecretsDB, consts.AccountTypes, id, &a)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
