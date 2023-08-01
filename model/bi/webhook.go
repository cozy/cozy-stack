package bi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/assets/statik"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/metadata"
)

// aggregatorID is the ID of the io.cozy.account CouchDB document where the
// user BI token is persisted.
const aggregatorID = "bi-aggregator"

const aggregatorUserID = "bi-aggregator-user"

// EventBI is a type used for the events sent by BI in the webhooks
type EventBI string

const (
	// EventConnectionSynced is emitted after a connection has been synced
	EventConnectionSynced EventBI = "CONNECTION_SYNCED"
	// EventConnectionDeleted is emitted after a connection has been deleted
	EventConnectionDeleted EventBI = "CONNECTION_DELETED"
	// EventAccountEnabled is emitted after a bank account was enabled
	EventAccountEnabled EventBI = "ACCOUNT_ENABLED"
	// EventAccountDisabled is emitted after a bank account was disabled
	EventAccountDisabled EventBI = "ACCOUNT_DISABLED"
)

// ParseEventBI returns the event of the webhook, or an error if the event
// cannot be handled by the stack.
func ParseEventBI(evt string) (EventBI, error) {
	if evt == "" {
		return EventConnectionSynced, nil
	}

	biEvent := EventBI(strings.ToUpper(evt))
	switch biEvent {
	case EventConnectionSynced, EventConnectionDeleted,
		EventAccountEnabled, EventAccountDisabled:
		return biEvent, nil
	}
	return EventBI("INVALID"), errors.New("invalid event")
}

// WebhookCall contains the data relative to a call from BI for a webhook.
type WebhookCall struct {
	Instance *instance.Instance
	Token    string
	BIurl    string
	Event    EventBI
	Payload  map[string]interface{}

	accounts []*account.Account
}

// Fire is used when the stack receives a call for a BI webhook, with an bearer
// token and a JSON payload. It will try to find a matching io.cozy.account and
// a io.cozy.trigger, and launch a job for them if needed.
func (c *WebhookCall) Fire() error {
	var accounts []*account.Account
	if err := couchdb.GetAllDocs(c.Instance, consts.Accounts, nil, &accounts); err != nil {
		return err
	}
	c.accounts = accounts

	if err := c.checkToken(); err != nil {
		return err
	}

	switch c.Event {
	case EventConnectionSynced:
		return c.handleConnectionSynced()
	case EventConnectionDeleted:
		return c.handleConnectionDeleted()
	case EventAccountEnabled, EventAccountDisabled:
		return c.handleAccountEnabledOrDisabled()
	}
	return errors.New("event not handled")
}

func (c *WebhookCall) checkToken() error {
	for _, acc := range c.accounts {
		if acc.ID() == aggregatorID {
			if subtle.ConstantTimeCompare([]byte(c.Token), []byte(acc.Token)) == 1 {
				return nil
			}
			return errors.New("token is invalid")
		}
	}
	return errors.New("no bi-aggregator account found")
}

func (c *WebhookCall) handleConnectionSynced() error {
	connID, err := extractPayloadConnID(c.Payload)
	if err != nil {
		return err
	}
	if connID == 0 {
		return errors.New("no connection.id")
	}

	uuid, err := extractPayloadConnectionConnectorUUID(c.Payload)
	if err != nil {
		return err
	}
	slug, err := mapUUIDToSlug(uuid)
	if err != nil {
		c.Instance.Logger().WithNamespace("webhook").
			Warnf("no slug found for uuid %s: %s", uuid, err)
		return err
	}
	konn, err := app.GetKonnectorBySlug(c.Instance, slug)
	if err != nil {
		userID, _ := extractPayloadUserID(c.Payload)
		c.Instance.Logger().WithNamespace("webhook").
			Warnf("konnector not installed id_connection=%d id_user=%d uuid=%s slug=%s", connID, userID, uuid, slug)
		return nil
	}

	var trigger job.Trigger
	account, err := findAccount(c.accounts, connID)
	if err != nil {
		account, trigger, err = c.createAccountAndTrigger(konn, connID)
	} else {
		trigger, err = findTrigger(c.Instance, account)
		if err != nil {
			trigger, err = konn.CreateTrigger(c.Instance, account.ID(), "")
		}
	}
	if err != nil {
		return err
	}

	if c.mustExecuteKonnector(trigger) {
		return c.fireTrigger(trigger, account)
	}
	return c.copyLastUpdate(account, konn)
}

