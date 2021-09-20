package bi

import (
	"errors"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// FireWebhook is used when the stack receives a call for a BI webhook, with an
// bearer token and a JSON payload. It will try to find a matching
// io.cozy.account and a io.cozy.trigger, and launch a job for them if
// needed.
func FireWebhook(inst *instance.Instance, token string, payload map[string]interface{}) error {
	connID, err := extractPayloadConnID(payload)
	if err != nil {
		return err
	}
	if connID == "" {
		return errors.New("no connection.id")
	}

	account, err := findAccount(inst, connID)
	if err != nil {
		return err
	}
	// TODO check token

	trigger, err := findTrigger(inst, account)
	if err != nil {
		return err
	}

	// TODO look if we really need to fire the trigger, or just update the lastUpdate
	return fireTrigger(inst, trigger, account, payload)
}

func extractPayloadConnID(payload map[string]interface{}) (string, error) {
	conn, ok := payload["connection"].(map[string]interface{})
	if !ok {
		return "", errors.New("connection not found")
	}
	connID, ok := conn["id"].(string)
	if !ok {
		return "", errors.New("id_user not found")
	}
	return connID, nil
}

func findAccount(inst *instance.Instance, connID string) (*account.Account, error) {
	var accounts []*account.Account
	if err := couchdb.GetAllDocs(inst, consts.Accounts, nil, &accounts); err != nil {
		return nil, err
	}
	for _, account := range accounts {
		id := extractAccountConnID(account.Data)
		if id == connID {
			return account, nil
		}
	}
	return nil, errors.New("no account found with this id_user")
}

func extractAccountConnID(data map[string]interface{}) string {
	auth, ok := data["auth"].(map[string]interface{})
	if !ok {
		return ""
	}
	bi, ok := auth["bi"].(map[string]interface{})
	if !ok {
		return ""
	}
	connID, _ := bi["connId"].(string)
	return connID
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

func fireTrigger(
	inst *instance.Instance,
	trigger job.Trigger,
	account *account.Account,
	payload map[string]interface{},
) error {
	return nil
}
