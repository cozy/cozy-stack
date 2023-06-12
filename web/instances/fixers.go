package instances

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/stack"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/labstack/echo/v4"
)

type mismatchStruct struct {
	SizeIndex int64 `json:"size_index"`
	SizeFile  int64 `json:"size_file"`
}

// resEntry contains an out entry of a 64k content mismatch
type resEntry struct {
	FilePath  string `json:"filepath"`
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type resStruct struct {
	DryRun  bool       `json:"dry_run"`
	Updated []resEntry `json:"updated"`
	Removed []resEntry `json:"removed"`
	Domain  string     `json:"domain"`
}

// contentMismatchFixer fixes the 64k bug
func contentMismatchFixer(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := instance.Get(domain)
	if err != nil {
		return fmt.Errorf("Cannot find instance %s", domain)
	}

	body := struct {
		DryRun bool `json:"dry_run"`
	}{
		DryRun: true,
	}

	// Try to get the dry_run param from the body. If there is no body, ignore
	// it
	_ = json.NewDecoder(c.Request().Body).Decode(&body)

	// Get the FSCK data from the instance
	buf, err := getFSCK(inst)
	if err != nil {
		return err
	}

	var content map[string]interface{}
	res := &resStruct{
		Domain:  domain,
		DryRun:  body.DryRun,
		Removed: []resEntry{},
		Updated: []resEntry{},
	}

	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		err = json.NewDecoder(bytes.NewReader(scanner.Bytes())).Decode(&content)
		if err != nil {
			return err
		}

		// Filtering the 64kb mismatch issue
		if content["type"] != "content_mismatch" {
			continue
		}

		// Prepare the struct & ensure the data should be fixed
		contentMismatch, err := prepareMismatchStruct(content)
		if err != nil {
			return err
		}
		if !is64ContentMismatch(contentMismatch) {
			continue
		}

		// Finally, fixing the file
		err = fixFile(content, contentMismatch, inst, res, body.DryRun)
		if err != nil {
			return err
		}
	}

	return c.JSON(http.StatusOK, res)
}

func getFSCK(inst *instance.Instance) (io.Reader, error) {
	buf := new(bytes.Buffer)
	encoder := json.NewEncoder(buf)

	logCh := make(chan *vfs.FsckLog)
	go func() {
		fs := inst.VFS()
		_ = fs.Fsck(func(log *vfs.FsckLog) { logCh <- log }, false)
		close(logCh)
	}()

	for log := range logCh {
		if !log.IsFile && !log.IsVersion && log.DirDoc != nil {
			log.DirDoc.DirsChildren = nil
			log.DirDoc.FilesChildren = nil
		}
		if errenc := encoder.Encode(log); errenc != nil {
			return nil, errenc
		}
	}

	return buf, nil
}

func prepareMismatchStruct(content map[string]interface{}) (*mismatchStruct, error) {
	contentMismatch := &mismatchStruct{}
	marshaled, _ := json.Marshal(content["content_mismatch"])
	if err := json.Unmarshal(marshaled, &contentMismatch); err != nil {
		return nil, err
	}

	return contentMismatch, nil
}

// is64ContentMismatch ensures we are treating a 64k content mismatch
func is64ContentMismatch(contentMismatch *mismatchStruct) bool {
	// SizeFile should be a multiple of 64k shorter than SizeIndex
	size := int64(64 * 1024)

	isSmallFile := contentMismatch.SizeIndex <= size && contentMismatch.SizeFile == 0
	isMultiple64 := (contentMismatch.SizeIndex-contentMismatch.SizeFile)%size == 0

	return isMultiple64 || isSmallFile
}

