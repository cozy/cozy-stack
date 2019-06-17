package archive

import (
	"archive/zip"
	"io"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
)

type zipMessage struct {
	Files    map[string]string `json:"files"` // path -> fileID
	DirID    string            `json:"dir_id"`
	Filename string            `json:"filename"`
}

// WorkerZip is a worker that creates zip archives.
func WorkerZip(ctx *job.WorkerContext) error {
	msg := &zipMessage{}
	if err := ctx.UnmarshalMessage(msg); err != nil {
		return err
	}
	fs := ctx.Instance.VFS()
	return createZip(fs, msg.Files, msg.DirID, msg.Filename)
}

func createZip(fs vfs.VFS, files map[string]string, dirID, filename string) error {
	now := time.Now()
	zipDoc, err := vfs.NewFileDoc(filename, dirID, -1, nil, "application/zip", "zip", now, false, false, nil)
	if err != nil {
		return err
	}
	zipDoc.CozyMetadata = vfs.NewCozyMetadata("")
	zipDoc.CozyMetadata.UploadedAt = &now
	z, err := fs.CreateFile(zipDoc, nil)
	if err != nil {
		return err
	}
	w := zip.NewWriter(z)
	for filePath, fileID := range files {
		err = addFileToZip(fs, w, fileID, filePath)
		if err != nil {
			break
		}
	}
	werr := w.Close()
	zerr := z.Close()
	if err != nil {
		return err
	}
	if werr != nil {
		return werr
	}
	return zerr
}

func addFileToZip(fs vfs.VFS, w *zip.Writer, fileID, filePath string) error {
	file, err := fs.FileByID(fileID)
	if err != nil {
		return err
	}
	fr, err := fs.OpenFile(file)
	if err != nil {
		return err
	}
	defer fr.Close()
	header := &zip.FileHeader{
		Name:     filePath,
		Method:   zip.Deflate,
		Modified: file.UpdatedAt,
	}
	if file.Executable {
		header.SetMode(0750)
	} else {
		header.SetMode(0640)
	}
	f, err := w.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, fr)
	return err
}
