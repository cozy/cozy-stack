package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/utils"
)

type unzipMessage struct {
	Zip         string `json:"zip"`
	Destination string `json:"destination"`
}

// WorkerUnzip is a worker that unzip a file.
func WorkerUnzip(ctx *job.WorkerContext) error {
	msg := &unzipMessage{}
	if err := ctx.UnmarshalMessage(msg); err != nil {
		return err
	}
	fs := ctx.Instance.VFS()
	return unzip(fs, msg.Zip, msg.Destination)
}

func unzip(fs vfs.VFS, zipID, destination string) error {
	zipDoc, err := fs.FileByID(zipID)
	if err != nil {
		return err
	}
	dstDoc, err := fs.DirByID(destination)
	if err != nil {
		return err
	}

	fr, err := fs.OpenFile(zipDoc)
	if err != nil {
		return err
	}
	defer fr.Close()
	r, err := zip.NewReader(fr, zipDoc.ByteSize)
	if err != nil {
		return err
	}

	dirs := make(map[string]*vfs.DirDoc)
	for _, f := range r.File {
		f.Name = utils.CleanUTF8(f.Name)
		name := path.Base(f.Name)
		dirname := path.Dir(f.Name)
		dir := dstDoc
		if dirname != "." {
			var ok bool
			dirname = path.Join(dstDoc.Fullpath, dirname)
			if dir, ok = dirs[dirname]; !ok {
				dir, err = vfs.MkdirAll(fs, dirname)
				if err != nil {
					if couchdb.IsConflictError(err) {
						dirname = fmt.Sprintf("%s - conflict - %d", dirname, time.Now().Unix())
						dir, err = vfs.MkdirAll(fs, dirname)
					}
					if err != nil {
						return err
					}
				}
				dirs[dirname] = dir
			}
		}

		if f.Mode().IsDir() {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		size := int64(f.UncompressedSize64)
		mime, class := vfs.ExtractMimeAndClassFromFilename(f.Name)
		mod := f.Modified
		doc, err := vfs.NewFileDoc(name, dir.ID(), size, nil, mime, class, mod, false, false, nil)
		if err != nil {
			return err
		}
		doc.CozyMetadata = vfs.NewCozyMetadata("")
		at := doc.CozyMetadata.CreatedAt
		doc.CozyMetadata.UploadedAt = &at
		file, err := fs.CreateFile(doc, nil)
		if err != nil {
			if couchdb.IsConflictError(err) {
				doc.DocName = fmt.Sprintf("%s - conflict - %d", doc.DocName, time.Now().Unix())
				file, err = fs.CreateFile(doc, nil)
			}
			if err != nil {
				return err
			}
		}
		_, err = io.Copy(file, rc)
		cerr := file.Close()
		if err != nil {
			return err
		}
		if cerr != nil {
			return cerr
		}
	}
	return nil
}
