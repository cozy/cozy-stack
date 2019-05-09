package exec

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/registry"
	"github.com/sirupsen/logrus"
)

const (
	konnErrorLoginFailed      = "LOGIN_FAILED"
	konnErrorUserActionNeeded = "USER_ACTION_NEEDED"
)

type konnectorWorker struct {
	slug string
	msg  *KonnectorMessage
	man  *app.KonnManifest

	err     error
	lastErr error
}

const (
	konnectorMsgTypeDebug    = "debug"
	konnectorMsgTypeInfo     = "info"
	konnectorMsgTypeWarning  = "warning"
	konnectorMsgTypeError    = "error"
	konnectorMsgTypeCritical = "critical"
)

// KonnectorMessage is the message structure sent to the konnector worker.
type KonnectorMessage struct {
	Account        string `json:"account"`        // Account is the identifier of the account
	Konnector      string `json:"konnector"`      // Konnector is the slug of the konnector
	FolderToSave   string `json:"folder_to_save"` // FolderToSave is the identifier of the folder
	AccountDeleted bool   `json:"account_deleted,omitempty"`

	// Data contains the original value of the message, even fields that are not
	// part of our message definition.
	data json.RawMessage
}

// ToJSON returns a JSON reprensation of the KonnectorMessage
func (m *KonnectorMessage) ToJSON() string {
	return string(m.data)
}

func (m *KonnectorMessage) updateFolderToSave(dir string) {
	m.FolderToSave = dir
	var d map[string]interface{}
	_ = json.Unmarshal(m.data, &d)
	d["folder_to_save"] = dir
	m.data, _ = json.Marshal(d)
}

func jobHookErrorCheckerKonnector(err error) bool {
	// If there was no previous error, we are fine to go on
	if err == nil {
		return true
	}

	lastError := err.Error()
	if strings.HasPrefix(lastError, konnErrorLoginFailed) ||
		strings.HasPrefix(lastError, konnErrorUserActionNeeded) {
		return false
	}
	return true
}

// beforeHookKonnector skips jobs from trigger that are failing on certain
// errors.
func beforeHookKonnector(j *job.Job) (bool, error) {
	var msg KonnectorMessage

	if err := json.Unmarshal(j.Message, &msg); err == nil {
		inst, err := lifecycle.GetInstance(j.DomainName())
		if err != nil {
			return false, err
		}
		app, err := registry.GetApplication(msg.Konnector, inst.Registries())
		if err != nil {
			j.Logger().Warnf("konnector %q could not get application to fetch maintenance status", msg.Konnector)
		} else if app.MaintenanceActivated {
			if j.Manual && !app.MaintenanceOptions.FlagDisallowManualExec {
				return true, nil
			}
			j.Logger().Infof("konnector %q has not been triggered because of its maintenance status", msg.Konnector)
			return false, nil
		}
	}

	if j.Manual || j.TriggerID == "" {
		return true, nil
	}

	state, err := job.GetTriggerState(j, j.TriggerID)
	if err != nil {
		return false, err
	}
	if state.Status == job.Errored {
		if strings.HasPrefix(state.LastError, konnErrorLoginFailed) ||
			strings.HasPrefix(state.LastError, konnErrorUserActionNeeded) {
			return false, nil
		}
	}
	return true, nil
}

