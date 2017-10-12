package exec

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/sirupsen/logrus"

	"github.com/spf13/afero"
)

type konnectorMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type konnectorWorker struct {
	slug     string
	msg      map[string]interface{}
	man      *apps.KonnManifest
	messages []konnectorMsg
}

// konnectorResult stores the result of a konnector execution.
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

const (
	konnectorMsgTypeDebug    = "debug"
	konnectorMsgTypeWarning  = "warning"
	konnectorMsgTypeError    = "error"
	konnectorMsgTypeCritical = "critical"
)

// const konnectorMsgTypeProgress string = "progress"

func (w *konnectorWorker) PrepareWorkDir(i *instance.Instance, m jobs.Message) (workDir string, err error) {
	var msg map[string]interface{}
	if err = m.Unmarshal(&msg); err != nil {
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
		folderToSave, _ := msg["folder_to_save"].(string)
		if folderToSave != "" {
			defaultFolderPath, _ := msg["default_folder_path"].(string)
			if defaultFolderPath == "" {
				defaultFolderPath = fmt.Sprintf("/???/%s", slug)
			}
			fs := i.VFS()
			if _, err = fs.DirByID(folderToSave); os.IsNotExist(err) {
				var dir *vfs.DirDoc
				dir, err = vfs.MkdirAll(fs, defaultFolderPath, nil)
				if err != nil {
					return
				}
				folderToSave = dir.ID()
			}
		}
		msg["folder_to_save"] = folderToSave
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

func (w *konnectorWorker) PrepareCmdEnv(i *instance.Instance, m jobs.Message) (cmd string, env []string, jobID string, err error) {
	jobID = fmt.Sprintf("konnector/%s/%s", w.slug, i.Domain)

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
		"COZY_JOB_ID=" + jobID,
	}
	return
}

func (w *konnectorWorker) ScanOuput(i *instance.Instance, log *logrus.Entry, line []byte) error {
	var msg konnectorMsg
	if err := json.Unmarshal(line, &msg); err != nil {
		return fmt.Errorf("Could not parse stdout as JSON: %q", string(line))
	}

	switch msg.Type {
	case konnectorMsgTypeDebug:
		log.Debug(msg.Message)
	case konnectorMsgTypeWarning:
		log.Warn(msg.Message)
	case konnectorMsgTypeError, konnectorMsgTypeCritical:
		log.Error(msg.Message)
	}

	w.messages = append(w.messages, msg)
	realtime.GetHub().Publish(&realtime.Event{
		Verb: realtime.EventCreate,
		Doc: couchdb.JSONDoc{Type: consts.JobEvents, M: map[string]interface{}{
			"type":    msg.Type,
			"message": msg.Message,
		}},
		Domain: i.Domain,
	})
	return nil
}

func (w *konnectorWorker) Error(i *instance.Instance, err error) error {
	// For retro-compatibility, we still use "error" logs as returned error, only
	// in the case that no "critical" message are actually returned. In such
	// case, We use the last "error" log as the returned error.
	var lastErrorMessage error
	for _, msg := range w.messages {
		if msg.Type == konnectorMsgTypeCritical {
			return errors.New(msg.Message)
		}
		if msg.Type == konnectorMsgTypeError {
			lastErrorMessage = errors.New(msg.Message)
		}
	}
	if lastErrorMessage != nil {
		return lastErrorMessage
	}

	return err
}

func (w *konnectorWorker) Commit(ctx context.Context, msg jobs.Message, errjob error) error {
	if w.msg == nil {
		return nil
	}

	accountID, _ := w.msg["account"].(string)
	domain := ctx.Value(jobs.ContextDomainKey).(string)

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

	// if err != nil {
	// 	return err
	// }

	// // if it is the first try we do not take into account an error, we bail.
	// if lastResult == nil {
	// 	return nil
	// }
	// // if the job has not errored, or the last one was already errored, we bail.
	// if state != jobs.Errored || lastResult.State == jobs.Errored {
	// 	return nil
	// }

	// konnectorURL := inst.SubDomain(consts.CollectSlug)
	// konnectorURL.Fragment = "/category/all/" + slug
	// mail := mails.Options{
	// 	Mode:         mails.ModeNoReply,
	// 	Subject:      inst.Translate("Error Konnector execution", domain),
	// 	TemplateName: "konnector_error_" + inst.Locale,
	// 	TemplateValues: map[string]string{
	// 		"KonnectorName": slug,
	// 		"KonnectorPage": konnectorURL.String(),
	// 	},
	// }
	// msg, err := jobs.NewMessage(&mail)
	// if err != nil {
	// 	return err
	// }
	// log := logger.WithDomain(domain)
	// log.Info("Konnector has failed definitively, should send mail.", mail)
	// _, err = globals.GetBroker().PushJob(&jobs.JobRequest{
	// 	Domain:     domain,
	// 	WorkerType: "sendmail",
	// 	Message:    msg,
	// })
	// return err
}
