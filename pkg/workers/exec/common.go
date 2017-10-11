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
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/sirupsen/logrus"
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
	PrepareWorkDir(i *instance.Instance, m jobs.Message) (string, error)
	PrepareCmdEnv(i *instance.Instance, m jobs.Message) (cmd string, env []string, jobID string, err error)
	ScanOuput(i *instance.Instance, log *logrus.Entry, line []byte) error
	Error(i *instance.Instance, err error) error
	Commit(ctx context.Context, msg jobs.Message, errjob error) error
}

func makeExecWorkerFunc() jobs.WorkerThreadedFunc {
	return func(ctx context.Context, cookie interface{}, m jobs.Message) error {
		worker := cookie.(execWorker)

		domain := ctx.Value(jobs.ContextDomainKey).(string)
		workerName := ctx.Value(jobs.ContextWorkerKey).(string)

		inst, err := instance.Get(domain)
		if err != nil {
			return err
		}

		workDir, err := worker.PrepareWorkDir(inst, m)
		if err != nil {
			return err
		}
		defer os.RemoveAll(workDir)

		cmdStr, env, jobID, err := worker.PrepareCmdEnv(inst, m)
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

		log := logger.WithDomain(domain)
		log = log.WithField("type", "konnector")
		log = log.WithField("job_id", jobID)

		if err = cmd.Start(); err != nil {
			return wrapErr(ctx, err)
		}

		go func() {
			for scanErr.Scan() {
				log.Errorf("[%s] %s: Stderr: %s", workerName, jobID, scanErr.Text())
			}
		}()

		for scanOut.Scan() {
			if errOut := worker.ScanOuput(inst, log, scanOut.Bytes()); errOut != nil {
				log.Errorf("[%s] %s: %s", workerName, jobID, errOut)
			}
		}

		if err = cmd.Wait(); err != nil {
			err = wrapErr(ctx, err)
			log.Errorf("[%s] %s: failed: %s", workerName, jobID, err)
		}

		return worker.Error(inst, err)
	}
}

func addExecWorker(name string, cfg *jobs.WorkerConfig, createWorker func() execWorker) {
	workerFunc := makeExecWorkerFunc()

	workerInit := func() (interface{}, error) {
		return createWorker(), nil
	}

	workerCommit := func(ctx context.Context, cookie interface{}, msg jobs.Message, errjob error) error {
		if w, ok := cookie.(execWorker); ok {
			return w.Commit(ctx, msg, errjob)
		}
		return nil
	}

	cfg = cfg.Clone()
	cfg.WorkerInit = workerInit
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
