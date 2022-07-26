package bi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// aggregatorID is the ID of the io.cozy.account CouchDB document where the
// user BI token is persisted.
const aggregatorID = "bi-aggregator"

// EventBI is a type used for the events sent by BI in the webhooks
type EventBI string

const (
	// EventConnectionSynced is emitted after a connection has been synced
	EventConnectionSynced EventBI = "CONNECTION_SYNCED"
	// EventConnectionDeleted is emitted after a connection has been deleted
	EventConnectionDeleted EventBI = "CONNECTION_DELETED"
)

// ParseEventBI returns the event of the webhook, or an error if the event
// cannot be handled by the stack.
func ParseEventBI(evt string) (EventBI, error) {
	if evt == "" {
		return EventConnectionSynced, nil
	}

	biEvent := EventBI(strings.ToUpper(evt))
	switch biEvent {
	case EventConnectionSynced, EventConnectionDeleted:
		return biEvent, nil
	}
	return EventBI("INVALID"), errors.New("invalid event")
}

type webhookCall struct {
	Instance *instance.Instance
	Accounts []*account.Account
	Token    string
	Event    EventBI
	Payload  map[string]interface{}
}

// FireWebhook is used when the stack receives a call for a BI webhook, with an
// bearer token and a JSON payload. It will try to find a matching
// io.cozy.account and a io.cozy.trigger, and launch a job for them if
// needed.
func FireWebhook(inst *instance.Instance, token string, evt EventBI, payload map[string]interface{}) error {
	var accounts []*account.Account
	if err := couchdb.GetAllDocs(inst, consts.Accounts, nil, &accounts); err != nil {
		return err
	}
	call := &webhookCall{
		Instance: inst,
		Accounts: accounts,
		Token:    token,
		Event:    evt,
		Payload:  payload,
	}

	if err := call.checkToken(); err != nil {
		return err
	}

	switch evt {
	case EventConnectionSynced:
		return call.handleConnectionSynced()
	case EventConnectionDeleted:
		return call.handleConnectionDeleted()
	}
	return errors.New("event not handled")
}

func (c *webhookCall) checkToken() error {
	for _, acc := range c.Accounts {
		if acc.ID() == aggregatorID {
			if subtle.ConstantTimeCompare([]byte(c.Token), []byte(acc.Token)) == 1 {
				return nil
			}
			return errors.New("token is invalid")
		}
	}
	return errors.New("no bi-aggregator account found")
}

func (c *webhookCall) handleConnectionSynced() error {
	connID, err := extractPayloadConnID(c.Payload)
	if err != nil {
		return err
	}
	if connID == 0 {
		return errors.New("no connection.id")
	}

	account, err := findAccount(c.Accounts, connID)
	if err != nil {
		return err
	}
	trigger, err := findTrigger(c.Instance, account)
	if err != nil {
		return err
	}

	if c.mustExecuteKonnector(trigger) {
		return c.fireTrigger(trigger, account)
	}
	return c.copyLastUpdate(account)
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

func (c *webhookCall) handleConnectionDeleted() error {
	connID, err := extractPayloadID(c.Payload)
	if err != nil {
		return err
	}
	if connID == 0 {
		return errors.New("no connection.id")
	}

	msg := "no io.cozy.accounts deleted"
	if account, _ := findAccount(c.Accounts, connID); account != nil {
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

func (c *webhookCall) mustExecuteKonnector(trigger job.Trigger) bool {
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

func (c *webhookCall) fireTrigger(trigger job.Trigger, account *account.Account) error {
	req := trigger.Infos().JobRequest()
	var msg map[string]interface{}
	if err := json.Unmarshal(req.Message, &msg); err == nil {
		msg["bi_webhook"] = true
		msg["event"] = string(c.Event)
		if updated, err := json.Marshal(msg); err == nil {
			req.Message = updated
		}
	}
	j, err := job.System().PushJob(c.Instance, req)
	if err == nil {
		c.Instance.Logger().WithNamespace("webhook").
			Debugf("Push job %s (account: %s - trigger: %s)", j.ID(), account.ID(), trigger.ID())
	}
	return err
}

func (c *webhookCall) copyLastUpdate(account *account.Account) error {
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
	err := couchdb.UpdateDoc(c.Instance, account)
	if err == nil {
		c.Instance.Logger().WithNamespace("webhook").
			Debugf("Set lastUpdate to %s (account :%s)", lastUpdate, account.ID())
	}
	return err
}
