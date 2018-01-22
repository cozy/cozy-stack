package exec

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/metrics"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

func init() {
	addExecWorker("konnector", &jobs.WorkerConfig{
		Concurrency:  runtime.NumCPU() * 2,
		MaxExecCount: 2,
		Timeout:      200 * time.Second,
	}, func() execWorker {
		return &konnectorWorker{}
	})

	addExecWorker("service", &jobs.WorkerConfig{
		Concurrency:  runtime.NumCPU() * 2,
		MaxExecCount: 2,
		Timeout:      200 * time.Second,
	}, func() execWorker {
		return &serviceWorker{}
	})
}

type execWorker interface {
	Slug() string
	PrepareWorkDir(ctx *jobs.WorkerContext, i *instance.Instance) (workDir string, err error)
	PrepareCmdEnv(ctx *jobs.WorkerContext, i *instance.Instance) (cmd string, env []string, err error)
	ScanOutput(ctx *jobs.WorkerContext, i *instance.Instance, log *logrus.Entry, line []byte) error
	Error(i *instance.Instance, err error) error
	Commit(ctx *jobs.WorkerContext, errjob error) error
}

func makeExecWorkerFunc() jobs.WorkerFunc {
	return func(ctx *jobs.WorkerContext) (err error) {
		worker := ctx.Cookie().(execWorker)
		domain := ctx.Domain()

		inst, err := instance.Get(domain)
		if err != nil {
			return err
		}

		workDir, err := worker.PrepareWorkDir(ctx, inst)
		if err != nil {
			return err
		}
		defer os.RemoveAll(workDir)

		cmdStr, env, err := worker.PrepareCmdEnv(ctx, inst)
		if err != nil {
			return err
		}

		log := ctx.Logger()

		var stderrBuf bytes.Buffer
		cmd := exec.CommandContext(ctx, cmdStr, workDir) // #nosec
		cmd.Env = env

		// set stderr writable with a bytes.Buffer limited total size of 256Ko
		cmd.Stderr = utils.LimitWriterDiscard(&stderrBuf, 256*1024)

		// Log out all things printed in stderr, whatever the result of the
		// konnector is.
		defer func() {
			if stderrBuf.Len() > 0 {
				log.Error("Stderr: ", stderrBuf.String())
			}
		}()

		cmdOut, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		scanOut := bufio.NewScanner(cmdOut)
		scanOut.Buffer(nil, 16*1024)

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

		for scanOut.Scan() {
			if errOut := worker.ScanOutput(ctx, inst, log, scanOut.Bytes()); errOut != nil {
				log.Error(errOut)
			}
		}

		if err = cmd.Wait(); err != nil {
			err = wrapErr(ctx, err)
			log.Errorf("cmd failed: %s", err)
		}

		return worker.Error(inst, err)
	}
}

func addExecWorker(workerType string, cfg *jobs.WorkerConfig, createWorker func() execWorker) {
	workerFunc := makeExecWorkerFunc()

	workerStart := func(ctx *jobs.WorkerContext) (*jobs.WorkerContext, error) {
		return ctx.WithCookie(createWorker()), nil
	}

	workerCommit := func(ctx *jobs.WorkerContext, errjob error) error {
		if w, ok := ctx.Cookie().(execWorker); ok {
			return w.Commit(ctx, errjob)
		}
		return nil
	}

	cfg = cfg.Clone()
	cfg.WorkerType = workerType
	cfg.WorkerStart = workerStart
	cfg.WorkerFunc = workerFunc
	cfg.WorkerCommit = workerCommit
	jobs.AddWorker(cfg)
}

func wrapErr(ctx context.Context, err error) error {
	if ctx.Err() == context.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	return err
}