func mapUUIDToSlug(uuid string) (string, error) {
	f := statik.GetAsset("/mappings/bi-banks.json")
	if f == nil {
		return "", os.ErrNotExist
	}
	var mapping map[string]string
	if err := json.Unmarshal(f.GetData(), &mapping); err != nil {
		return "", err
	}
	slug, ok := mapping[uuid]
	if !ok || slug == "" {
		return "", errors.New("not found")
	}
	return slug, nil
}

func extractPayloadConnectionConnectorUUID(payload map[string]interface{}) (string, error) {
	conn, ok := payload["connection"].(map[string]interface{})
	if !ok {
		return "", errors.New("connection not found")
	}
	uuid, ok := conn["connector_uuid"].(string)
	if !ok {
		return "", errors.New("connection.connector not found")
	}
	return uuid, nil
}

func extractPayloadConnID(payload map[string]interface{}) (int, error) {
	conn, ok := payload["connection"].(map[string]interface{})
	if !ok {
		return 0, errors.New("connection not found")
	}
	connID, ok := conn["id"].(float64)
	if !ok {
		return 0, errors.New("connection.id not found")
	}
	return int(connID), nil
}

func extractPayloadUserID(payload map[string]interface{}) (int, error) {
	user, ok := payload["user"].(map[string]interface{})
	if !ok {
		return 0, errors.New("user not found")
	}
	id, ok := user["id"].(float64)
	if !ok {
		return 0, errors.New("user.id not found")
	}
	return int(id), nil
}

func (c *WebhookCall) handleConnectionDeleted() error {
	connID, err := extractPayloadID(c.Payload)
	if err != nil {
		return err
	}
	if connID == 0 {
		return errors.New("no connection.id")
	}

	msg := "no io.cozy.accounts deleted"
	if account, _ := findAccount(c.accounts, connID); account != nil {
		// The account has already been deleted on BI side, so we can skip the
		// on_delete execution for the konnector.
		account.ManualCleaning = true
		if err := couchdb.DeleteDoc(c.Instance, account); err != nil {
			c.Instance.Logger().WithNamespace("webhook").
				Warnf("failed to delete account: %s", err)
			return err
		}
		msg = fmt.Sprintf("account %s ", account.ID())

		trigger, _ := findTrigger(c.Instance, account)
		if trigger != nil {
			jobsSystem := job.System()
			if err := jobsSystem.DeleteTrigger(c.Instance, trigger.ID()); err != nil {
				c.Instance.Logger().WithNamespace("webhook").
					Errorf("failed to delete trigger: %s", err)
			}
			msg += fmt.Sprintf("and trigger %s ", trigger.ID())
		}
		msg += "deleted"
	}

	userID, _ := extractPayloadIDUser(c.Payload)
	c.Instance.Logger().WithNamespace("webhook").
		Infof("Connection deleted user_id=%d connection_id=%d %s", userID, connID, msg)

	// If the user has no longer any connections on BI, we must remove their
	// data from BI.
	api, err := newApiClient(c.BIurl)
	if err != nil {
		return err
	}
	nb, err := api.getNumberOfConnections(c.Instance, c.Token)
	if err != nil {
		return fmt.Errorf("getNumberOfConnections: %s", err)
	}
	if nb == 0 {
		if err := api.deleteUser(c.Token); err != nil {
			return fmt.Errorf("deleteUser: %s", err)
		}
		if err := c.resetAggregator(); err != nil {
			return fmt.Errorf("resetAggregator: %s", err)
		}
	}
	return nil
}

