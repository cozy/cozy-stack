package moves

import (
	"runtime"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/move"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/mail"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "export",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Timeout:      1 * time.Hour,
		WorkerFunc:   ExportWorker,
	})

	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "import",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Timeout:      1 * time.Hour,
		WorkerFunc:   ImportWorker,
	})
}

// ExportWorker is the worker responsible for creating an export of the
// instance.
func ExportWorker(c *job.WorkerContext) error {
	var opts move.ExportOptions
	if err := c.UnmarshalMessage(&opts); err != nil {
		return err
	}

	if opts.ContextualDomain != "" {
		c.Instance = c.Instance.WithContextualDomain(opts.ContextualDomain)
	}

	archiver := move.SystemArchiver()
	exportDoc, err := move.CreateExport(c.Instance, opts, archiver)
	if err != nil {
		return err
	}

	if opts.MoveTo == nil {
		return exportDoc.SendExportMail(c.Instance)
	}

	return exportDoc.NotifyTarget(c.Instance, opts.MoveTo)
}

// ImportWorker is the worker responsible for inserting the data from an export
// inside an instance.
func ImportWorker(c *job.WorkerContext) error {
	var opts move.ImportOptions
	if err := c.UnmarshalMessage(&opts); err != nil {
		return err
	}

	if err := lifecycle.Block(c.Instance, instance.BlockedImporting.Code); err != nil {
		return err
	}

	notInstalled, err := move.Import(c.Instance, opts)

	if erru := lifecycle.Unblock(c.Instance); erru != nil {
		// Try again
		time.Sleep(10 * time.Second)
		inst, errg := instance.GetFromCouch(c.Instance.Domain)
		if errg == nil {
			erru = lifecycle.Unblock(inst)
		}
		if err == nil {
			err = erru
		}
	}

	var email mail.Options
	if err == nil {
		publicName, _ := c.Instance.PublicName()
		link := c.Instance.SubDomain(consts.HomeSlug)
		email = mail.Options{
			Mode:         mail.ModeFromStack,
			TemplateName: "import_success",
			TemplateValues: map[string]interface{}{
				"AppsNotInstalled": strings.Join(notInstalled, ", "),
				"CozyLink":         link.String(),
				"PublicName":       publicName,
			},
		}
	} else {
		c.Instance.Logger().WithField("nspace", "move").
			Warnf("Import failed: %s", err)
		email = mail.Options{
			Mode:         mail.ModeFromStack,
			TemplateName: "import_error",
		}
	}

	msg, err := job.NewMessage(&email)
	if err != nil {
		return err
	}
	_, err = job.System().PushJob(c.Instance, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}
