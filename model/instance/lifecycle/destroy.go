package lifecycle

import (
	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	job "github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/hooks"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/mail"
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
	if err := deleteAccounts(inst); err != nil {
		sendAlert(inst, err)
		return err
	}

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

func deleteAccounts(inst *instance.Instance) error {
	var accounts []*account.Account
	if err := couchdb.GetAllDocs(inst, consts.Accounts, nil, &accounts); err != nil {
		if couchdb.IsNoDatabaseError(err) {
			return nil
		}
		return err
	}

	var toClean []account.CleanEntry
	for _, acc := range accounts {
		// Accounts that are not tied to a konnector must not be deleted, and
		// the aggregator accounts in particular.
		slug := acc.AccountType
		if slug == "" {
			continue
		}
		man, err := app.GetKonnectorBySlug(inst, slug)
		if err != nil {
			return err
		}
		entry := account.CleanEntry{
			Account:          acc,
			Trigger:          nil, // We don't care, the triggers will all be deleted a bit later
			ManifestOnDelete: man.OnDeleteAccount != "",
			Slug:             slug,
		}
		toClean = append(toClean, entry)
	}
	if len(toClean) == 0 {
		return nil
	}

	return account.CleanAndWait(inst, toClean)
}

func sendAlert(inst *instance.Instance, e error) {
	alert := config.GetConfig().AlertAddr
	if alert == "" {
		return
	}
	addr := &mail.Address{
		Name:  "Support",
		Email: alert,
	}
	values := map[string]interface{}{
		"Domain": inst.Domain,
		"Error":  e.Error(),
	}
	msg, err := job.NewMessage(mail.Options{
		Mode:           mail.ModeFrom,
		To:             []*mail.Address{addr},
		TemplateName:   "alert_account",
		TemplateValues: values,
		Layout:         mail.CozyCloudLayout,
	})
	if err == nil {
		_, _ = job.System().PushJob(inst, &job.JobRequest{
			WorkerType: "sendmail",
			Message:    msg,
		})
	}
}
