package move

import (
	"encoding/base64"
	"net/url"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/workers/mails"
)

func init() {
	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType:   "export",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Timeout:      10 * 60 * time.Second,
		WorkerFunc:   Worker,
	})
}

func Worker(c *jobs.WorkerContext) error {
	i, err := instance.Get(c.Domain())
	if err != nil {
		return err
	}

	exportDoc, err := Export(i, nopArchiver{})
	if err != nil {
		return err
	}

	link := i.PageURL("/move/exports",
		url.Values{"Secret": {base64.URLEncoding.EncodeToString(exportDoc.GenerateAuthMessage(i))}})

	mail := mails.Options{
		Mode:           mails.ModeNoReply,
		TemplateName:   "archiver",
		TemplateValues: map[string]string{"ArchiveLink": link},
	}

	msg, err := jobs.NewMessage(&mail)
	if err != nil {
		return err
	}

	_, err = jobs.System().PushJob(&jobs.JobRequest{
		Domain:     i.Domain,
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}
