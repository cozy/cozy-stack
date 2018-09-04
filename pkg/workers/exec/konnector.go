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
	"time"

	"github.com/cozy/cozy-stack/pkg/accounts"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/sirupsen/logrus"

	"github.com/cozy/afero"
)

const (
	konnErrorLoginFailed      = "LOGIN_FAILED"
	konnErrorUserActionNeeded = "USER_ACTION_NEEDED"
)

type konnectorWorker struct {
	slug string
	msg  *konnectorMessage
	man  *apps.KonnManifest

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

type konnectorMessage struct {
	Account           string `json:"account"`
	Konnector         string `json:"konnector"`
	FolderToSave      string `json:"folder_to_save"`
	DefaultFolderPath string `json:"default_folder_path"`
	AccountDeleted    bool   `json:"account_deleted"`

	// Data contains the original value of the message, even fields that are not
	// part of our message definition.
	data json.RawMessage
}

func (m *konnectorMessage) ToJSON() string {
	return string(m.data)
}

// konnectorResult stores the result of a konnector execution.
// TODO: remove this type kept for retro-compatibility.
type konnectorResult struct {
	DocID       string     `json:"_id,omitempty"`
	DocRev      string     `json:"_rev,omitempty"`
	CreatedAt   time.Time  `json:"last_execution"`
	LastSuccess time.Time  `json:"last_success"`
	Account     string     `json:"account"`
	AccountRev  string     `json:"account_rev"`
	State       jobs.State `json:"state"`
	Error       string     `json:"error"`
}

// beforeHookKonnector skips jobs from trigger that are failing on certain
// errors.
func beforeHookKonnector(req *jobs.JobRequest) (bool, error) {
	if req.Manual || req.Trigger == nil {
		return true, nil
	}
	trigger := req.Trigger
	state, err := jobs.GetTriggerState(trigger)
	if err != nil {
		return false, err
	}
	if state.Status == jobs.Errored {
		if strings.HasPrefix(state.LastError, konnErrorLoginFailed) ||
			strings.HasPrefix(state.LastError, konnErrorUserActionNeeded) {
			return false, nil
		}
	}
	return true, nil
}

func (r *konnectorResult) ID() string         { return r.DocID }
func (r *konnectorResult) Rev() string        { return r.DocRev }
func (r *konnectorResult) DocType() string    { return consts.KonnectorResults }
func (r *konnectorResult) Clone() couchdb.Doc { c := *r; return &c }
func (r *konnectorResult) SetID(id string)    { r.DocID = id }
func (r *konnectorResult) SetRev(rev string)  { r.DocRev = rev }

func (w *konnectorWorker) PrepareWorkDir(ctx *jobs.WorkerContext, i *instance.Instance) (string, error) {
	var err error
	var data json.RawMessage
	var msg konnectorMessage
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
	w.man, err = apps.GetKonnectorBySlug(i, slug)
	if err == apps.ErrNotFound {
		return "", jobs.ErrBadTrigger{Err: err}
	} else if err != nil {
		return "", err
	}

	// Check that the associated account is present.
	var account *accounts.Account
	if msg.Account != "" && !ctx.Manual() && !msg.AccountDeleted {
		account := &accounts.Account{}
		err = couchdb.GetDoc(i, consts.Accounts, msg.Account, account)
		if couchdb.IsNotFoundError(err) {
			return "", jobs.ErrBadTrigger{Err: err}
		}
	}

	// TODO: disallow konnectors on state Installed to be run when we define our
	// workflow to accept permissions changes on konnectors.
	man := w.man
	if s := man.State(); s != apps.Ready && s != apps.Installed {
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
	tarFile, err := fileServer.Open(slug, man.Version(), apps.KonnectorArchiveName)
	if err == nil {
		err = extractTar(workFS, tarFile)
		if errc := tarFile.Close(); err == nil {
			err = errc
		}
	} else if os.IsNotExist(err) {
		err = copyFiles(workFS, fileServer, slug, man.Version())
	}
	if err != nil {
		return "", err
	}

	// Create the folder in which the konnector has the right to write.
	if err = ensureFolderToSave(i, &msg, slug, account); err != nil {
		return "", nil
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
			return "", jobs.ErrAbort
		}
		workDir = path.Join(workDir, fileExecPath)
	}

	return workDir, nil
}

// ensureFolderToSave tries hard to give a folder to the konnector where it can
// write its files if it needs to do so.
func ensureFolderToSave(inst *instance.Instance, msg *konnectorMessage, slug string, account *accounts.Account) error {
	fs := inst.VFS()

	// 1. Check if the folder identified by its ID exists
	if msg.FolderToSave != "" {
		dir, err := fs.DirByID(msg.FolderToSave)
		if err == nil {
			if !strings.HasPrefix(dir.Fullpath, vfs.TrashDirName) {
				return nil
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	// 2. Check if the konnector has a reference to a folder
	start := []string{consts.Konnectors, consts.Konnectors + "/" + slug}
	end := []string{start[0], start[1], couchdb.MaxString}
	req := &couchdb.ViewRequest{
		StartKey:    start,
		EndKey:      end,
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse
	if err := couchdb.ExecView(inst, consts.FilesReferencedByView, req, &res); err == nil {
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
			msg.FolderToSave = dirID
			return nil
		}
	}

	// 3. Recreate the folder
	if account == nil {
		return nil
	}
	admin := inst.Translate("Tree Administrative")
	r := strings.NewReplacer("&", "_", "/", "_", "\\", "_", "#", "_",
		",", "_", "+", "_", "(", "_", ")", "_", "$", "_", "@", "_", "~",
		"_", "%", "_", ".", "_", "'", "_", "\"", "_", ":", "_", "*", "_",
		"?", "_", "<", "_", ">", "_", "{", "_", "}", "_")
	accountName := r.Replace(account.Name)
	folderPath := fmt.Sprintf("/%s/%s/%s", admin, strings.Title(slug), accountName)
	dir, err := vfs.MkdirAll(fs, folderPath)
	if err != nil {
		log := inst.Logger().WithField("nspace", "konnector")
		log.Warnf("Can't create the default folder %s: %s", folderPath, err)
		return err
	}
	msg.FolderToSave = dir.ID()
	if len(dir.ReferencedBy) == 0 {
		dir.AddReferencedBy(couchdb.DocReference{
			Type: consts.Konnectors,
			ID:   consts.Konnectors + "/" + slug,
		})
		couchdb.UpdateDoc(inst, dir)
	}
	return nil
}

func copyFiles(workFS afero.Fs, fileServer apps.FileServer, slug, version string) error {
	files, err := fileServer.FilesList(slug, version)
	if err != nil {
		return err
	}
	for _, file := range files {
		var src io.ReadCloser
		var dst io.WriteCloser
		src, err = fileServer.Open(slug, version, file)
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

func (w *konnectorWorker) PrepareCmdEnv(ctx *jobs.WorkerContext, i *instance.Instance) (cmd string, env []string, err error) {
	paramsJSON, err := json.Marshal(w.man.Parameters)
	if err != nil {
		return
	}

	language := w.man.Language
	if language == "" {
		language = "node"
	}

	// Directly pass the job message as fields parameters
	fieldsJSON := w.msg.ToJSON()
	token := i.BuildKonnectorToken(w.man)

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

func (w *konnectorWorker) Logger(ctx *jobs.WorkerContext) *logrus.Entry {
	return ctx.Logger().WithField("slug", w.slug)
}

func (w *konnectorWorker) ScanOutput(ctx *jobs.WorkerContext, i *instance.Instance, line []byte) error {
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

func (w *konnectorWorker) Commit(ctx *jobs.WorkerContext, errjob error) error {
	if w.msg == nil {
		return nil
	}

	// TODO: remove this retro-compatibility block
	// <<<<<<<<<<<<<
	accountID := w.msg.Account
	domain := ctx.Domain()

	inst, err := instance.Get(domain)
	if err != nil {
		return err
	}

	lastResult := &konnectorResult{}
	err = couchdb.GetDoc(inst, consts.KonnectorResults, w.slug, lastResult)
	if err != nil {
		if !couchdb.IsNotFoundError(err) {
			return err
		}
		lastResult = nil
	}

	var state jobs.State
	var errstr string
	var lastSuccess time.Time
	if errjob != nil {
		if lastResult != nil {
			lastSuccess = lastResult.LastSuccess
		}
		errstr = errjob.Error()
		state = jobs.Errored
	} else {
		lastSuccess = time.Now()
		state = jobs.Done
	}

	result := &konnectorResult{
		DocID:       w.slug,
		Account:     accountID,
		CreatedAt:   time.Now(),
		LastSuccess: lastSuccess,
		State:       state,
		Error:       errstr,
	}
	if lastResult == nil {
		err = couchdb.CreateNamedDocWithDB(inst, result)
	} else {
		result.SetRev(lastResult.Rev())
		err = couchdb.UpdateDoc(inst, result)
	}
	return err
	// >>>>>>>>>>>>>

	// if errjob == nil {
	//  return nil
	// }

	// triggerID, ok := ctx.TriggerID()
	// if !ok {
	// 	return nil
	// }

	// sched := jobs.System()
	// t, err := sched.GetTrigger(ctx.Domain(), triggerID)
	// if err != nil {
	// 	return err
	// }

	// lastJob, err := scheduler.GetLastJob(t)
	// // if it is the first try we do not take into account an error, we bail.
	// if err == scheduler.ErrNotFoundTrigger {
	// 	return nil
	// }
	// if err != nil {
	// 	return err
	// }

	// // if the last job was already errored, we bail.
	// if lastJob.State == jobs.Errored {
	// 	return nil
	// }

	// i, err := instance.Get(ctx.Domain())
	// if err != nil {
	// 	return err
	// }

	// konnectorURL := i.SubDomain(consts.CollectSlug)
	// konnectorURL.Fragment = "/category/all/" + w.slug
	// mail := mails.Options{
	// 	Mode:         mails.ModeNoReply,
	// 	TemplateName: "konnector_error",
	// 	TemplateValues: map[string]string{
	// 		"KonnectorName": w.slug,
	// 		"KonnectorPage": konnectorURL.String(),
	// 	},
	// }

	// msg, err := jobs.NewMessage(&mail)
	// if err != nil {
	// 	return err
	// }

	// ctx.Logger().Info("Konnector has failed definitively, should send mail.", mail)
	// _, err = jobs.System().PushJob(&jobs.JobRequest{
	// 	Domain:     ctx.Domain(),
	// 	WorkerType: "sendmail",
	// 	Message:    msg,
	// })
	// return err
}
