package lifecycle

import (
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	job "github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/hooks"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

// Destroy is used to remove the instance. All the data linked to this
// instance will be permanently deleted.
func Destroy(domain string) error {
	domain, err := validateDomain(domain)
	if err != nil {
		return err
	}
	return hooks.Execute("remove-instance", []string{domain}, func() error {
		return DestroyWithoutHooks(domain)
	})
}

// DestroyWithoutHooks is used to remove the instance. The difference with
// Destroy is that scripts hooks are not executed for this function.
func DestroyWithoutHooks(domain string) error {
	var err error
	domain, err = validateDomain(domain)
	if err != nil {
		return err
	}
	inst, err := instance.GetFromCouch(domain)
	if err != nil {
		return err
	}

	// Deleting accounts manually to invoke the "account deletion hook" which may
	// launch a worker in order to clean the account.
	deleteAccounts(inst)

	// Reload the instance, it can have been updated in CouchDB if the instance
	// had at least one account and was not up-to-date for its indexes/views.
	inst, err = instance.GetFromCouch(domain)
	if err != nil {
		return err
	}

	sched := job.System()
	triggers, err := sched.GetAllTriggers(inst)
	if err == nil {
		for _, t := range triggers {
			if err = sched.DeleteTrigger(inst, t.Infos().TID); err != nil {
				logger.WithDomain(domain).Error(
					"Failed to remove trigger: ", err)
			}
		}
	}

	if err = couchdb.DeleteAllDBs(inst); err != nil {
		inst.Logger().Errorf("Could not delete all CouchDB databases: %s", err.Error())
		return err
	}

	if err = inst.VFS().Delete(); err != nil {
		inst.Logger().Errorf("Could not delete VFS: %s", err.Error())
		return err
	}

	return couchdb.DeleteDoc(couchdb.GlobalDB, inst)
}

func deleteAccounts(inst *instance.Instance) {
	var accounts []*couchdb.JSONDoc
	if err := couchdb.GetAllDocs(inst, consts.Accounts, nil, &accounts); err != nil || len(accounts) == 0 {
		return
	}

	ds := realtime.GetHub().Subscriber(inst)
	defer ds.Close()

	accountsCount := 0
	for _, account := range accounts {
		account.Type = consts.Accounts
		if err := couchdb.DeleteDoc(inst, account); err == nil {
			accountsCount++
		}
	}
	if accountsCount == 0 {
		return
	}

	if err := ds.Subscribe(consts.Jobs); err != nil {
		return
	}

	timeout := time.After(1 * time.Minute)
	for {
		select {
		case e := <-ds.Channel:
			j, ok := e.Doc.(*couchdb.JSONDoc)
			if ok {
				deleted, _ := j.M["account_deleted"].(bool)
				stateStr, _ := j.M["state"].(string)
				state := job.State(stateStr)
				if deleted && (state == job.Done || state == job.Errored) {
					accountsCount--
					if accountsCount == 0 {
						return
					}
				}
			}
		case <-timeout:
			return
		}
	}
}
