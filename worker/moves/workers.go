package moves

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/move"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/mail"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "export",
		Concurrency:  4,
		MaxExecCount: 1,
		Timeout:      1 * time.Hour,
		WorkerFunc:   ExportWorker,
	})

	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "import",
		Concurrency:  4,
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
	exportDoc, err := move.Export(c.Instance, opts, archiver)
	if err != nil {
		return err
	}

	mac := base64.URLEncoding.EncodeToString(exportDoc.GenerateAuthMessage(c.Instance))
	link := c.Instance.SubDomain(consts.SettingsSlug)
	link.Fragment = fmt.Sprintf("/exports/%s", mac)
	publicName, _ := c.Instance.PublicName()
	mail := mail.Options{
		Mode:         mail.ModeFromStack,
		TemplateName: "archiver",
		TemplateValues: map[string]interface{}{
			"ArchiveLink": link.String(),
			"PublicName":  publicName,
		},
	}

	msg, err := job.NewMessage(&mail)
	if err != nil {
		return err
	}

	_, err = job.System().PushJob(c.Instance, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}

// ImportWorker is the worker responsible for inserting the data from an export
// inside an instance.
func ImportWorker(c *job.WorkerContext) error {
	var opts move.ImportOptions
	if err := c.UnmarshalMessage(&opts); err != nil {
		return err
	}

	fmt.Printf("manifest_url = %q\n", opts.ManifestURL)
	notInstalled, err := move.Import(c.Instance, opts)

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
