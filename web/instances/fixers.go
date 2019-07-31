package instances

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/labstack/echo/v4"
)

// contentMismatchFixer fixes the 64k bug
func contentMismatchFixer(c echo.Context) error {
	corruptedSuffix := "-corrupted"

	domain := c.Param("domain")
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return fmt.Errorf("Cannot find instance %s", domain)
	}

	body := struct {
		DryRun bool `json:"dry_run"`
	}{
		DryRun: true,
	}

	// Try to get the dru_run param from the body. If there is no body, ignore
	// it
	_ = json.NewDecoder(c.Request().Body).Decode(&body)

	buf := new(bytes.Buffer)
	encoder := json.NewEncoder(buf)

	logCh := make(chan *vfs.FsckLog)
	go func() {
		fs := inst.VFS()
		err = fs.Fsck(func(log *vfs.FsckLog) { logCh <- log })
		close(logCh)
	}()

	for log := range logCh {
		if !log.IsFile && !log.IsVersion && log.DirDoc != nil {
			log.DirDoc.DirsChildren = nil
			log.DirDoc.FilesChildren = nil
		}
		if errenc := encoder.Encode(log); errenc != nil {
			return errenc
		}
	}

	var content map[string]interface{}

	type resEntry struct {
		FilePath  string `json:"filepath"`
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	resStruct := struct {
		DryRun  bool       `json:"dry_run"`
		Updated []resEntry `json:"updated"`
		Removed []resEntry `json:"removed"`
		Domain  string     `json:"domain"`
	}{
		Domain:  domain,
		DryRun:  body.DryRun,
		Removed: []resEntry{},
		Updated: []resEntry{},
	}

	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		err = json.NewDecoder(bytes.NewReader(scanner.Bytes())).Decode(&content)
		if err != nil {
			return err
		}

		// Filtering the 64kb mismatch issue
		if content["type"] != "content_mismatch" {
			continue
		}

		contentMismatch := struct {
			SizeIndex int64 `json:"size_index"`
			SizeFile  int64 `json:"size_file"`
		}{}
		marshaled, _ := json.Marshal(content["content_mismatch"])
		err = json.Unmarshal(marshaled, &contentMismatch)
		if err != nil {
			return err
		}

		// SizeFile should be a multiple of 64k shorter than SizeIndex
		size := int64(64 * 1024)

		isSmallFile := contentMismatch.SizeIndex <= size && contentMismatch.SizeFile == 0
		isMultiple64 := (contentMismatch.SizeIndex-contentMismatch.SizeFile)%size == 0
		if !isMultiple64 && !isSmallFile {
			continue
		}

		// Removes/update
		fileDoc := content["file_doc"].(map[string]interface{})

		doc := &vfs.FileDoc{}
		err = couchdb.GetDoc(inst, consts.Files, fileDoc["_id"].(string), doc)
		if err != nil {
			return err
		}
		instanceVFS := inst.VFS()

		// Checks if the file is trashed
		if fileDoc["restore_path"] != nil {
			// This is a trashed file, just delete it
			resStruct.Removed = append(resStruct.Removed, resEntry{
				ID:        fileDoc["_id"].(string),
				FilePath:  fileDoc["path"].(string),
				CreatedAt: doc.CreatedAt.String(),
				UpdatedAt: doc.UpdatedAt.String(),
			})
			if !body.DryRun {
				err := instanceVFS.DestroyFile(doc)
				if err != nil {
					return err
				}
			}
			continue
		}

		// Fixing :
		// - Appending a corrupted suffix to the file
		// - Force the file index size to the real file size
		newFileDoc := doc.Clone().(*vfs.FileDoc)

		newFileDoc.DocName = doc.DocName + corruptedSuffix
		newFileDoc.ByteSize = contentMismatch.SizeFile

		resStruct.Updated = append(resStruct.Updated, resEntry{
			ID:        fileDoc["_id"].(string),
			FilePath:  fileDoc["path"].(string),
			CreatedAt: doc.CreatedAt.String(),
			UpdatedAt: doc.UpdatedAt.String(),
		})
		if !body.DryRun {
			// Let the UpdateFileDoc handles the file doc update. For swift
			// layout V1, the file should also be renamed
			err := instanceVFS.UpdateFileDoc(doc, newFileDoc)
			if err != nil {
				return err
			}
		}
	}

	return c.JSON(http.StatusOK, resStruct)
}
