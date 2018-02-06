package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/sirupsen/logrus"
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

func (w *serviceWorker) PrepareWorkDir(ctx *jobs.WorkerContext, i *instance.Instance) (workDir string, err error) {
	opts := &ServiceOptions{}
	if err = ctx.UnmarshalMessage(&opts); err != nil {
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

func (w *serviceWorker) Slug() string {
	if w.opts != nil {
		return w.opts.Slug
	}
	return ""
}

func (w *serviceWorker) PrepareCmdEnv(ctx *jobs.WorkerContext, i *instance.Instance) (cmd string, env []string, err error) {
	token := i.BuildAppToken(w.man, "")
	cmd = config.GetConfig().Konnectors.Cmd
	env = []string{
		"COZY_URL=" + i.PageURL("/", nil),
		"COZY_CREDENTIALS=" + token,
		"COZY_TYPE=" + w.opts.Type,
		"COZY_LOCALE=" + i.Locale,
		"COZY_TIME_LIMIT=" + ctxToTimeLimit(ctx),
		"COZY_JOB_ID=" + ctx.ID(),
	}
	return
}

func (w *serviceWorker) Logger(ctx *jobs.WorkerContext) *logrus.Entry {
	return ctx.Logger().WithField("slug", w.Slug())
}

func (w *serviceWorker) ScanOutput(ctx *jobs.WorkerContext, i *instance.Instance, line []byte) error {
	var msg struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return fmt.Errorf("Could not parse stdout as JSON: %q", string(line))
	}
	log := w.Logger(ctx)
	switch msg.Type {
	case konnectorMsgTypeDebug, konnectorMsgTypeInfo:
		log.Debug(msg.Message)
	case konnectorMsgTypeWarning, "warn":
		log.Warn(msg.Message)
	case konnectorMsgTypeError:
		log.Error(msg.Message)
	case konnectorMsgTypeCritical:
		log.Error(msg.Message)
	}
	return nil
}

func (w *serviceWorker) Error(i *instance.Instance, err error) error {
	return err
}

func (w *serviceWorker) Commit(ctx *jobs.WorkerContext, errjob error) error {
	return nil
}
