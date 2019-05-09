package move

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/mail"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "export",
		Concurrency:  4,
		MaxExecCount: 1,
		Timeout:      5 * 60 * time.Second,
		WorkerFunc:   ExportWorker,
	})
}

// ExportOptions contains the options for launching the export worker.
type ExportOptions struct {
	PartsSize    int64         `json:"parts_size"`
	MaxAge       time.Duration `json:"max_age"`
	WithDoctypes []string      `json:"with_doctypes,omitempty"`
	WithoutFiles bool          `json:"without_files,omitempty"`
}

// ExportWorker is the worker responsible for creating an export of the
// instance.
func ExportWorker(c *job.WorkerContext) error {
	var opts ExportOptions
	if err := c.UnmarshalMessage(&opts); err != nil {
		return err
	}

	exportDoc, err := Export(c.Instance, opts, SystemArchiver())
	if err != nil {
		return err
	}

	mac := base64.URLEncoding.EncodeToString(exportDoc.GenerateAuthMessage(c.Instance))
	link := c.Instance.SubDomain(consts.SettingsSlug)
	link.Fragment = fmt.Sprintf("/exports/%s", mac)
	publicName, err := c.Instance.PublicName()
	if err != nil {
		return err
	}
	mail := mail.Options{
		Mode:         mail.ModeNoReply,
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
