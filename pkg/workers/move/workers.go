package move

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/workers/mails"
)

func init() {
	jobs.AddWorker(&jobs.WorkerConfig{
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
func ExportWorker(c *jobs.WorkerContext) error {
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
	mail := mails.Options{
		Mode:         mails.ModeNoReply,
		TemplateName: "archiver",
		TemplateValues: map[string]interface{}{
			"ArchiveLink": link.String(),
		},
	}

	msg, err := jobs.NewMessage(&mail)
	if err != nil {
		return err
	}

	_, err = jobs.System().PushJob(c.Instance, &jobs.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}
