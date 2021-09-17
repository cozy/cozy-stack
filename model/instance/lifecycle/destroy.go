package lifecycle

import (
	"time"

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
		return destroyWithoutHooks(domain)
	})
}

// destroyWithoutHooks is used to remove the instance. The difference with
// Destroy is that scripts hooks are not executed for this function.
func destroyWithoutHooks(domain string) error {
	inst, err := instance.GetFromCouch(domain)
	if err != nil {
		return err
	}

	// Check that we don't try to run twice the deletion of accounts
	if inst.Deleting {
		return instance.ErrDeletionAlreadyRequested
	}
	inst.Deleting = true
	if err := inst.Update(); err != nil {
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
	inst.Deleting = false
	_ = inst.Update()

	removeTriggers(inst)

	if err = couchdb.DeleteAllDBs(inst); err != nil {
		inst.Logger().Errorf("Could not delete all CouchDB databases: %s", err.Error())
		return err
	}

	if err = inst.VFS().Delete(); err != nil {
		inst.Logger().Errorf("Could not delete VFS: %s", err.Error())
		return err
	}

	err = inst.Delete()
	if couchdb.IsConflictError(err) {
		// We may need to try again as CouchDB can return an old version of
		// this document when we have concurrent updates for indexes/views
		// version and deleting flag.
		time.Sleep(3 * time.Second)
		inst, errg := instance.GetFromCouch(domain)
		if couchdb.IsNotFoundError(errg) {
			err = nil
		} else if inst != nil {
			err = inst.Delete()
		}
	}
	return err
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
		if err == app.ErrNotFound {
			copier := app.Copier(consts.KonnectorType, inst)
			installer, erri := app.NewInstaller(inst, copier,
				&app.InstallerOptions{
					Operation:  app.Install,
					Type:       consts.KonnectorType,
					SourceURL:  "registry://" + slug + "/stable",
					Slug:       slug,
					Registries: inst.Registries(),
				},
			)
			if erri == nil {
				if appManifest, erri := installer.RunSync(); erri == nil {
					man = appManifest.(*app.KonnManifest)
					err = nil
				}
			}
		}
		if err != nil {
			return err
		}
		entry := account.CleanEntry{
			Account:          acc,
			Triggers:         nil, // We don't care, the triggers will all be deleted a bit later
			ManifestOnDelete: man.OnDeleteAccount() != "",
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
		Mode:           mail.ModeFromUser,
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

func removeTriggers(inst *instance.Instance) {
	sched := job.System()
	triggers, err := sched.GetAllTriggers(inst)
	if err == nil {
		for _, t := range triggers {
			if err = sched.DeleteTrigger(inst, t.Infos().TID); err != nil {
				logger.WithDomain(inst.Domain).Error(
					"Failed to remove trigger: ", err)
			}
		}
	}
}
