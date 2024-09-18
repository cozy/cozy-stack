package rag

import (
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/rag"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "rag-index",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Reserved:     true,
		Timeout:      15 * time.Minute,
		WorkerFunc:   WorkerIndex,
	})
}

func WorkerIndex(ctx *job.TaskContext) error {
	logger := ctx.Logger()
	var msg rag.IndexMessage
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	logger.Debugf("RAG: index %s", msg.Doctype)
	return rag.Index(ctx.Instance, logger, msg)
}
