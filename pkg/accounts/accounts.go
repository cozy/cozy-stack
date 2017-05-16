package accounts

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// Account holds configuration information for an account
type Account struct {
	DocID       string                 `json:"_id,omitempty"`
	DocRev      string                 `json:"_rev,omitempty"`
	Name        string                 `json:"name"`
	AccountType string                 `json:"account_type"`
	Basic       *BasicInfo             `json:"auth,omitempty"`
	Oauth       *OauthInfo             `json:"oauth,omitempty"`
	Extras      map[string]interface{} `json:"oauth_callback_results,omitempty"`
}

// OauthInfo holds configuration information for an oauth account
type OauthInfo struct {
	AccessToken  string    `json:"access_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
}

// BasicInfo holds configuration information for an user/pass account
type BasicInfo struct {
	Login    string `json:"login,omitempty"`
	Password string `json:"password,omitempty"`
}

// ID is used to implement the couchdb.Doc interface
func (ac *Account) ID() string { return ac.DocID }

// Rev is used to implement the couchdb.Doc interface
func (ac *Account) Rev() string { return ac.DocRev }

// SetID is used to implement the couchdb.Doc interface
func (ac *Account) SetID(id string) { ac.DocID = id }

// SetRev is used to implement the couchdb.Doc interface
func (ac *Account) SetRev(rev string) { ac.DocRev = rev }

// DocType implements couchdb.Doc
func (ac *Account) DocType() string { return consts.Accounts }

// Clone implements couchdb.Doc
func (ac *Account) Clone() couchdb.Doc { cloned := *ac; return &cloned }

// Valid implements permissions.Validable
func (ac *Account) Valid(field, expected string) bool {
	return field == "account_type" && expected == ac.AccountType
}