// fixFile fixes a content-mismatch file
// Trashed:
// - Removes it if the file
// Not Trashed:
// - Appending a corrupted suffix to the file
// - Force the file index size to the real file size
func fixFile(content map[string]interface{}, contentMismatch *mismatchStruct, inst *instance.Instance, res *resStruct, dryRun bool) error {
	corruptedSuffix := "-corrupted"

	// Removes/update
	fileDoc := content["file_doc"].(map[string]interface{})

	doc := &vfs.FileDoc{}
	err := couchdb.GetDoc(inst, consts.Files, fileDoc["_id"].(string), doc)
	if err != nil {
		return err
	}
	instanceVFS := inst.VFS()

	// File is trashed
	if fileDoc["restore_path"] != nil {
		// This is a trashed file, just delete it
		res.Removed = append(res.Removed, resEntry{
			ID:        fileDoc["_id"].(string),
			FilePath:  fileDoc["path"].(string),
			CreatedAt: doc.CreatedAt.String(),
			UpdatedAt: doc.UpdatedAt.String(),
		})

		if !dryRun {
			return instanceVFS.DestroyFile(doc)
		}
		return nil
	}

	// File is not trashed, updating it
	newFileDoc := doc.Clone().(*vfs.FileDoc)

	newFileDoc.DocName = doc.DocName + corruptedSuffix
	newFileDoc.ByteSize = contentMismatch.SizeFile

	res.Updated = append(res.Updated, resEntry{
		ID:        fileDoc["_id"].(string),
		FilePath:  fileDoc["path"].(string),
		CreatedAt: doc.CreatedAt.String(),
		UpdatedAt: doc.UpdatedAt.String(),
	})
	if !dryRun {
		// Let the UpdateFileDoc handles the file doc update. For swift
		// layout V1, the file should also be renamed
		return instanceVFS.UpdateFileDoc(doc, newFileDoc)
	}

	return nil
}

func passwordDefinedFixer(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := instance.Get(domain)
	if err != nil {
		return err
	}

	if inst.PasswordDefined != nil {
		return c.NoContent(http.StatusNoContent)
	}

	defined := false
	if inst.OnboardingFinished {
		defined = true
		if inst.HasForcedOIDC() || inst.MagicLink {
			bitwarden, err := settings.Get(inst)
			if err == nil && !bitwarden.ExtensionInstalled {
				defined = false
			}
		}
	}
	inst.PasswordDefined = &defined
	if err := inst.Update(); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func orphanAccountFixer(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := instance.Get(domain)
	if err != nil {
		return err
	}

	var accounts []*account.Account
	err = couchdb.GetAllDocs(inst, consts.Accounts, nil, &accounts)
	if err != nil || len(accounts) == 0 {
		return err
	}

	var konnectors []*couchdb.JSONDoc
	err = couchdb.GetAllDocs(inst, consts.Konnectors, nil, &konnectors)
	if err != nil {
		return err
	}

	var slugsToDelete []string
	for _, acc := range accounts {
		if acc.AccountType == "" {
			continue // Skip the design docs
		}
		found := false
		for _, konn := range konnectors {
			if konn.M["slug"] == acc.AccountType {
				found = true
				break
			}
		}
		if !found {
			for _, slug := range slugsToDelete {
				if slug == acc.AccountType {
					found = true
					break
				}
			}
			if !found {
				slugsToDelete = append(slugsToDelete, acc.AccountType)
			}
		}
	}
	if len(slugsToDelete) == 0 {
		return nil
	}

	if _, err = stack.Start(); err != nil {
		return err
	}
	jobsSystem := job.System()
	log := inst.Logger().WithNamespace("fixer")
	copier := app.Copier(consts.KonnectorType, inst)

	for _, slug := range slugsToDelete {
		opts := &app.InstallerOptions{
			Operation:  app.Install,
			Type:       consts.KonnectorType,
			SourceURL:  "registry://" + slug + "/stable",
			Slug:       slug,
			Registries: inst.Registries(),
		}
		ins, err := app.NewInstaller(inst, copier, opts)
		if err != nil {
			return err
		}
		if _, err = ins.RunSync(); err != nil {
			return err
		}

		for _, acc := range accounts {
			if acc.AccountType != slug {
				continue
			}
			acc.ManualCleaning = true
			oldRev := acc.Rev() // The deletion job needs the rev just before the deletion
			if err := couchdb.DeleteDoc(inst, acc); err != nil {
				log.Errorf("Cannot delete account: %v", err)
			}
			j, err := account.PushAccountDeletedJob(jobsSystem, inst, acc.ID(), oldRev, slug)
			if err != nil {
				log.Errorf("Cannot push a job for account deletion: %v", err)
			}
			if err = j.WaitUntilDone(inst); err != nil {
				log.Error(err.Error())
			}
		}
		opts.Operation = app.Delete
		ins, err = app.NewInstaller(inst, copier, opts)
		if err != nil {
			return err
		}
		if _, err = ins.RunSync(); err != nil {
			return err
		}
	}

	return c.NoContent(http.StatusNoContent)
}

type serviceMessage struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	// and some other fields not needed here
}

