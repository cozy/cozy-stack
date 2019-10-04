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
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Timeout:      10 * time.Minute,
		WorkerFunc:   WorkerTrashFiles,
	})
}

// WorkerTrashFiles is a worker to remove files in Swift after they have been
// removed from CouchDB. It is used when cleaning the trash, as removing a lot
// of files from Swift can take some time.
func WorkerTrashFiles(ctx *job.WorkerContext) error {
	opts := vfs.TrashJournal{}
	err := ctx.UnmarshalMessage(&opts)
	if err != nil {
		return err
	}
	fs := ctx.Instance.VFS()
	return fs.EnsureErased(opts)
}
