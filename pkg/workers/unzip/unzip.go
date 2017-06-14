package unzip

import (
	"archive/zip"
	"context"
	"io"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

type zipMessage struct {
	Zip         string `json:"zip"`
	Destination string `string:"destination"`
}

func init() {
	jobs.AddWorker("unzip", &jobs.WorkerConfig{
		Concurrency:  (runtime.NumCPU() + 1) / 2,
		MaxExecCount: 2,
		Timeout:      30 * time.Second,
		WorkerFunc:   Worker,
	})
}

// Worker is a worker that unzip a file.
func Worker(ctx context.Context, m *jobs.Message) error {
	msg := &zipMessage{}
	if err := m.Unmarshal(msg); err != nil {
		return err
	}
	domain := ctx.Value(jobs.ContextDomainKey).(string)
	log := logger.WithDomain(domain)
	log.Infof("[jobs] unzip %s in %s", msg.Zip, msg.Destination)
	i, err := instance.Get(domain)
	if err != nil {
		return err
	}
	fs := i.VFS()

	zipDoc, err := fs.FileByID(msg.Zip)
	if err != nil {
		return err
	}
	dstDoc, err := fs.DirByID(msg.Destination)
	if err != nil {
		return err
	}

	fr, err := i.VFS().OpenFile(zipDoc)
	if err != nil {
		return err
	}
	defer fr.Close()
	r, err := zip.NewReader(fr, zipDoc.ByteSize)
	if err != nil {
		return err
	}
	for _, f := range r.File {
		// TODO check if f.Name has slashes
		rc, err := f.Open()
		if err != nil {
			return err
		}

		size := int64(f.UncompressedSize64)
		mime, class := vfs.ExtractMimeAndClassFromFilename(f.Name)
		now := time.Now()
		doc, err := vfs.NewFileDoc(f.Name, dstDoc.ID(), size, nil, mime, class, now, false, false, nil)
		if err != nil {
			return err
		}
		file, err := fs.CreateFile(doc, nil)
		if err != nil {
			// TODO what about conflict?
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, rc)
		if err != nil {
			return err
		}
	}
	return nil
}
