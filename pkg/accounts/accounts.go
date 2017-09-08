package accounts

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/stack"
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
func (ac *Account) Clone() couchdb.Doc {
	cloned := *ac
	if ac.Oauth != nil {
		clonedOauth := *ac.Oauth
		cloned.Oauth = &clonedOauth
	}
	if ac.Basic != nil {
		clonedBasic := *ac.Basic
		cloned.Basic = &clonedBasic
	}
	cloned.Extras = make(map[string]interface{})
	for k, v := range ac.Extras {
		cloned.Extras[k] = v
	}
	return &cloned
}

// Valid implements permissions.Validable
func (ac *Account) Valid(field, expected string) bool {
	return field == "account_type" && expected == ac.AccountType
}

func init() {
	couchdb.AddHook(consts.Accounts, couchdb.EventDelete,
		func(domain string, doc couchdb.Doc, old couchdb.Doc) error {
			trigs, err := stack.GetScheduler().GetAll(domain)
			if err != nil {
				logger.WithDomain(domain).Error(
					"Failed to fetch triggers after account deletion: ", err)
				return err
			}
			for _, t := range trigs {
				toDelete := false

				if t.Infos().WorkerType != "konnector" {
					continue
				}

				var msg struct {
					Account string `json:"account"`
				}
				err := t.Infos().Message.Unmarshal(&msg)
				if err != nil {
					toDelete = true
				} else {
					if msg.Account == doc.ID() {
						toDelete = true
					}
				}
				if toDelete {
					if err := stack.GetScheduler().Delete(domain, t.ID()); err != nil {
						logger.WithDomain(domain).Errorln("failed to delete orphan trigger", err)
					}
				}
			}
			return nil
		})
}
