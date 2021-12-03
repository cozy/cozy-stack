package bi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// aggregatorID is the ID of the io.cozy.account CouchDB document where the
// user BI token is persisted.
const aggregatorID = "bi-aggregator"

// FireWebhook is used when the stack receives a call for a BI webhook, with an
// bearer token and a JSON payload. It will try to find a matching
// io.cozy.account and a io.cozy.trigger, and launch a job for them if
// needed.
func FireWebhook(inst *instance.Instance, token string, payload map[string]interface{}) error {
	var accounts []*account.Account
	if err := couchdb.GetAllDocs(inst, consts.Accounts, nil, &accounts); err != nil {
		return err
	}
	if err := checkToken(accounts, token); err != nil {
		return err
	}

	connID, err := extractPayloadConnID(payload)
	if err != nil {
		return err
	}
	if connID == 0 {
		return errors.New("no connection.id")
	}

	account, err := findAccount(accounts, connID)
	if err != nil {
		return err
	}
	trigger, err := findTrigger(inst, account)
	if err != nil {
		return err
	}

	if mustExecuteKonnector(inst, trigger, payload) {
		return fireTrigger(inst, trigger, account, payload)
	}
	return copyLastUpdate(inst, account, payload)
}

func checkToken(accounts []*account.Account, token string) error {
	for _, acc := range accounts {
		if acc.ID() == aggregatorID {
			if subtle.ConstantTimeCompare([]byte(token), []byte(acc.Token)) == 1 {
				return nil
			}
			return errors.New("token is invalid")
		}
	}
	return errors.New("no bi-aggregator account found")
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

func mustExecuteKonnector(
	inst *instance.Instance,
	trigger job.Trigger,
	payload map[string]interface{},
) bool {
	return payloadHasAccounts(payload) || lastExecNotSuccessful(inst, trigger)
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

func fireTrigger(
	inst *instance.Instance,
	trigger job.Trigger,
	account *account.Account,
	payload map[string]interface{},
) error {
	req := trigger.Infos().JobRequest()
	var msg map[string]interface{}
	if err := json.Unmarshal(req.Message, &msg); err == nil {
		msg["bi_webhook"] = true
		if updated, err := json.Marshal(msg); err == nil {
			req.Message = updated
		}
	}
	j, err := job.System().PushJob(inst, req)
	if err == nil {
		inst.Logger().WithNamespace("webhook").
			Debugf("Push job %s (account: %s - trigger: %s)", j.ID(), account.ID(), trigger.ID())
	}
	return err
}

func copyLastUpdate(
	inst *instance.Instance,
	account *account.Account,
	payload map[string]interface{},
) error {
	conn, ok := payload["connection"].(map[string]interface{})
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
	err := couchdb.UpdateDoc(inst, account)
	if err == nil {
		inst.Logger().WithNamespace("webhook").
			Debugf("Set lastUpdate to %s (account :%s)", lastUpdate, account.ID())
	}
	return err
}
