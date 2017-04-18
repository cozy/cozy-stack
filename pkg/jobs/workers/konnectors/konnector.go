package konnectors

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/spf13/afero"
)

func init() {
	jobs.AddWorker("konnector", &jobs.WorkerConfig{
		Concurrency:  4,
		MaxExecCount: 2,
		Timeout:      30 * time.Second,
		WorkerFunc:   Worker,
	})
}

// KonnectorOptions contains the options to execute a konnector.
type KonnectorOptions struct {
	Slug         string `json:"slug"`
	Account      string `json:"account"`
	FolderToSave string `json:"folder_to_save"`
}

// Worker is the worker that runs a konnector by executing an external process.
func Worker(ctx context.Context, m *jobs.Message) error {
	opts := &KonnectorOptions{}
	if err := m.Unmarshal(&opts); err != nil {
		return err
	}

	slug := opts.Slug
	fields := string(m.Data)
	domain := ctx.Value(jobs.ContextDomainKey).(string)
	worker := ctx.Value(jobs.ContextWorkerKey).(string)
	jobID := fmt.Sprintf("%s/%s/%s", worker, slug, domain)

	inst, err := instance.Get(domain)
	if err != nil {
		return err
	}

	man, err := apps.GetBySlug(inst, slug, apps.Konnector)
	if err != nil {
		return err
	}

	token := inst.BuildKonnectorToken(man)

	osFS := afero.NewOsFs()
	workDir, err := afero.TempDir(osFS, "", "konnector-"+slug)
	if err != nil {
		return err
	}
	defer osFS.RemoveAll(workDir)
	workFS := afero.NewBasePathFs(osFS, workDir)

	fileServer := inst.KonnectorsFileServer()
	tarFile, err := fileServer.Open(slug, man.Version(), apps.KonnectorArchiveName)
	if err != nil {
		return err
	}

	tr := tar.NewReader(tarFile)
	for {
		var hdr *tar.Header
		hdr, err = tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		dirname := path.Dir(hdr.Name)
		if dirname != "." {
			if err = workFS.MkdirAll(dirname, 0755); err != nil {
				return nil
			}
		}
		var f afero.File
		f, err = workFS.OpenFile(hdr.Name, os.O_CREATE|os.O_WRONLY, os.FileMode(hdr.Mode))
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		if err != nil {
			return err
		}
	}

	konnCmd := config.GetConfig().Konnectors.Cmd
	cmd := exec.CommandContext(ctx, konnCmd, workDir) // #nosec
	cmd.Env = []string{
		"COZY_CREDENTIALS=" + token,
		"COZY_FIELDS=" + fields,
		"COZY_DOMAIN=" + domain,
	}

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

	hub := realtime.InstanceHub(domain)

	go doScanOut(jobID, scanOut, hub)
	go doScanErr(jobID, scanErr)

	if err = cmd.Start(); err != nil {
		return wrapErr(ctx, err)
	}
	if err = cmd.Wait(); err != nil {
		return wrapErr(ctx, err)
	}
	return nil
}

func doScanOut(jobID string, scanner *bufio.Scanner, hub realtime.Hub) {
	for scanner.Scan() {
		doc := couchdb.JSONDoc{Type: consts.JobEvents}
		err := json.Unmarshal(scanner.Bytes(), &doc.M)
		if err != nil {
			log.Warnf("[konnector] %s: Could not parse Stdout as JSON: %s", jobID, err)
			continue
		}
		hub.Publish(&realtime.Event{
			Type: realtime.EventCreate,
			Doc:  doc,
		})
	}
	if err := scanner.Err(); err != nil {
		log.Errorf("[konnector] %s: Error while reading stdout: %s", jobID, err)
	}
}

func doScanErr(jobID string, scanner *bufio.Scanner) {
	for scanner.Scan() {
		log.Errorf("[konnector] %s: Stderr: %s", jobID, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Errorf("[konnector] %s: Error while reading stderr: %s", jobID, err)
	}
}

func wrapErr(ctx context.Context, err error) error {
	if ctx.Err() == context.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	return err
}
