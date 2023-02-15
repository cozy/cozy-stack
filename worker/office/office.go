package office

import (
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/office"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "office-save",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Reserved:     true,
		Timeout:      30 * time.Second,
		WorkerFunc:   WorkerSave,
	})
}

// WorkerSave is used for asking OnlyOffice to save a document in the Cozy.
func WorkerSave(ctx *job.WorkerContext) error {
	var msg office.SendSaveMessage
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	return office.SendSave(ctx.Instance, msg)
}
