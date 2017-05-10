package accounts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// This file contains the account_type object as defined in
// docs/konnectors_oauth

// AuthorizationCode is the server-side grant type.
var AuthorizationCode = "authorization_code"

// RefreshToken is the refresh grant type
var RefreshToken = "refresh_token"

// ErrUnrefreshable is the error when an account type or information
// within an account does not allow refreshing it.
var ErrUnrefreshable = errors.New("this account can not be refreshed")

// AccountType holds configuration information for
type AccountType struct {
	DocID                 string
	DocRev                string
	GrantMode             string
	ClientID              string
	ClientSecret          string
	AuthEndpoint          string
	TokenEndpoint         string
	RegisteredRedirectURI string
	ExtraAuthQuery        map[string]string
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
func (at *AccountType) Clone() couchdb.Doc { cloned := *at; return &cloned }

// ensure AccountType implements couchdb.Doc
var _ couchdb.Doc = (*AccountType)(nil)

type tokenEndpointResponse struct {
	RefreshToken     string `json:"refresh_token"`
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	TokenType        string `json:"token_type"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// MakeOauthStartURL returns the url at which direct the user to start
// the oauth flow
func (at *AccountType) MakeOauthStartURL(scope string, state string) (string, error) {

	u, err := url.Parse(at.AuthEndpoint)
	if err != nil {
		return "", err
	}
	vv := &url.Values{}
	vv.Add("scope", scope)
	vv.Add("response_type", "code")
	vv.Add("client_id", at.ClientID)
	vv.Add("state", state)
	vv.Add("redirect_uri", at.RegisteredRedirectURI)

	for k, v := range at.ExtraAuthQuery {
		vv.Add(k, v)
	}

	u.RawQuery = vv.Encode()
	return u.String(), nil

}

// AccessCodeToAccessToken exchange an access code for an Access Token
// as defined in https://tools.ietf.org/html/rfc6749#section-4.1.3
func (at *AccountType) AccessCodeToAccessToken(authcode string) (*Account, error) {
	res, err := http.PostForm(at.TokenEndpoint, url.Values{
		"grant_type":    []string{AuthorizationCode},
		"code":          []string{authcode},
		"redirect_uri":  []string{at.RegisteredRedirectURI},
		"client_id":     []string{at.ClientID},
		"client_secret": []string{at.ClientSecret},
	})

	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		return nil, errors.New("oauth services responded with non-200 res")
	}

	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var out tokenEndpointResponse
	err = json.Unmarshal(resBody, &out)
	if err != nil {
		return nil, err
	}

	if out.Error != "" {
		return nil, fmt.Errorf("OauthError(%s) %s", out.Error, out.ErrorDescription)
	}

	var ExpiresAt time.Time
	if out.ExpiresIn != 0 {
		ExpiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	}

	a := &Account{
		AccountType: at.ID(),
		Oauth:       &OauthInfo{ExpiresAt: ExpiresAt},
	}

	if out.AccessToken == "" {
		return nil, errors.New("server responded without access token")
	}

	a.Oauth.AccessToken = out.AccessToken
	a.Oauth.RefreshToken = out.RefreshToken
	a.Oauth.TokenType = out.TokenType

	// decode same resBody into a map for non-standard fields
	var extras map[string]interface{}
	json.Unmarshal(resBody, &extras)
	delete(extras, "access_token")
	delete(extras, "refresh_token")
	delete(extras, "token_type")
	delete(extras, "expires_in")

	if len(extras) > 0 {
		a.Extras = extras
	}

	return a, nil
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
