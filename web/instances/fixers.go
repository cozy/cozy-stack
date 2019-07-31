package instances

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/labstack/echo/v4"
)

type mismatchStruct struct {
	SizeIndex int64 `json:"size_index"`
	SizeFile  int64 `json:"size_file"`
}

// resEntry contains an out entry of a 64k content mismatch
type resEntry struct {
	FilePath  string `json:"filepath"`
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type resStruct struct {
	DryRun  bool       `json:"dry_run"`
	Updated []resEntry `json:"updated"`
	Removed []resEntry `json:"removed"`
	Domain  string     `json:"domain"`
}

// contentMismatchFixer fixes the 64k bug
func contentMismatchFixer(c echo.Context) error {
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

	// Get the FSCK data from the instance
	buf, err := getFSCK(inst)
	if err != nil {
		return err
	}

	var content map[string]interface{}
	res := &resStruct{
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

		// Prepare the struct & ensure the data should be fixed
		contentMismatch, err := prepareMismatchStruct(content)
		if err != nil {
			return err
		}
		if !is64ContentMismatch(contentMismatch) {
			continue
		}

		// Finally, fixing the file
		err = fixFile(content, contentMismatch, inst, res, body.DryRun)
		if err != nil {
			return err
		}
	}

	return c.JSON(http.StatusOK, res)
}

func getFSCK(inst *instance.Instance) (io.Reader, error) {
	buf := new(bytes.Buffer)
	encoder := json.NewEncoder(buf)

	logCh := make(chan *vfs.FsckLog)
	go func() {
		fs := inst.VFS()
		_ = fs.Fsck(func(log *vfs.FsckLog) { logCh <- log })
		close(logCh)
	}()

	for log := range logCh {
		if !log.IsFile && !log.IsVersion && log.DirDoc != nil {
			log.DirDoc.DirsChildren = nil
			log.DirDoc.FilesChildren = nil
		}
		if errenc := encoder.Encode(log); errenc != nil {
			return nil, errenc
		}
	}

	return buf, nil
}

func prepareMismatchStruct(content map[string]interface{}) (*mismatchStruct, error) {
	contentMismatch := &mismatchStruct{}
	marshaled, _ := json.Marshal(content["content_mismatch"])
	if err := json.Unmarshal(marshaled, &contentMismatch); err != nil {
		return nil, err
	}

	return contentMismatch, nil
}

// is64ContentMismatch ensures we are treating a 64k content mismatch
func is64ContentMismatch(contentMismatch *mismatchStruct) bool {
	// SizeFile should be a multiple of 64k shorter than SizeIndex
	size := int64(64 * 1024)

	isSmallFile := contentMismatch.SizeIndex <= size && contentMismatch.SizeFile == 0
	isMultiple64 := (contentMismatch.SizeIndex-contentMismatch.SizeFile)%size == 0

	return isMultiple64 || isSmallFile
}

// fixFile fixes a content-mismatch file
// Trashed:
// - Removes it if the file
// Not Trashed:
// - Appending a corrupted suffix to the file
// - Force the file index size to the real file size
func fixFile(content map[string]interface{}, contentMismatch *mismatchStruct, inst *instance.Instance, res *resStruct, dryRun bool) error {
	corruptedSuffix := "-corrupted"

	// Removes/update
	fileDoc := content["file_doc"].(map[string]interface{})

	doc := &vfs.FileDoc{}
	err := couchdb.GetDoc(inst, consts.Files, fileDoc["_id"].(string), doc)
	if err != nil {
		return err
	}
	instanceVFS := inst.VFS()

	// File is trashed
	if fileDoc["restore_path"] != nil {
		// This is a trashed file, just delete it
		res.Removed = append(res.Removed, resEntry{
			ID:        fileDoc["_id"].(string),
			FilePath:  fileDoc["path"].(string),
			CreatedAt: doc.CreatedAt.String(),
			UpdatedAt: doc.UpdatedAt.String(),
		})

		if !dryRun {
			return instanceVFS.DestroyFile(doc)
		}
		return nil
	}

	// File is not trashed, updating it
	newFileDoc := doc.Clone().(*vfs.FileDoc)

	newFileDoc.DocName = doc.DocName + corruptedSuffix
	newFileDoc.ByteSize = contentMismatch.SizeFile

	res.Updated = append(res.Updated, resEntry{
		ID:        fileDoc["_id"].(string),
		FilePath:  fileDoc["path"].(string),
		CreatedAt: doc.CreatedAt.String(),
		UpdatedAt: doc.UpdatedAt.String(),
	})
	if !dryRun {
		// Let the UpdateFileDoc handles the file doc update. For swift
		// layout V1, the file should also be renamed
		return instanceVFS.UpdateFileDoc(doc, newFileDoc)
	}

	return nil
}
