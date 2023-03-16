package notes

import (
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/note"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "notes-save",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Reserved:     true,
		Timeout:      30 * time.Second,
		WorkerFunc:   WorkerPersist,
	})
}

// WorkerPersist is used to persist a note to its file in the VFS. The changes
// (title and steps) on a notes can happen with a high frequency, and
// debouncing them allows to not make too many calls to Swift.
func WorkerPersist(ctx *job.WorkerContext) error {
	var msg note.DebounceMessage
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	ctx.Logger().Debugf("Persist %#v", msg)
	err := note.Update(ctx.Instance, msg.NoteID)
	if err != nil {
		ctx.Logger().Warnf("Cannot persist note %s: %s", msg.NoteID, err)
	}
	return err
}
