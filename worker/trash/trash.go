package trash

import (
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "trash-files",
		Concurrency:  runtime.NumCPU() * 4,
		MaxExecCount: 2,
		Reserved:     true,
		Timeout:      2 * time.Hour,
		WorkerFunc:   WorkerTrashFiles,
	})
}

// WorkerTrashFiles is a worker to remove files in Swift after they have been
// removed from CouchDB. It is used when cleaning the trash, as removing a lot
// of files from Swift can take some time.
func WorkerTrashFiles(ctx *job.WorkerContext) error {
	opts := vfs.TrashJournal{}
	if err := ctx.UnmarshalMessage(&opts); err != nil {
		return err
	}
	fs := ctx.Instance.VFS()
	if err := fs.EnsureErased(opts); err != nil {
		ctx.Logger().WithField("critical", "true").
			Errorf("Error: %s", err)
		return err
	}
	return nil
}
