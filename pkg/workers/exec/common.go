package exec

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
)

func init() {
	addExecWorker("konnector", &jobs.WorkerConfig{
		Concurrency:  runtime.NumCPU() * 2,
		MaxExecCount: 2,
		MaxExecTime:  200 * time.Second,
		Timeout:      200 * time.Second,
	}, func() execWorker {
		return &konnectorWorker{}
	})

	addExecWorker("service", &jobs.WorkerConfig{
		Concurrency:  runtime.NumCPU() * 2,
		MaxExecCount: 2,
		MaxExecTime:  200 * time.Second,
		Timeout:      200 * time.Second,
	}, func() execWorker {
		return &serviceWorker{}
	})
}

type execWorker interface {
	PrepareWorkDir(ctx *jobs.WorkerContext, i *instance.Instance) (string, error)
	PrepareCmdEnv(ctx *jobs.WorkerContext, i *instance.Instance) (cmd string, env []string, jobID string, err error)
	ScanOuput(ctx *jobs.WorkerContext, i *instance.Instance, line []byte) error
	Error(i *instance.Instance, err error) error
	Commit(ctx *jobs.WorkerContext, errjob error) error
}

func makeExecWorkerFunc() jobs.WorkerFunc {
	return func(ctx *jobs.WorkerContext) error {
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

		cmdStr, env, jobID, err := worker.PrepareCmdEnv(ctx, inst)
		if err != nil {
			return err
		}

		cmd := exec.CommandContext(ctx, cmdStr, workDir) // #nosec
		cmd.Env = env

		cmdErr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}
		cmdOut, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}

		scanErr := bufio.NewScanner(cmdErr)
		scanOut := bufio.NewScanner(cmdOut)
		scanOut.Buffer(nil, 256*1024)

		if err = cmd.Start(); err != nil {
			return wrapErr(ctx, err)
		}

		go func() {
			for scanErr.Scan() {
				ctx.Logger().Errorf("%s: Stderr: %s", jobID, scanErr.Text())
			}
		}()

		for scanOut.Scan() {
			if errOut := worker.ScanOuput(ctx, inst, scanOut.Bytes()); errOut != nil {
				ctx.Logger().Errorf("%s: %s", jobID, errOut)
			}
		}

		if err = cmd.Wait(); err != nil {
			err = wrapErr(ctx, err)
			ctx.Logger().Errorf("%s: failed: %s", jobID, err)
		}

		return worker.Error(inst, err)
	}
}

func addExecWorker(name string, cfg *jobs.WorkerConfig, createWorker func() execWorker) {
	workerFunc := makeExecWorkerFunc()

	workerInit := func(ctx *jobs.WorkerContext) (*jobs.WorkerContext, error) {
		return ctx.WithCookie(createWorker()), nil
	}

	workerCommit := func(ctx *jobs.WorkerContext, errjob error) error {
		if w, ok := ctx.Cookie().(execWorker); ok {
			return w.Commit(ctx, errjob)
		}
		return nil
	}

	cfg = cfg.Clone()
	cfg.WorkerInit = workerInit
	cfg.WorkerFunc = workerFunc
	cfg.WorkerCommit = workerCommit

	jobs.AddWorker(name, cfg)
}

func wrapErr(ctx context.Context, err error) error {
	if ctx.Err() == context.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	return err
}
