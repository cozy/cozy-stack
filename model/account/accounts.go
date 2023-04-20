package account

import (
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/hashicorp/go-multierror"
)

// Account holds configuration information for an account
type Account struct {
	DocID         string                 `json:"_id,omitempty"`
	DocRev        string                 `json:"_rev,omitempty"`
	Relationships map[string]interface{} `json:"relationships,omitempty"`
	Metadata      *metadata.CozyMetadata `json:"cozyMetadata,omitempty"`

	AccountType       string                   `json:"account_type"`
	Name              string                   `json:"name"`                        // Filled during creation request
	FolderPath        string                   `json:"folderPath,omitempty"`        // Legacy. Replaced by DefaultFolderPath
	DefaultFolderPath string                   `json:"defaultFolderPath,omitempty"` // Computed from other attributes if not provided
	Identifier        string                   `json:"identifier,omitempty"`        // Name of the Basic attribute used as identifier
	Basic             *BasicInfo               `json:"auth,omitempty"`
	Oauth             *OauthInfo               `json:"oauth,omitempty"`
	Extras            map[string]interface{}   `json:"oauth_callback_results,omitempty"`
	Data              map[string]interface{}   `json:"data,omitempty"`
	State             string                   `json:"state,omitempty"`
	TwoFACode         string                   `json:"twoFACode,omitempty"`
	MutedErrors       []map[string]interface{} `json:"mutedErrors,omitempty"`
	Token             string                   `json:"token,omitempty"`   // Used by bi-aggregator
	UserID            string                   `json:"user_id,omitempty"` // Used by bi-aggregator-user

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
	Login                string `json:"login,omitempty"`
	Email                string `json:"email,omitempty"`          // Legacy, used in some accounts instead of login
	Identifier           string `json:"identifier,omitempty"`     // Legacy, used in some accounts instead of login
	NewIdentifier        string `json:"new_identifier,omitempty"` // Legacy, used in some accounts instead of login
	AccountName          string `json:"accountName,omitempty"`    // Used when konnector has no credentials
	Password             string `json:"password,omitempty"`       // Legacy, used when no encryption
	EncryptedCredentials string `json:"credentials_encrypted,omitempty"`
	Token                string `json:"token,omitempty"` // Used by legacy OAuth konnectors
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
	cloned.Relationships = make(map[string]interface{})
	for k, v := range ac.Relationships {
		cloned.Relationships[k] = v
	}
	return &cloned
}

// Fetch implements permission.Fetcher
func (ac *Account) Fetch(field string) []string {
	switch field {
	case "account_type":
		return []string{ac.AccountType}
	default:
		return nil
	}
}

