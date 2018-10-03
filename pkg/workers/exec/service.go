package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/sirupsen/logrus"
)

// ServiceOptions contains the options to execute a service.
type ServiceOptions struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	File string `json:"service_file"`

	Message *ServiceOptions `json:"message"`
}

type serviceWorker struct {
	man  *apps.WebappManifest
	slug string
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
	name := opts.Name

	man, err := apps.GetWebappBySlugAndUpdate(i, slug,
		i.AppsCopier(apps.Webapp), i.Registries())
	if err != nil {
		if err == apps.ErrNotFound {
			err = jobs.ErrBadTrigger{Err: err}
		}
		return
	}
	if man.State() != apps.Ready {
		err = errors.New("Application is not ready")
		return
	}

	var service *apps.Service
	var ok bool
	if name != "" {
		service, ok = man.Services[name]
	} else {
		for _, s := range man.Services {
			if s.File == opts.File {
				service, ok = s, true
				break
			}
		}
	}
	if !ok {
		err = jobs.ErrBadTrigger{Err: fmt.Errorf("Service %q was not found", name)}
		return
	}
	if triggerID, ok := ctx.TriggerID(); ok && service.TriggerID != "" {
		if triggerID != service.TriggerID {
			err = jobs.ErrBadTrigger{Err: fmt.Errorf("Trigger %q is orphan", triggerID)}
			return
		}
	}

	w.man = man
	w.slug = slug

	osFS := afero.NewOsFs()
	workDir, err = afero.TempDir(osFS, "", "service-"+slug)
	if err != nil {
		return
	}
	workFS := afero.NewBasePathFs(osFS, workDir)

	fs := i.AppsFileServer()
	src, err := fs.Open(man.Slug(), man.Version(), path.Join("/", service.File))
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
	return w.slug
}

func (w *serviceWorker) PrepareCmdEnv(ctx *jobs.WorkerContext, i *instance.Instance) (cmd string, env []string, err error) {
	token := i.BuildAppToken(w.man, "")
	cmd = config.GetConfig().Konnectors.Cmd
	env = []string{
		"COZY_URL=" + i.PageURL("/", nil),
		"COZY_CREDENTIALS=" + token,
		"COZY_LANGUAGE=node", // default to node language for services
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
