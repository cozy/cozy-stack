// Package archive is for the archive worker, that can zip and unzip files.
package archive

import (
	"archive/zip"
	"io"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
)

type zipMessage struct {
	Files    map[string]interface{} `json:"files"` // path -> fileID | {id,page}
	DirID    string                 `json:"dir_id"`
	Filename string                 `json:"filename"`
}

// WorkerZip is a worker that creates zip archives.
func WorkerZip(ctx *job.TaskContext) error {
	msg := &zipMessage{}
	if err := ctx.UnmarshalMessage(msg); err != nil {
		return err
	}
	fs := ctx.Instance.VFS()
	return createZip(fs, msg.Files, msg.DirID, msg.Filename)
}

func createZip(fs vfs.VFS, files map[string]interface{}, dirID, filename string) error {
	now := time.Now()
	zipDoc, err := vfs.NewFileDoc(filename, dirID, -1, nil, "application/zip", "zip", now, false, false, false, nil)
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
	for filePath, target := range files {
		var fileID string
		var page int
		switch target := target.(type) {
		case string:
			fileID = target
		case map[string]interface{}:
			fileID, _ = target["id"].(string)
			page64, _ := target["page"].(float64)
			page = int(page64)
		}
		err = addFileToZip(fs, w, fileID, filePath, page)
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

func addFileToZip(fs vfs.VFS, w *zip.Writer, fileID, filePath string, page int) error {
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

	if page <= 0 {
		_, err = io.Copy(f, fr)
		return err
	}
	extracted, err := config.PDF().ExtractPage(fr, page)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, extracted)
	return err
}
