package workers

import (
	"context"
	"encoding/json"
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
		WorkerFunc:   KonnectorWorker,
	})
}

// KonnectorOptions contains the options to execute a konnector.
type KonnectorOptions struct {
	Slug   string          `json:"slug"`
	Fields json.RawMessage `json:"fields"`
}

// KonnectorWorker is the worker that runs a konnector by executing an external process.
func KonnectorWorker(ctx context.Context, m *jobs.Message) error {
	opts := &KonnectorOptions{}
	if err := m.Unmarshal(&opts); err != nil {
		return err
	}

	credentials := ""
	fields := string(opts.Fields)
	domain := ctx.Value(jobs.ContextDomainKey).(string)
	cozyURL := url.URL{
		Scheme: "https",
		Host:   domain,
	}

	konnCmd := config.GetConfig().Konnectors.Cmd
	cmd := exec.CommandContext(ctx, konnCmd, opts.Slug) // #nosec
	cmd.Env = []string{
		"COZY_CREDENTIALS=" + credentials,
		"COZY_FIELDS=" + fields,
		"COZY_DOMAIN=" + domain,
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
