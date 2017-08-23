package exec

import (
	"bufio"
	"context"
	"os/exec"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
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
}

type execWorker interface {
	PrepareWorkDir(i *instance.Instance, m *jobs.Message) (string, error)
	PrepareCmdEnv(i *instance.Instance, m *jobs.Message) (cmd string, env []string, jobID string, err error)
	ScanOuput(i *instance.Instance, line []byte) error
	Error(i *instance.Instance, err error) error
	Commit(ctx context.Context, msg *jobs.Message, errjob error) error
}

func makeExecWorkerFunc(createWorker func() execWorker) jobs.WorkerThreadedFunc {
	return func(ctx context.Context, m *jobs.Message) (context.Context, error) {
		worker := createWorker()

		ctx = context.WithValue(ctx, jobs.ContextExecWorkerKey, worker)
		domain := ctx.Value(jobs.ContextDomainKey).(string)
		workerName := ctx.Value(jobs.ContextWorkerKey).(string)

		inst, err := instance.Get(domain)
		if err != nil {
			return ctx, err
		}

		workDir, err := worker.PrepareWorkDir(inst, m)
		if err != nil {
			return ctx, err
		}

		cmdStr, env, jobID, err := worker.PrepareCmdEnv(inst, m)
		if err != nil {
			return ctx, err
		}

		cmd := exec.CommandContext(ctx, cmdStr, workDir) // #nosec
		cmd.Env = env

		cmdErr, err := cmd.StderrPipe()
		if err != nil {
			return ctx, err
		}
		cmdOut, err := cmd.StdoutPipe()
		if err != nil {
			return ctx, err
		}

		scanErr := bufio.NewScanner(cmdErr)
		scanOut := bufio.NewScanner(cmdOut)
		scanOut.Buffer(nil, 256*1024)

		log := logger.WithDomain(domain)

		if err = cmd.Start(); err != nil {
			return ctx, wrapErr(ctx, err)
		}

		go func() {
			for scanErr.Scan() {
				log.Errorf("[%s] %s: Stderr: %s", workerName, jobID, scanErr.Text())
			}
		}()

		for scanOut.Scan() {
			if errOut := worker.ScanOuput(inst, scanOut.Bytes()); errOut != nil {
				log.Errorf("[%s] %s: %s", workerName, jobID, errOut)
			}
		}

		if err = cmd.Wait(); err != nil {
			err = wrapErr(ctx, err)
			log.Errorf("[%s] %s: failed: %s", workerName, jobID, err)
		}

		return ctx, worker.Error(inst, err)
	}
}

func addExecWorker(name string, cfg *jobs.WorkerConfig, createWorker func() execWorker) {
	workerFunc := makeExecWorkerFunc(createWorker)
	workerCommit := func(ctx context.Context, msg *jobs.Message, errjob error) error {
		worker := ctx.Value(jobs.ContextExecWorkerKey)
		if w, ok := worker.(execWorker); ok {
			return w.Commit(ctx, msg, errjob)
		}
		return errjob
	}

	cfg = cfg.Clone()
	cfg.WorkerThreadedFunc = workerFunc
	cfg.WorkerCommit = workerCommit

	jobs.AddWorker(name, cfg)
}

func wrapErr(ctx context.Context, err error) error {
	if ctx.Err() == context.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	return err
}