func extractPayloadID(payload map[string]interface{}) (int, error) {
	id, ok := payload["id"].(float64)
	if !ok {
		return 0, errors.New("id not found")
	}
	return int(id), nil
}

func extractPayloadIDUser(payload map[string]interface{}) (int, error) {
	id, ok := payload["id_user"].(float64)
	if !ok {
		return 0, errors.New("id not found")
	}
	return int(id), nil
}

func findAccount(accounts []*account.Account, connID int) (*account.Account, error) {
	for _, account := range accounts {
		id := extractAccountConnID(account.Data)
		if id == connID {
			return account, nil
		}
	}
	return nil, fmt.Errorf("no account found with the connection id %d", connID)
}

func extractAccountConnID(data map[string]interface{}) int {
	if data == nil {
		return 0
	}
	auth, ok := data["auth"].(map[string]interface{})
	if !ok {
		return 0
	}
	bi, ok := auth["bi"].(map[string]interface{})
	if !ok {
		return 0
	}
	connID, _ := bi["connId"].(float64)
	return int(connID)
}

func findTrigger(inst *instance.Instance, acc *account.Account) (job.Trigger, error) {
	jobsSystem := job.System()
	triggers, err := account.GetTriggers(jobsSystem, inst, acc.ID())
	if err != nil {
		return nil, err
	}
	if len(triggers) == 0 {
		return nil, errors.New("no trigger found for this account")
	}
	return triggers[0], nil
}

func (c *WebhookCall) resetAggregator() error {
	aggregator := findAccountByID(c.accounts, aggregatorID)
	if aggregator != nil {
		aggregator.Token = ""
		if err := couchdb.UpdateDoc(c.Instance, aggregator); err != nil {
			return err
		}
	}

	user := findAccountByID(c.accounts, aggregatorUserID)
	if user != nil {
		user.UserID = ""
		if err := couchdb.UpdateDoc(c.Instance, user); err != nil {
			return err
		}
	}

	return nil
}

func findAccountByID(accounts []*account.Account, id string) *account.Account {
	for _, account := range accounts {
		if id == account.DocID {
			return account
		}
	}
	return nil
}

func (c *WebhookCall) createAccountAndTrigger(konn *app.KonnManifest, connectionID int) (*account.Account, job.Trigger, error) {
	acc := couchdb.JSONDoc{Type: consts.Accounts}
	data := map[string]interface{}{
		"auth": map[string]interface{}{
			"bi": map[string]interface{}{
				"connId": connectionID,
			},
		},
	}
	rels := map[string]interface{}{
		"parent": map[string]interface{}{
			"data": map[string]interface{}{
				"_id":   aggregatorID,
				"_type": consts.Accounts,
			},
		},
	}
	acc.M = map[string]interface{}{
		"account_type":  konn.Slug(),
		"data":          data,
		"relationships": rels,
	}

	account.Encrypt(acc)
	account.ComputeName(acc)

	cm := metadata.New()
	cm.CreatedByApp = konn.Slug()
	cm.CreatedByAppVersion = konn.Version()
	cm.UpdatedByApps = []*metadata.UpdatedByAppEntry{
		{
			Slug:    konn.Slug(),
			Version: konn.Version(),
			Date:    cm.UpdatedAt,
		},
	}
	// This is not the expected type for a JSON doc but it should work since it
	// will be marshalled when saved.
	acc.M["cozyMetadata"] = cm

	if err := couchdb.CreateDoc(c.Instance, &acc); err != nil {
		return nil, nil, err
	}

	trigger, err := konn.CreateTrigger(c.Instance, acc.ID(), "")
	if err != nil {
		return nil, nil, err
	}

	created := &account.Account{
		DocID:         acc.ID(),
		DocRev:        acc.Rev(),
		AccountType:   konn.Slug(),
		Data:          data,
		Relationships: rels,
	}
	return created, trigger, nil
}

