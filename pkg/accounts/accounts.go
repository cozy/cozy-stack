package accounts

import (
	"net/url"
	"time"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/globals"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
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
	AccessToken  string      `json:"access_token,omitempty"`
	TokenType    string      `json:"token_type,omitempty"`
	ExpiresAt    time.Time   `json:"expires_at,omitempty"`
	RefreshToken string      `json:"refresh_token,omitempty"`
	ClientID     string      `json:"client_id,omitempty"`
	ClientSecret string      `json:"client_secret,omitempty"`
	Query        *url.Values `json:"query,omitempty"`
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
		tmp := *ac.Oauth
		cloned.Oauth = &tmp
	}
	if ac.Basic != nil {
		tmp := *ac.Basic
		cloned.Basic = &tmp
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
			sched := globals.GetScheduler()
			broker := globals.GetBroker()

			trigs, err := sched.GetAll(domain)
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
				} else if msg.Account == doc.ID() {
					toDelete = true
				}
				if toDelete {
					if err := sched.Delete(domain, t.ID()); err != nil {
						logger.WithDomain(domain).Errorln("failed to delete orphan trigger", err)
					}
				}
			}

			// When an account is deleted, we need to push a new job in order to
			// delete possible data associated with this account. This is done via
			// this hook.
			//
			// This may require additionnal specifications to allow konnectors to
			// define more explicitly when and how they want to be called in order to
			// cleanup or update their associated content. For now we make this
			// process really specific to the deletion of an account, which is our
			// only detailed usecase.
			if old != nil {
				var konnector string
				switch v := old.(type) {
				case *Account:
					konnector = v.AccountType
				case couchdb.JSONDoc:
					konnector, _ = v.Get("account_type").(string)
				if konnector == "" {
					logger.WithDomain(domain).Info("No associated konnector for account, cannot create on_delete job")
					return nil
				}

				logger.WithDomain(domain).Info("Creating job for konnector on_delete", konnector)
				msg, err := jobs.NewMessage(struct {
					Account        string `json:"account"`
					AccountRev     string `json:"account_rev"`
					Konnector      string `json:"konnector"`
					AccountDeleted bool   `json:"account_deleted"`
				}{
					Account:        old.ID(),
					AccountRev:     old.Rev(),
					Konnector:      konnector,
					AccountDeleted: true,
				})
				if err != nil {
					return err
				}
				if _, err = broker.PushJob(&jobs.JobRequest{
					Domain:     domain,
					WorkerType: "konnector",
					Message:    msg,
				}); err != nil {
					return err
				}
			}

			return nil
		})
}