func (w *konnectorWorker) PrepareWorkDir(ctx *job.WorkerContext, i *instance.Instance) (string, error) {
	// Reset the errors from previous runs on retries
	w.err = nil
	w.lastErr = nil

	var err error
	var data json.RawMessage
	var msg KonnectorMessage
	if err = ctx.UnmarshalMessage(&data); err != nil {
		return "", err
	}
	if err = json.Unmarshal(data, &msg); err != nil {
		return "", err
	}
	msg.data = data

	slug := msg.Konnector
	w.slug = slug
	w.msg = &msg

	w.man, err = app.GetKonnectorBySlugAndUpdate(i, slug,
		i.AppsCopier(consts.KonnectorType), i.Registries())
	if err == app.ErrNotFound {
		return "", job.ErrBadTrigger{Err: err}
	} else if err != nil {
		return "", err
	}

	// Check that the associated account is present.
	var acc *account.Account
	if msg.Account != "" && !msg.AccountDeleted {
		acc = &account.Account{}
		err = couchdb.GetDoc(i, consts.Accounts, msg.Account, acc)
		if couchdb.IsNotFoundError(err) {
			return "", job.ErrBadTrigger{Err: err}
		}
	}

	// TODO: disallow konnectors on state Installed to be run when we define our
	// workflow to accept permissions changes on konnectors.
	man := w.man
	if s := man.State(); s != app.Ready && s != app.Installed {
		return "", errors.New("Konnector is not ready")
	}

	var workDir string
	osFS := afero.NewOsFs()
	workDir, err = afero.TempDir(osFS, "", "konnector-"+slug)
	if err != nil {
		return "", err
	}
	workFS := afero.NewBasePathFs(osFS, workDir)

	fileServer := i.KonnectorsFileServer()
	tarFile, err := fileServer.Open(slug, man.Version(), man.Checksum(), app.KonnectorArchiveName)
	if err == nil {
		err = extractTar(workFS, tarFile)
		if errc := tarFile.Close(); err == nil {
			err = errc
		}
	} else if os.IsNotExist(err) {
		err = copyFiles(workFS, fileServer, slug, man.Version(), man.Checksum())
	}
	if err != nil {
		return "", err
	}

	// Create the folder in which the konnector has the right to write.
	if err = w.ensureFolderToSave(ctx, i, acc); err != nil {
		return "", err
	}

	// Make sure the konnector can write to this folder
	if err = w.ensurePermissions(i); err != nil {
		return "", err
	}

	// If we get the AccountDeleted flag on, we check if the konnector manifest
	// has defined an "on_delete_account" field, containing the path of the file
	// to execute on account deletation. If no such field is present, the job is
	// aborted.
	if w.msg.AccountDeleted {
		// make sure we are not executing a path outside of the konnector's
		// directory
		fileExecPath := path.Join("/", path.Clean(w.man.OnDeleteAccount))
		fileExecPath = fileExecPath[1:]
		if fileExecPath == "" {
			return "", job.ErrAbort
		}
		workDir = path.Join(workDir, fileExecPath)
	}

	return workDir, nil
}

