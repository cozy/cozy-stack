package account

import (
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Account holds configuration information for an account
type Account struct {
	DocID       string                 `json:"_id,omitempty"`
	DocRev      string                 `json:"_rev,omitempty"`
	Name        string                 `json:"name"`
	AccountType string                 `json:"account_type"`
	FolderPath  string                 `json:"folderPath,omitempty"`
	Basic       *BasicInfo             `json:"auth,omitempty"`
	Oauth       *OauthInfo             `json:"oauth,omitempty"`
	Extras      map[string]interface{} `json:"oauth_callback_results,omitempty"`

	// When an account is deleted, the stack cleans the triggers and calls its
	// konnector to clean the account remotely (when available). It is done via
	// a hook on deletion, but when the konnector is removed, this cleaning is
	// done manually before uninstalling the konnector, and this flag is used
	// to not try doing the cleaning in the hook as it is already too late (the
	// konnector is no longer available).
	ManualCleaning bool `json:"manual_cleaning,omitempty"`
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
	Login      string `json:"login,omitempty"`
	Password   string `json:"password,omitempty"`
	FolderPath string `json:"folderPath,omitempty"`
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

// Match implements permissions.Matcher
func (ac *Account) Match(field, expected string) bool {
	return field == "account_type" && expected == ac.AccountType
}

// GetTriggers returns the list of triggers associated with the given
// accountID. In particular, the the stack will need to remove them when the
// account is deleted.
func GetTriggers(jobsSystem job.JobSystem, db prefixer.Prefixer, accountID string) ([]job.Trigger, error) {
	triggers, err := jobsSystem.GetAllTriggers(db)
	if err != nil {
		return nil, err
	}

	var toDelete []job.Trigger
	for _, t := range triggers {
		if t.Infos().WorkerType != "konnector" {
			continue
		}

		var msg struct {
			Account string `json:"account"`
		}
		err := t.Infos().Message.Unmarshal(&msg)
		if err == nil && msg.Account == accountID {
			toDelete = append(toDelete, t)
		}
	}
	return toDelete, nil
}

// PushAccountDeletedJob adds a job for the given account and konnector with
// the AccountDeleted flag, to allow the konnector to clear the account
// remotely.
func PushAccountDeletedJob(jobsSystem job.JobSystem, db prefixer.Prefixer, accountID, accountRev, konnector string) (*job.Job, error) {
	logger.WithDomain(db.DomainName()).
		WithField("account_id", accountID).
		WithField("account_rev", accountRev).
		WithField("konnector", konnector).
		Info("Pushing job for konnector on_delete")

	msg, err := job.NewMessage(struct {
		Account        string `json:"account"`
		AccountRev     string `json:"account_rev"`
		Konnector      string `json:"konnector"`
		AccountDeleted bool   `json:"account_deleted"`
	}{
		Account:        accountID,
		AccountRev:     accountRev,
		Konnector:      konnector,
		AccountDeleted: true,
	})
	if err != nil {
		return nil, err
	}
	return jobsSystem.PushJob(db, &job.JobRequest{
		WorkerType: "konnector",
		Message:    msg,
		Manual:     true, // Select high-priority for these jobs
	})
}

func init() {
	couchdb.AddHook(consts.Accounts, couchdb.EventDelete,
		func(db prefixer.Prefixer, doc couchdb.Doc, old couchdb.Doc) error {
			manualCleaning := false
			switch v := doc.(type) {
			case *Account:
				manualCleaning = v.ManualCleaning
			case *couchdb.JSONDoc:
				manualCleaning, _ = v.M["manual_cleaning"].(bool)
			case couchdb.JSONDoc:
				manualCleaning, _ = v.M["manual_cleaning"].(bool)
			}
			if manualCleaning {
				return nil
			}

			jobsSystem := job.System()
			triggers, err := GetTriggers(jobsSystem, db, doc.ID())
			if err != nil {
				logger.WithDomain(db.DomainName()).Error(
					"Failed to fetch triggers after account deletion: ", err)
				return err
			}
			for _, t := range triggers {
				if err := jobsSystem.DeleteTrigger(db, t.ID()); err != nil {
					logger.WithDomain(db.DomainName()).Errorln("failed to delete orphan trigger", err)
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
			if old == nil {
				return nil
			}
			var konnector string
			switch v := old.(type) {
			case *Account:
				konnector = v.AccountType
			case *couchdb.JSONDoc:
				konnector, _ = v.M["account_type"].(string)
			case couchdb.JSONDoc:
				konnector, _ = v.M["account_type"].(string)
			}
			if konnector == "" {
				logger.WithDomain(db.DomainName()).
					WithField("account_id", old.ID()).
					WithField("account_rev", old.Rev()).
					Info("No associated konnector for account: cannot create on_delete job")
				return nil
			}
			_, err = PushAccountDeletedJob(jobsSystem, db, old.ID(), old.Rev(), konnector)
			return err
		})
}