func serviceTriggersFixer(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := instance.Get(domain)
	if err != nil {
		return err
	}

	jobsSystem := job.System()
	triggers, err := jobsSystem.GetAllTriggers(inst)
	if err != nil {
		return err
	}
	byApps := make(map[string][]job.Trigger)
	for _, trigger := range triggers {
		trigger := trigger
		infos := trigger.Infos()
		if infos.WorkerType != "service" {
			continue
		}
		if infos.Type == "@at" {
			continue
		}
		var msg serviceMessage
		if err := json.Unmarshal(infos.Message, &msg); err != nil {
			continue
		}
		list := byApps[msg.Slug]
		list = append(list, trigger)
		byApps[msg.Slug] = list
	}

	var toDelete []job.Trigger
	recreated := 0
	updated := 0

	for slug, triggers := range byApps {
		manifest, err := app.GetWebappBySlug(inst, slug)
		if errors.Is(err, app.ErrNotFound) {
			// The app has been uninstalled, but some duplicate triggers has
			// been left
			toDelete = append(toDelete, triggers...)
			continue
		} else if err != nil {
			return err
		}

		// Fill the trigger ids for the services when they are missing.
		updateApp := false
		for name, service := range manifest.Services() {
			if service.TriggerOptions == "" {
				continue
			}
			var recreate bool
			if service.TriggerID == "" {
				for _, trigger := range triggers {
					infos := trigger.Infos()
					if infos.Debounce != service.Debounce {
						continue
					}
					opts := infos.Type + " " + infos.Arguments
					if opts != service.TriggerOptions {
						continue
					}
					var msg serviceMessage
					if err := json.Unmarshal(infos.Message, &msg); err != nil {
						continue
					}
					if msg.Name != name {
						continue
					}
					service.TriggerID = infos.TID
					updateApp = true
					break
				}
				recreate = service.TriggerID == ""
			} else {
				trigger, err := jobsSystem.GetTrigger(inst, service.TriggerID)
				recreate = errors.Is(err, job.ErrNotFoundTrigger)
				if err == nil {
					var msg serviceMessage
					if err := json.Unmarshal(trigger.Infos().Message, &msg); err != nil {
						return err
					}
					if msg.Name == "" {
						fixTriggerName(inst, trigger, msg, name)
						updated++
					}
				}
			}

			if recreate {
				triggerID, err := app.CreateServiceTrigger(inst, slug, name, service)
				if err != nil {
					return err
				}
				service.TriggerID = triggerID
				updateApp = true
				recreated++
			}
		}

		if updateApp {
			if err := couchdb.UpdateDoc(inst, manifest); err != nil {
				return err
			}
		}

		// Add to the list of triggers that should be deleted all the triggers
		// for this application that are not tied to a service.
		for _, trigger := range triggers {
			trigger := trigger
			tid := trigger.Infos().TID
			found := false
			for _, service := range manifest.Services() {
				if service.TriggerID == tid {
					found = true
				}
			}
			if !found {
				toDelete = append(toDelete, trigger)
			}
		}
	}

	for _, trigger := range toDelete {
		if err := jobsSystem.DeleteTrigger(inst, trigger.ID()); err != nil {
			return err
		}
	}

	return c.JSON(http.StatusOK, echo.Map{
		"Domain":                 domain,
		"RecreatedTriggersCount": recreated,
		"UpdatedTriggersCount":   updated,
		"DeletedTriggersCount":   len(toDelete),
	})
}

func fixTriggerName(inst *instance.Instance, trigger job.Trigger, msg serviceMessage, name string) error {
	msg.Name = name
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	infos := trigger.Infos().Clone().(*job.TriggerInfos)
	infos.Message = job.Message(raw)
	return couchdb.UpdateDoc(inst, infos)
}

func indexesFixer(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := instance.Get(domain)
	if err != nil {
		return err
	}

	if err := lifecycle.DefineViewsAndIndex(inst); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