// ensureFolderToSave tries hard to give a folder to the konnector where it can
// write its files if it needs to do so.
func (w *konnectorWorker) ensureFolderToSave(ctx *job.WorkerContext, inst *instance.Instance, acc *account.Account) error {
	fs := inst.VFS()
	msg := w.msg

	var normalizedFolderPath string
	if acc != nil {
		admin := inst.Translate("Tree Administrative")
		r := strings.NewReplacer("&", "_", "/", "_", "\\", "_", "#", "_",
			",", "_", "+", "_", "(", "_", ")", "_", "$", "_", "@", "_", "~",
			"_", "%", "_", ".", "_", "'", "_", "\"", "_", ":", "_", "*", "_",
			"?", "_", "<", "_", ">", "_", "{", "_", "}", "_")
		accountName := r.Replace(acc.Name)
		normalizedFolderPath = fmt.Sprintf("/%s/%s/%s", admin, strings.Title(w.slug), accountName)

		// This is code to handle legacy: if the konnector does not actually require
		// a directory (for instance because it does not upload files), but a folder
		// has been created in the past by the stack which is still empty, then we
		// delete it.
		if msg.FolderToSave == "" && acc.FolderPath == "" && (acc.Basic == nil || acc.Basic.FolderPath == "") {
			if dir, errp := fs.DirByPath(normalizedFolderPath); errp == nil {
				if acc.Name == "" {
					innerDirPath := path.Join(normalizedFolderPath, strings.Title(w.slug))
					if innerDir, errp := fs.DirByPath(innerDirPath); errp == nil {
						if isEmpty, _ := innerDir.IsEmpty(fs); isEmpty {
							w.Logger(ctx).Warnf("Deleting empty directory for konnector: %q:%q", innerDir.ID(), normalizedFolderPath)
							_ = fs.DeleteDirDoc(innerDir)
						}
					}
				}
				if isEmpty, _ := dir.IsEmpty(fs); isEmpty {
					w.Logger(ctx).Warnf("Deleting empty directory for konnector: %q:%q", dir.ID(), normalizedFolderPath)
					_ = fs.DeleteDirDoc(dir)
				}
			}
		}
	}

	// 1. Check if the folder identified by its ID exists
	if msg.FolderToSave != "" {
		dir, err := fs.DirByID(msg.FolderToSave)
		if err == nil {
			if !strings.HasPrefix(dir.Fullpath, vfs.TrashDirName) {
				if len(dir.ReferencedBy) == 0 {
					dir.AddReferencedBy(couchdb.DocReference{
						Type: consts.Konnectors,
						ID:   consts.Konnectors + "/" + w.slug,
					})
					_ = couchdb.UpdateDoc(inst, dir)
				}
				return nil
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	// 2. Check if the konnector has a reference to a folder
	start := []string{consts.Konnectors, consts.Konnectors + "/" + w.slug}
	end := []string{start[0], start[1], couchdb.MaxString}
	req := &couchdb.ViewRequest{
		StartKey:    start,
		EndKey:      end,
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse
	if err := couchdb.ExecView(inst, couchdb.FilesReferencedByView, req, &res); err == nil {
		count := 0
		dirID := ""
		for _, row := range res.Rows {
			dir := &vfs.DirDoc{}
			if err := couchdb.GetDoc(inst, consts.Files, row.ID, dir); err == nil {
				if !strings.HasPrefix(dir.Fullpath, vfs.TrashDirName) {
					count++
					dirID = row.ID
				}
			}
		}
		if count == 1 {
			msg.updateFolderToSave(dirID)
			return nil
		}
	}

	// 3 Check if a folder should be created
	if acc == nil {
		return nil
	}
	if msg.FolderToSave == "" && acc.FolderPath == "" && (acc.Basic == nil || acc.Basic.FolderPath == "") {
		return nil
	}

	// 4. Recreate the folder
	folderPath := acc.FolderPath
	if folderPath == "" && acc.Basic != nil {
		folderPath = acc.Basic.FolderPath
	}
	if folderPath == "" {
		folderPath = normalizedFolderPath
	}

	dir, err := vfs.MkdirAll(fs, folderPath)
	if err != nil {
		log := inst.Logger().WithField("nspace", "konnector")
		log.Warnf("Can't create the default folder %s: %s", folderPath, err)
		return err
	}
	msg.updateFolderToSave(dir.ID())
	if len(dir.ReferencedBy) == 0 {
		dir.AddReferencedBy(couchdb.DocReference{
			Type: consts.Konnectors,
			ID:   consts.Konnectors + "/" + w.slug,
		})
		_ = couchdb.UpdateDoc(inst, dir)
	}
	return nil
}

// ensurePermissions checks that the konnector has the permissions to write
// files in the folder referenced by the konnector, and adds the permission if
// needed.
func (w *konnectorWorker) ensurePermissions(inst *instance.Instance) error {
	perms, err := permission.GetForKonnector(inst, w.slug)
	if err != nil {
		return err
	}
	value := consts.Konnectors + "/" + w.slug
	for _, rule := range perms.Permissions {
		if rule.Type == consts.Files && rule.Selector == couchdb.SelectorReferencedBy {
			for _, val := range rule.Values {
				if val == value {
					return nil
				}
			}
		}
	}
	rule := permission.Rule{
		Type:        consts.Files,
		Title:       "referenced folders",
		Description: "folders referenced by the konnector",
		Selector:    couchdb.SelectorReferencedBy,
		Values:      []string{value},
	}
	perms.Permissions = append(perms.Permissions, rule)
	return couchdb.UpdateDoc(inst, perms)
}

func copyFiles(workFS afero.Fs, fileServer appfs.FileServer, slug, version, shasum string) error {
	files, err := fileServer.FilesList(slug, version, shasum)
	if err != nil {
		return err
	}
	for _, file := range files {
		var src io.ReadCloser
		var dst io.WriteCloser
		src, err = fileServer.Open(slug, version, shasum, file)
		if err != nil {
			return err
		}
		dirname := path.Dir(file)
		if dirname != "." {
			if err = workFS.MkdirAll(dirname, 0755); err != nil {
				return err
			}
		}
		dst, err = workFS.OpenFile(file, os.O_CREATE|os.O_WRONLY, 0640)
		if err != nil {
			return err
		}
		_, err = io.Copy(dst, src)
		errc1 := dst.Close()
		errc2 := src.Close()
		if err != nil {
			return err
		}
		if errc1 != nil {
			return errc1
		}
		if errc2 != nil {
			return errc2
		}
	}
	return nil
}

func extractTar(workFS afero.Fs, tarFile io.ReadCloser) error {
	tr := tar.NewReader(tarFile)
	for {
		var hdr *tar.Header
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		dirname := path.Dir(hdr.Name)
		if dirname != "." {
			if err = workFS.MkdirAll(dirname, 0755); err != nil {
				return err
			}
		}
		var f afero.File
		f, err = workFS.OpenFile(hdr.Name, os.O_CREATE|os.O_WRONLY, 0640)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		errc := f.Close()
		if err != nil {
			return err
		}
		if errc != nil {
			return errc
		}
	}
}

func (w *konnectorWorker) Slug() string {
	return w.slug
}

func (w *konnectorWorker) PrepareCmdEnv(ctx *job.WorkerContext, i *instance.Instance) (cmd string, env []string, err error) {
	var parameters interface{} = w.man.Parameters

	accountTypes, err := account.FindAccountTypesBySlug(w.slug)
	if err == nil && len(accountTypes) == 1 && accountTypes[0].GrantMode == "secret" {
		secret := accountTypes[0].Secret
		if w.man.Parameters == nil {
			parameters = map[string]interface{}{"secret": secret}
		} else {
			var params map[string]interface{}
			_ = json.Unmarshal(*w.man.Parameters, &params)
			params["secret"] = secret
			parameters = params
		}
	}

	paramsJSON, err := json.Marshal(parameters)
	if err != nil {
		return
	}

	language := w.man.Language
	if language == "" {
		language = "node"
	}

	// Directly pass the job message as fields parameters
	fieldsJSON := w.msg.ToJSON()
	token := i.BuildKonnectorToken(w.man.Slug())

	cmd = config.GetConfig().Konnectors.Cmd
	env = []string{
		"COZY_URL=" + i.PageURL("/", nil),
		"COZY_CREDENTIALS=" + token,
		"COZY_FIELDS=" + fieldsJSON,
		"COZY_PARAMETERS=" + string(paramsJSON),
		"COZY_LANGUAGE=" + language,
		"COZY_LOCALE=" + i.Locale,
		"COZY_TIME_LIMIT=" + ctxToTimeLimit(ctx),
		"COZY_JOB_ID=" + ctx.ID(),
		"COZY_JOB_MANUAL_EXECUTION=" + strconv.FormatBool(ctx.Manual()),
	}
	return
}

func (w *konnectorWorker) Logger(ctx *job.WorkerContext) *logrus.Entry {
	return ctx.Logger().WithField("slug", w.slug)
}

func (w *konnectorWorker) ScanOutput(ctx *job.WorkerContext, i *instance.Instance, line []byte) error {
	var msg struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		NoRetry bool   `json:"no_retry"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return fmt.Errorf("Could not parse stdout as JSON: %q", string(line))
	}

	log := w.Logger(ctx)
	switch msg.Type {
	case konnectorMsgTypeDebug, konnectorMsgTypeInfo:
		log.Debug(msg.Message)
	case konnectorMsgTypeWarning, "warn":
		log.Warn(msg.Message)
	case konnectorMsgTypeError:
		// For retro-compatibility, we still use "error" logs as returned error,
		// only in the case that no "critical" message are actually returned. In
		// such case, We use the last "error" log as the returned error.
		w.lastErr = errors.New(msg.Message)
		log.Error(msg.Message)
	case konnectorMsgTypeCritical:
		w.err = errors.New(msg.Message)
		if msg.NoRetry {
			ctx.SetNoRetry()
		}
		log.Error(msg.Message)
	}

	realtime.GetHub().Publish(i,
		realtime.EventCreate,
		couchdb.JSONDoc{Type: consts.JobEvents, M: map[string]interface{}{
			"type":    msg.Type,
			"message": msg.Message,
		}},
		nil)
	return nil
}

func (w *konnectorWorker) Error(i *instance.Instance, err error) error {
	if w.err != nil {
		return w.err
	}
	if w.lastErr != nil {
		return w.lastErr
	}
	return err
}

func (w *konnectorWorker) Commit(ctx *job.WorkerContext, errjob error) error {
	log := w.Logger(ctx)
	if w.msg != nil {
		log = log.WithField("account_id", w.msg.Account)
	}
	if w.man != nil {
		log = log.WithField("version", w.man.Version())
	}
	if errjob == nil {
		log.Info("Konnector success")
	} else {
		log.Infof("Konnector failure: %s", errjob)
	}
	return nil
}
