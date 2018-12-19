package exec

import (
	"bufio"
	"bytes"
	"context"
	"math"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/metrics"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

var defaultTimeout = 300 * time.Second

func init() {
	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType: "konnector",
		WorkerStart: func(ctx *jobs.WorkerContext) (*jobs.WorkerContext, error) {
			return ctx.WithCookie(&konnectorWorker{}), nil
		},
		BeforeHook:   beforeHookKonnector,
		WorkerFunc:   worker,
		WorkerCommit: commit,
		Concurrency:  runtime.NumCPU() * 2,
		MaxExecCount: 2,
		Timeout:      defaultTimeout,
	})

	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType: "service",
		WorkerStart: func(ctx *jobs.WorkerContext) (*jobs.WorkerContext, error) {
			return ctx.WithCookie(&serviceWorker{}), nil
		},
		WorkerFunc:   worker,
		WorkerCommit: commit,
		Concurrency:  runtime.NumCPU() * 2,
		MaxExecCount: 2,
		Timeout:      defaultTimeout,
	})
}

type execWorker interface {
	Slug() string
	PrepareWorkDir(ctx *jobs.WorkerContext, i *instance.Instance) (workDir string, err error)
	PrepareCmdEnv(ctx *jobs.WorkerContext, i *instance.Instance) (cmd string, env []string, err error)
	ScanOutput(ctx *jobs.WorkerContext, i *instance.Instance, line []byte) error
	Error(i *instance.Instance, err error) error
	Logger(ctx *jobs.WorkerContext) *logrus.Entry
	Commit(ctx *jobs.WorkerContext, errjob error) error
}

func worker(ctx *jobs.WorkerContext) (err error) {
	worker := ctx.Cookie().(execWorker)
	domain := ctx.Domain()

	inst, err := instance.Get(domain)
	if err != nil {
		return err
	}

	workDir, err := worker.PrepareWorkDir(ctx, inst)
	if err != nil {
		worker.Logger(ctx).Errorf("PrepareWorkDir: %s", err)
		return err
	}
	defer os.RemoveAll(workDir)

	cmdStr, env, err := worker.PrepareCmdEnv(ctx, inst)
	if err != nil {
		worker.Logger(ctx).Errorf("PrepareCmdEnv: %s", err)
		return err
	}

	var stderrBuf bytes.Buffer
	cmd := createCmd(cmdStr, workDir) // #nosec
	cmd.Env = env

	// set stderr writable with a bytes.Buffer limited total size of 256Ko
	cmd.Stderr = utils.LimitWriterDiscard(&stderrBuf, 256*1024)

	// Log out all things printed in stderr, whatever the result of the
	// konnector is.
	log := worker.Logger(ctx)
	defer func() {
		if stderrBuf.Len() > 0 {
			log.Error("Stderr: ", stderrBuf.String())
		}
	}()

	cmdOut, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	scanBuf := make([]byte, 16*1024)
	scanOut := bufio.NewScanner(cmdOut)
	scanOut.Buffer(scanBuf, 64*1024)

	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		var result string
		if err != nil {
			result = metrics.WorkerExecResultErrored
		} else {
			result = metrics.WorkerExecResultSuccess
		}
		metrics.WorkersKonnectorsExecDurations.
			WithLabelValues(worker.Slug(), result).
			Observe(v)
	}))
	defer timer.ObserveDuration()

	if err = cmd.Start(); err != nil {
		return wrapErr(ctx, err)
	}

	waitDone := make(chan error)
	go func() {
		for scanOut.Scan() {
			if errOut := worker.ScanOutput(ctx, inst, scanOut.Bytes()); errOut != nil {
				log.Error(errOut)
			}
		}
		if errs := scanOut.Err(); errs != nil {
			log.Errorf("could not scan stdout: %s", errs)
		}
		waitDone <- cmd.Wait()
		close(waitDone)
	}()

	select {
	case err = <-waitDone:
	case <-ctx.Done():
		err = ctx.Err()
		killCmd(cmd)
		<-waitDone
	}

	return worker.Error(inst, err)
}

func commit(ctx *jobs.WorkerContext, errjob error) error {
	return ctx.Cookie().(execWorker).Commit(ctx, errjob)
}

func ctxToTimeLimit(ctx *jobs.WorkerContext) string {
	var limit float64
	if deadline, ok := ctx.Deadline(); ok {
		limit = time.Until(deadline).Seconds()
	}
	if limit <= 0 {
		limit = defaultTimeout.Seconds()
	}
	// add a little gap of 5 seconds to prevent racing the two deadlines
	return strconv.Itoa(int(math.Ceil(limit)) + 5)
}

func wrapErr(ctx context.Context, err error) error {
	if ctx.Err() == context.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	return err
}
