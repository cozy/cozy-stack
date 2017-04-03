package konnector

import (
	"context"
	"net/url"
	"os/exec"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
)

func init() {
	jobs.AddWorker("konnector", &jobs.WorkerConfig{
		Concurrency:  4,
		MaxExecCount: 2,
		Timeout:      30 * time.Second,
		WorkerFunc:   Worker,
	})
}

// Options contains the option
type Options struct {
	Name string `json:"name"`
}

// Worker is the worker that runs a konnector by executing an external process.
func Worker(ctx context.Context, m *jobs.Message) error {
	opts := &Options{}
	if err := m.Unmarshal(&opts); err != nil {
		return err
	}

	credentials := ""
	cozyURL := url.URL{
		Scheme: "https",
		Host:   ctx.Value(jobs.ContextDomainKey).(string),
	}

	cmd := exec.CommandContext(ctx,
		config.GetConfig().Konnectors.Cmd, opts.Name,
	)
	cmd.Env = []string{
		"CREDENTIALS=" + credentials,
		"COZY_URL=" + cozyURL.String(),
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return context.DeadlineExceeded
		}
		return err
	}
	return nil
}