func (c *WebhookCall) handleAccountEnabledOrDisabled() error {
	connID, err := extractPayloadIDConnection(c.Payload)
	if err != nil {
		return err
	}
	if connID == 0 {
		return errors.New("no id_connection")
	}

	var trigger job.Trigger
	account, err := findAccount(c.accounts, connID)
	if err != nil {
		api, err := newApiClient(c.BIurl)
		if err != nil {
			return err
		}
		uuid, err := api.getConnectorUUID(connID, c.Token)
		if err != nil {
			return err
		}
		slug, err := mapUUIDToSlug(uuid)
		if err != nil {
			return err
		}
		konn, err := app.GetKonnectorBySlug(c.Instance, slug)
		if err != nil {
			return err
		}
		account, trigger, err = c.createAccountAndTrigger(konn, connID)
		if err != nil {
			return err
		}
	} else {
		trigger, err = findTrigger(c.Instance, account)
		if err != nil {
			return err
		}
	}

	return c.fireTrigger(trigger, account)
}

func extractPayloadIDConnection(payload map[string]interface{}) (int, error) {
	id, ok := payload["id_connection"].(float64)
	if !ok {
		return 0, errors.New("id_connection not found")
	}
	return int(id), nil
}

func (c *WebhookCall) mustExecuteKonnector(trigger job.Trigger) bool {
	return payloadHasAccounts(c.Payload) || lastExecNotSuccessful(c.Instance, trigger)
}

func payloadHasAccounts(payload map[string]interface{}) bool {
	conn, ok := payload["connection"].(map[string]interface{})
	if !ok {
		return false
	}
	accounts, ok := conn["accounts"].([]interface{})
	if !ok {
		return false
	}
	return len(accounts) > 0
}

func lastExecNotSuccessful(inst *instance.Instance, trigger job.Trigger) bool {
	lastJobs, err := job.GetJobs(inst, trigger.ID(), 1)
	if err != nil || len(lastJobs) == 0 {
		return true
	}
	return lastJobs[0].State != job.Done
}

func (c *WebhookCall) fireTrigger(trigger job.Trigger, account *account.Account) error {
	req := trigger.Infos().JobRequest()
	var msg map[string]interface{}
	if err := json.Unmarshal(req.Message, &msg); err == nil {
		msg["bi_webhook"] = true
		msg["event"] = string(c.Event)
		if updated, err := json.Marshal(msg); err == nil {
			req.Message = updated
		}
	}
	if raw, err := json.Marshal(c.Payload); err == nil {
		req.Payload = raw
	}
	j, err := job.System().PushJob(c.Instance, req)
	if err == nil {
		c.Instance.Logger().WithNamespace("webhook").
			Debugf("Push job %s (account: %s - trigger: %s)", j.ID(), account.ID(), trigger.ID())
	}
	return err
}

func (c *WebhookCall) copyLastUpdate(account *account.Account, konn *app.KonnManifest) error {
	conn, ok := c.Payload["connection"].(map[string]interface{})
	if !ok {
		return errors.New("no connection")
	}
	lastUpdate, ok := conn["last_update"].(string)
	if !ok {
		return errors.New("no connection.last_update")
	}
	if account.Data == nil {
		return fmt.Errorf("no data in account %s", account.ID())
	}
	auth, ok := account.Data["auth"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no data.auth in account %s", account.ID())
	}
	bi, ok := auth["bi"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no data.auth.bi in account %s", account.ID())
	}
	bi["lastUpdate"] = lastUpdate

	if account.Metadata == nil {
		cm := metadata.New()
		cm.CreatedByApp = konn.Slug()
		cm.CreatedByAppVersion = konn.Version()
		cm.UpdatedByApps = []*metadata.UpdatedByAppEntry{
			{
				Slug:    konn.Slug(),
				Version: konn.Version(),
				Date:    cm.UpdatedAt,
			},
		}
		account.Metadata = cm
	}

	err := couchdb.UpdateDoc(c.Instance, account)
	if err == nil {
		c.Instance.Logger().WithNamespace("webhook").
			Debugf("Set lastUpdate to %s (account :%s)", lastUpdate, account.ID())
	}
	return err
}
