package konnectors

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/sirupsen/logrus"

	"github.com/spf13/afero"
)

func init() {
	jobs.AddWorker("konnector", &jobs.WorkerConfig{
		Concurrency:  runtime.NumCPU() * 2,
		MaxExecCount: 2,
		MaxExecTime:  200 * time.Second,
		Timeout:      200 * time.Second,
		WorkerFunc:   Worker,
		WorkerCommit: commit,
	})
}

// Options contains the options to execute a konnector.
type Options struct {
	Konnector    string `json:"konnector"`
	Account      string `json:"account"`
	FolderToSave string `json:"folder_to_save"`
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

// Worker is the worker that runs a konnector by executing an external process.
func Worker(ctx context.Context, m *jobs.Message) error {
	opts := &Options{}
	if err := m.Unmarshal(&opts); err != nil {
		return err
	}

	slug := opts.Konnector
	fields := struct {
		Account      string `json:"account"`
		FolderToSave string `json:"folder_to_save"`
	}{
		Account:      opts.Account,
		FolderToSave: opts.FolderToSave,
	}
	domain := ctx.Value(jobs.ContextDomainKey).(string)
	worker := ctx.Value(jobs.ContextWorkerKey).(string)
	jobID := fmt.Sprintf("%s/%s/%s", worker, slug, domain)

	inst, err := instance.Get(domain)
	if err != nil {
		return err
	}

	man, err := apps.GetKonnectorBySlug(inst, slug)
	if err != nil {
		return err
	}
	if man.State() != apps.Ready {
		return errors.New("Konnector is not ready")
	}

	token := inst.BuildKonnectorToken(man)

	osFS := afero.NewOsFs()
	workDir, err := afero.TempDir(osFS, "", "konnector-"+slug)
	if err != nil {
		return err
	}
	defer osFS.RemoveAll(workDir)
	workFS := afero.NewBasePathFs(osFS, workDir)

	fileServer := inst.KonnectorsFileServer()
	tarFile, err := fileServer.Open(slug, man.Version(), apps.KonnectorArchiveName)
	if err != nil {
		return err
	}

	tr := tar.NewReader(tarFile)
	for {
		var hdr *tar.Header
		hdr, err = tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		dirname := path.Dir(hdr.Name)
		if dirname != "." {
			if err = workFS.MkdirAll(dirname, 0755); err != nil {
				return nil
			}
		}
		var f afero.File
		f, err = workFS.OpenFile(hdr.Name, os.O_CREATE|os.O_WRONLY, 0644)
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

	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return err
	}

	konnCmd := config.GetConfig().Konnectors.Cmd
	cmd := exec.CommandContext(ctx, konnCmd, workDir) // #nosec
	cmd.Env = []string{
		"COZY_URL=" + inst.PageURL("/", nil),
		"COZY_CREDENTIALS=" + token,
		"COZY_FIELDS=" + string(fieldsJSON),
		"COZY_TYPE=" + man.Type,
		"COZY_JOB_ID=" + jobID,
	}

	cmdErr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	cmdOut, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	scanErr := bufio.NewScanner(cmdErr)
	scanOut := bufio.NewScanner(cmdOut)
	scanOut.Buffer(nil, 256*1024)

	var messages []konnectorMsg

	log := logger.WithDomain(domain)

	if err = cmd.Start(); err != nil {
		return wrapErr(ctx, err)
	}

	go doScanErr(jobID, scanErr, log)

	hub := realtime.GetHub()

	for scanOut.Scan() {
		line := scanOut.Bytes()
		var msg konnectorMsg
		if err = json.Unmarshal(line, &msg); err != nil {
			log.Warnf("[konnector] %s: Could not parse stdout as JSON: \"\"", jobID, string(line))
			continue
		}
		// TODO: filter some of the messages
		messages = append(messages, msg)
		hub.Publish(&realtime.Event{
			Verb: realtime.EventCreate,
			Doc: couchdb.JSONDoc{Type: consts.JobEvents, M: map[string]interface{}{
				"type":    msg.Type,
				"message": msg.Message,
			}},
			Domain: domain,
		})
	}

	if err = cmd.Wait(); err != nil {
		err = wrapErr(ctx, err)
		log.Errorf("[konnector] %s: Konnector has failed: %s", jobID, err.Error())
	}

	errLogs := couchdb.Upsert(inst, &konnectorLogs{
		Slug:     slug,
		Messages: messages,
	})
	if errLogs != nil {
		fmt.Println("Failed to save konnector logs", errLogs)
	}

	for _, msg := range messages {
		if msg.Type == konnectorMsgTypeError {
			// konnector err is more explicit
			return errors.New(msg.Message)
		}
	}

	return err
}

func doScanErr(jobID string, scanner *bufio.Scanner, log *logrus.Entry) {
	for scanner.Scan() {
		log.Errorf("[konnector] %s: Stderr: %s", jobID, scanner.Text())
	}
}

func commit(ctx context.Context, m *jobs.Message, errjob error) error {
	opts := &Options{}
	if err := m.Unmarshal(&opts); err != nil {
		return err
	}

	slug := opts.Konnector
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
		Account:     opts.Account,
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

func wrapErr(ctx context.Context, err error) error {
	if ctx.Err() == context.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	return err
}
