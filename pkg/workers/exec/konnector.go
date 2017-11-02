package exec

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/sirupsen/logrus"

	"github.com/spf13/afero"
)

type konnectorWorker struct {
	slug string
	msg  map[string]interface{}
	man  *apps.KonnManifest

	err     error
	lastErr error
}

const (
	konnectorMsgTypeDebug    = "debug"
	konnectorMsgTypeWarning  = "warning"
	konnectorMsgTypeError    = "error"
	konnectorMsgTypeCritical = "critical"
)

// konnectorResult stores the result of a konnector execution.
// TODO: remove this type kept for retro-compatibility.
type konnectorResult struct {
	DocID       string    `json:"_id,omitempty"`
	DocRev      string    `json:"_rev,omitempty"`
	CreatedAt   time.Time `json:"last_execution"`
	LastSuccess time.Time `json:"last_success"`
	Account     string    `json:"account"`
	State       string    `json:"state"`
	Error       string    `json:"error"`
}

func (r *konnectorResult) ID() string         { return r.DocID }
func (r *konnectorResult) Rev() string        { return r.DocRev }
func (r *konnectorResult) DocType() string    { return consts.KonnectorResults }
func (r *konnectorResult) Clone() couchdb.Doc { c := *r; return &c }
func (r *konnectorResult) SetID(id string)    { r.DocID = id }
func (r *konnectorResult) SetRev(rev string)  { r.DocRev = rev }

func (w *konnectorWorker) PrepareWorkDir(ctx *jobs.WorkerContext, i *instance.Instance) (workDir string, err error) {
	var msg map[string]interface{}
	if err = ctx.UnmarshalMessage(&msg); err != nil {
		return
	}

	slug, _ := msg["konnector"].(string)
	man, err := apps.GetKonnectorBySlug(i, slug)
	if err != nil {
		return
	}

	// TODO: disallow konnectors on state Installed to be run when we define our
	// workflow to accept permissions changes on konnectors.
	if s := man.State(); s != apps.Ready && s != apps.Installed {
		err = errors.New("Konnector is not ready")
		return
	}

	w.slug = slug
	w.msg = msg
	w.man = man

	osFS := afero.NewOsFs()
	workDir, err = afero.TempDir(osFS, "", "konnector-"+slug)
	if err != nil {
		return
	}
	workFS := afero.NewBasePathFs(osFS, workDir)

	fileServer := i.KonnectorsFileServer()
	tarFile, err := fileServer.Open(slug, man.Version(), apps.KonnectorArchiveName)
	if err != nil {
		return
	}

	// Create the folder in which the konnector has the right to write.
	{
		fs := i.VFS()
		folderToSave, _ := msg["folder_to_save"].(string)
		if folderToSave != "" {
			if _, err = fs.DirByID(folderToSave); os.IsNotExist(err) {
				folderToSave = ""
			}
		}
		if folderToSave == "" {
			defaultFolderPath, _ := msg["default_folder_path"].(string)
			if defaultFolderPath == "" {
				name := i.Translate("Tree Administrative")
				defaultFolderPath = fmt.Sprintf("/%s/%s", name, strings.Title(slug))
			}
			var dir *vfs.DirDoc
			dir, err = vfs.MkdirAll(fs, defaultFolderPath, nil)
			if err != nil {
				log := logger.WithDomain(i.Domain)
				log.Warnf("Can't create the default folder %s for konnector %s: %s", defaultFolderPath, slug, err)
				return
			}
			msg["folder_to_save"] = dir.ID()
		}
	}

	tr := tar.NewReader(tarFile)
	for {
		var hdr *tar.Header
		hdr, err = tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}
		dirname := path.Dir(hdr.Name)
		if dirname != "." {
			if err = workFS.MkdirAll(dirname, 0755); err != nil {
				return
			}
		}
		var f afero.File
		f, err = workFS.OpenFile(hdr.Name, os.O_CREATE|os.O_WRONLY, 0640)
		if err != nil {
			return
		}
		_, err = io.Copy(f, tr)
		errc := f.Close()
		if err != nil {
			return
		}
		if errc != nil {
			err = errc
			return
		}
	}

	return workDir, nil
}

func (w *konnectorWorker) Slug() string {
	return w.slug
}

func (w *konnectorWorker) PrepareCmdEnv(ctx *jobs.WorkerContext, i *instance.Instance) (cmd string, env []string, err error) {
	// Directly pass the job message as fields parameters
	fieldsJSON, err := json.Marshal(w.msg)
	if err != nil {
		return
	}

	paramsJSON, err := json.Marshal(w.man.Parameters)
	if err != nil {
		return
	}

	token := i.BuildKonnectorToken(w.man)

	cmd = config.GetConfig().Konnectors.Cmd
	env = []string{
		"COZY_URL=" + i.PageURL("/", nil),
		"COZY_CREDENTIALS=" + token,
		"COZY_FIELDS=" + string(fieldsJSON),
		"COZY_PARAMETERS=" + string(paramsJSON),
		"COZY_TYPE=" + w.man.Type,
		"COZY_LOCALE=" + i.Locale,
		"COZY_JOB_ID=" + ctx.ID(),
	}
	return
}

func (w *konnectorWorker) ScanOuput(ctx *jobs.WorkerContext, i *instance.Instance, log *logrus.Entry, line []byte) error {
	var msg struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return fmt.Errorf("Could not parse stdout as JSON: %q", string(line))
	}

	switch msg.Type {
	case konnectorMsgTypeDebug:
		log.Debug(msg.Message)
	case konnectorMsgTypeWarning:
		log.Warn(msg.Message)
	case konnectorMsgTypeError:
		// For retro-compatibility, we still use "error" logs as returned error,
		// only in the case that no "critical" message are actually returned. In
		// such case, We use the last "error" log as the returned error.
		w.lastErr = errors.New(msg.Message)
		log.Error(msg.Message)
	case konnectorMsgTypeCritical:
		w.err = errors.New(msg.Message)
		log.Error(msg.Message)
	}

	realtime.GetHub().Publish(&realtime.Event{
		Domain: i.Domain,
		Verb:   realtime.EventCreate,
		Doc: couchdb.JSONDoc{Type: consts.JobEvents, M: map[string]interface{}{
			"type":    msg.Type,
			"message": msg.Message,
		}},
	})
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
	// TODO: remove this retro-compatibility block
	// <<<<<<<<<<<<<
	accountID, _ := w.msg["account"].(string)
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

	var state, errstr string
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

	// sched := globals.GetScheduler()
	// t, err := sched.Get(ctx.Domain(), triggerID)
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
	// 	Subject:      i.Translate("Error Konnector execution", ctx.Domain()),
	// 	TemplateName: "konnector_error_" + i.Locale,
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
	// _, err = globals.GetBroker().PushJob(&jobs.JobRequest{
	// 	Domain:     ctx.Domain(),
	// 	WorkerType: "sendmail",
	// 	Message:    msg,
	// })
	// return err
}
