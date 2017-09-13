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

	"github.com/spf13/afero"
)

// KonnectorOptions contains the options to execute a konnector.
type KonnectorOptions struct {
	Konnector    string `json:"konnector"`
	Account      string `json:"account"`
	FolderToSave string `json:"folder_to_save"`
}

type konnectorWorker struct {
	opts     *KonnectorOptions
	man      *apps.KonnManifest
	messages []konnectorMsg
}

// result stores the result of a konnector execution.
type result struct {
	DocID       string         `json:"_id,omitempty"`
	DocRev      string         `json:"_rev,omitempty"`
	CreatedAt   time.Time      `json:"last_execution"`
	LastSuccess time.Time      `json:"last_success"`
	Logs        []konnectorMsg `json:"logs"`
	Account     string         `json:"account"`
	State       string         `json:"state"`
	Error       string         `json:"error"`
}

func (r *result) ID() string         { return r.DocID }
func (r *result) Rev() string        { return r.DocRev }
func (r *result) DocType() string    { return consts.KonnectorResults }
func (r *result) Clone() couchdb.Doc { c := *r; return &c }
func (r *result) SetID(id string)    { r.DocID = id }
func (r *result) SetRev(rev string)  { r.DocRev = rev }

const konnectorMsgTypeError string = "error"

// const konnectorMsgTypeDebug string = "debug"
// const konnectorMsgTypeWarning string = "warning"
// const konnectorMsgTypeProgress string = "progress"

type konnectorMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type konnectorLogs struct {
	Slug     string         `json:"_id,omitempty"`
	DocRev   string         `json:"_rev,omitempty"`
	Messages []konnectorMsg `json:"logs"`
}

func (kl *konnectorLogs) ID() string         { return kl.Slug }
func (kl *konnectorLogs) Rev() string        { return kl.DocRev }
func (kl *konnectorLogs) DocType() string    { return consts.KonnectorLogs }
func (kl *konnectorLogs) Clone() couchdb.Doc { c := *kl; return &c }
func (kl *konnectorLogs) SetID(id string)    {}
func (kl *konnectorLogs) SetRev(rev string)  { kl.DocRev = rev }

func (w *konnectorWorker) PrepareWorkDir(i *instance.Instance, m *jobs.Message) (workDir string, err error) {
	opts := &KonnectorOptions{}
	if err = m.Unmarshal(&opts); err != nil {
		return
	}

	slug := opts.Konnector

	man, err := apps.GetKonnectorBySlug(i, slug)
	if err != nil {
		return
	}
	if man.State() != apps.Ready {
		err = errors.New("Konnector is not ready")
		return
	}

	w.opts = opts
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

func (w *konnectorWorker) PrepareCmdEnv(i *instance.Instance, m *jobs.Message) (cmd string, env []string, jobID string, err error) {
	jobID = fmt.Sprintf("konnector/%s/%s", w.opts.Konnector, i.Domain)

	fields := struct {
		Account      string `json:"account"`
		FolderToSave string `json:"folder_to_save,omitempty"`
	}{
		Account:      w.opts.Account,
		FolderToSave: w.opts.FolderToSave,
	}

	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return
	}

	token := i.BuildKonnectorToken(w.man)

	cmd = config.GetConfig().Konnectors.Cmd
	env = []string{
		"COZY_URL=" + i.PageURL("/", nil),
		"COZY_CREDENTIALS=" + token,
		"COZY_FIELDS=" + string(fieldsJSON),
		"COZY_TYPE=" + w.man.Type,
		"COZY_LOCALE=" + i.Locale,
		"COZY_JOB_ID=" + jobID,
	}
	return
}

func (w *konnectorWorker) ScanOuput(i *instance.Instance, line []byte) error {
	var msg konnectorMsg
	if err := json.Unmarshal(line, &msg); err != nil {
		return fmt.Errorf("Could not parse stdout as JSON: %q", string(line))
	}
	// TODO: filter some of the messages
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
	errLogs := couchdb.Upsert(i, &konnectorLogs{
		Slug:     w.opts.Konnector,
		Messages: w.messages,
	})
	if errLogs != nil {
		fmt.Println("Failed to save konnector logs", errLogs)
	}

	for _, msg := range w.messages {
		if msg.Type == konnectorMsgTypeError {
			// konnector err is more explicit
			return errors.New(msg.Message)
		}
	}

	return err
}

func (w *konnectorWorker) Commit(ctx context.Context, msg *jobs.Message, errjob error) error {
	if w.opts == nil {
		return nil
	}

	slug := w.opts.Konnector
	domain := ctx.Value(jobs.ContextDomainKey).(string)

	inst, err := instance.Get(domain)
	if err != nil {
		return err
	}

	lastResult := &result{}
	err = couchdb.GetDoc(inst, consts.KonnectorResults, slug, lastResult)
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
	result := &result{
		DocID:       slug,
		Account:     w.opts.Account,
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
	// if err != nil {
	// 	return err
	// }

	return err
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
	// msg, err := jobs.NewMessage(jobs.JSONEncoding, &mail)
	// if err != nil {
	// 	return err
	// }
	// log := logger.WithDomain(domain)
	// log.Info("Konnector has failed definitively, should send mail.", mail)
	// _, err = stack.GetBroker().PushJob(&jobs.JobRequest{
	// 	Domain:     domain,
	// 	WorkerType: "sendmail",
	// 	Message:    msg,
	// })
	// return err
}
