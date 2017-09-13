package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/spf13/afero"
)

// ServiceOptions contains the options to execute a service.
type ServiceOptions struct {
	Slug        string          `json:"slug"`
	Type        string          `json:"type"`
	ServiceFile string          `json:"service_file"`
	Message     *ServiceOptions `json:"message"`
}

type serviceWorker struct {
	opts *ServiceOptions
	man  *apps.WebappManifest
}

func (w *serviceWorker) PrepareWorkDir(i *instance.Instance, m *jobs.Message) (workDir string, err error) {
	opts := &ServiceOptions{}
	if err = m.Unmarshal(&opts); err != nil {
		return
	}
	if opts.Message != nil {
		opts = opts.Message
	}

	slug := opts.Slug

	man, err := apps.GetWebappBySlug(i, slug)
	if err != nil {
		return
	}
	if man.State() != apps.Ready {
		err = errors.New("Application is not ready")
		return
	}

	w.opts = opts
	w.man = man

	osFS := afero.NewOsFs()
	workDir, err = afero.TempDir(osFS, "", "service-"+slug)
	if err != nil {
		return
	}
	workFS := afero.NewBasePathFs(osFS, workDir)

	fs := i.AppsFileServer()
	src, err := fs.Open(man.Slug(), man.Version(), path.Join("/", opts.ServiceFile))
	if err != nil {
		return
	}
	defer src.Close()

	dst, err := workFS.OpenFile("index.js", os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return
	}

	return workDir, nil
}

func (w *serviceWorker) PrepareCmdEnv(i *instance.Instance, m *jobs.Message) (cmd string, env []string, jobID string, err error) {
	jobID = fmt.Sprintf("service/%s/%s", w.opts.Slug, i.Domain)

	token := i.BuildAppToken(w.man)

	cmd = config.GetConfig().Konnectors.Cmd
	env = []string{
		"COZY_URL=" + i.PageURL("/", nil),
		"COZY_CREDENTIALS=" + token,
		"COZY_TYPE=" + w.opts.Type,
		"COZY_LOCALE=" + i.Locale,
		"COZY_JOB_ID=" + jobID,
	}
	return
}

func (w *serviceWorker) ScanOuput(i *instance.Instance, line []byte) error {
	return nil
}

func (w *serviceWorker) Error(i *instance.Instance, err error) error {
	return err
}

func (w *serviceWorker) Commit(ctx context.Context, msg *jobs.Message, errjob error) error {
	return nil
}