func (ac *Account) toJSONDoc() (*couchdb.JSONDoc, error) {
	buf, err := json.Marshal(ac)
	if err != nil {
		return nil, err
	}
	doc := couchdb.JSONDoc{}
	if err := json.Unmarshal(buf, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// GetTriggers returns the list of triggers associated with the given
// accountID. In particular, the stack will need to remove them when the
// account is deleted.
func GetTriggers(jobsSystem job.JobSystem, db prefixer.Prefixer, accountID string) ([]job.Trigger, error) {
	triggers, err := jobsSystem.GetAllTriggers(db)
	if err != nil {
		return nil, err
	}

	var toDelete []job.Trigger
	for _, t := range triggers {
		if !t.Infos().IsKonnectorTrigger() {
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

// CleanEntry is a struct with an account and its associated trigger.
type CleanEntry struct {
	Account          *Account
	Triggers         []job.Trigger
	ManifestOnDelete bool // the manifest of the konnector has a field "on_delete_account"
	Slug             string
}

// CleanAndWait deletes the accounts. If an account is for a konnector with
// "on_delete_account", a job is pushed and it waits for the job success to
// continue. Finally, the associated trigger can be deleted.
func CleanAndWait(inst *instance.Instance, toClean []CleanEntry) error {
	ch := make(chan error)
	for i := range toClean {
		go func(entry CleanEntry) {
			ch <- cleanAndWaitSingle(inst, entry)
		}(toClean[i])
	}
	var errm error
	for range toClean {
		if err := <-ch; err != nil {
			inst.Logger().
				WithNamespace("accounts").
				WithField("critical", "true").
				Errorf("Error on delete_for_account: %v", err)
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}

func cleanAndWaitSingle(inst *instance.Instance, entry CleanEntry) error {
	jobsSystem := job.System()
	acc := entry.Account
	createSoftDeletedAccount(inst, acc)
	acc.ManualCleaning = true
	oldRev := acc.Rev() // The deletion job needs the rev just before the deletion
	if err := couchdb.DeleteDoc(inst, acc); err != nil {
		return err
	}
	// If the konnector has a field "on_delete_account", we need to execute a job
	// for this konnector to clean the account on the remote API, and
	// wait for this job to be done before uninstalling the konnector.
	if entry.ManifestOnDelete {
		j, err := PushAccountDeletedJob(jobsSystem, inst, acc.ID(), oldRev, entry.Slug)
		if err != nil {
			return err
		}
		err = j.WaitUntilDone(inst)
		if err != nil {
			return err
		}
	}
	for _, t := range entry.Triggers {
		err := jobsSystem.DeleteTrigger(inst, t.ID())
		if err != nil {
			inst.Logger().WithNamespace("accounts").
				Errorf("Cannot delete the trigger: %v", err)
		}
	}
	return nil
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

// ComputeName tries to use the value of the `auth` attribute pointed by the
// value of the `identifier` attribute as the Account name and set it in the
// JSON document.
//
// See https://github.com/cozy/cozy-doctypes/blob/master/docs/io.cozy.accounts.md#about-the-name-of-the-account
func ComputeName(doc couchdb.JSONDoc) {
	auth, ok := doc.M["auth"].(map[string]interface{})
	if !ok || auth == nil {
		return
	}

	identifier, ok := doc.M["identifier"].(string)
	if !ok || identifier == "" {
		if login, ok := auth["login"].(string); ok {
			doc.M["name"] = login
		}
		return
	}

	if name, ok := auth[identifier].(string); ok {
		doc.M["name"] = name
	}
}

func init() {
	couchdb.AddHook(consts.Accounts, couchdb.EventDelete,
		func(db prefixer.Prefixer, doc couchdb.Doc, old couchdb.Doc) error {
			logger.WithDomain(db.DomainName()).
				WithField("account_id", old.ID()).
				Info("Executing account deletion hook")

			manualCleaning := false
			switch v := doc.(type) {
			case *Account:
				manualCleaning = v.ManualCleaning
			case *couchdb.JSONDoc:
				manualCleaning, _ = v.M["manual_cleaning"].(bool)
			}
			if manualCleaning {
				return nil
			}

			jobsSystem := job.System()
			triggers, err := GetTriggers(jobsSystem, db, doc.ID())
			if err != nil {
				logger.WithDomain(db.DomainName()).Errorf(
					"Failed to fetch triggers after account deletion: %s", err)
				return err
			}
			for _, t := range triggers {
				if err := jobsSystem.DeleteTrigger(db, t.ID()); err != nil {
					logger.WithDomain(db.DomainName()).
						Errorf("failed to delete orphan trigger: %s", err)
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
			}
			if konnector == "" {
				logger.WithDomain(db.DomainName()).
					WithField("account_id", old.ID()).
					WithField("account_rev", old.Rev()).
					Info("No associated konnector for account: cannot create on_delete job")
				return nil
			}

			createSoftDeletedAccount(db, old)

			// Execute the OnDeleteAccount if the konnector has declared one
			man, err := app.GetKonnectorBySlug(db, konnector)
			if man != nil && man.OnDeleteAccount() != "" {
				_, err = PushAccountDeletedJob(jobsSystem, db, old.ID(), old.Rev(), konnector)
				return err
			}
			if !errors.Is(err, app.ErrNotFound) {
				return err
			}

			return nil
		})
}

func createSoftDeletedAccount(db prefixer.Prefixer, old couchdb.Doc) {
	var cloned *couchdb.JSONDoc
	switch old := old.(type) {
	case *Account:
		doc, err := old.toJSONDoc()
		if err != nil {
			logger.WithDomain(db.DomainName()).Errorf("Failed to soft-delete account: %s", err)
			return
		}
		cloned = doc
	case *couchdb.JSONDoc:
		cloned = old.Clone().(*couchdb.JSONDoc)
	default:
		return
	}

	cloned.Type = consts.SoftDeletedAccounts
	cloned.M["soft_deleted_rev"] = cloned.Rev()
	cloned.SetRev("")
	if err := createNamedDocWithDB(db, cloned); err != nil {
		logger.WithDomain(db.DomainName()).Errorf("Failed to soft-delete account: %s", err)
	}
	if err := couchdb.Compact(db, consts.Accounts); err != nil {
		logger.WithDomain(db.DomainName()).Infof("Failed to compact accounts: %s", err)
	}
}

func createNamedDocWithDB(db prefixer.Prefixer, doc couchdb.Doc) error {
	err := couchdb.CreateNamedDoc(db, doc)
	if couchdb.IsNoDatabaseError(err) {
		// XXX Ignore errors: we can have several requests in parallel to
		// create the database, and only one of them will succeed, but the
		// stack can still create documents in other goroutines / servers.
		_ = couchdb.CreateDB(db, doc.DocType())
		return couchdb.CreateNamedDoc(db, doc)
	}
	return err
}

var _ permission.Fetcher = &Account{}
