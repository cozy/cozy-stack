package office

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
)

// Status list is described on https://api.onlyoffice.com/editors/callback#status
const (
	// StatusReadyForSaving is used when the file should be saved after being
	// edited.
	StatusReadyForSaving = 2
)

// CallbackParameters is a struct for the parameters sent by the document
// server to the stack.
// Cf https://api.onlyoffice.com/editors/callback
type CallbackParameters struct {
	Key    string `json:"key"`
	Status int    `json:"status"`
	URL    string `json:"url"`
}

var docserverClient = &http.Client{
	Timeout: 60 * time.Second,
}

// Callback will manage the callback from the document server.
func Callback(inst *instance.Instance, params CallbackParameters) error {
	parts := strings.SplitN(params.Key, "-", 2)
	if len(parts) != 2 {
		return errors.New("Invalid key")
	}
	id := parts[0]
	rev := parts[1]

	switch params.Status {
	case StatusReadyForSaving:
		return saveFile(inst, id, rev, params.URL)
	default:
		return nil
	}
}

func saveFile(inst *instance.Instance, id, rev, downloadURL string) error {
	fs := inst.VFS()
	file, err := fs.FileByID(id)
	if err != nil {
		return err
	}
	if !isOfficeDocument(file) {
		return ErrInvalidFile
	}

	res, err := docserverClient.Get(downloadURL)
	if err != nil {
		return err
	}
	defer func() {
		// Flush the body in case of error to allow reusing the connection with
		// Keep-Alive
		_, _ = io.Copy(ioutil.Discard, res.Body)
		_ = res.Body.Close()
	}()

	newfile := file.Clone().(*vfs.FileDoc)
	newfile.MD5Sum = nil // Let the VFS compute the new md5sum
	newfile.ByteSize = res.ContentLength
	newfile.UpdatedAt = time.Now()
	newfile.CozyMetadata.UpdatedAt = file.UpdatedAt

	if file.Rev() != rev {
		// Conflict: save it in a new file
		file = nil
		newfile.SetID("")
		newfile.SetRev("")
		newfile.DocName = fmt.Sprintf("%s - conflict - %d", newfile.DocName, time.Now().Unix())
		newfile.ResetFullpath()
		_, _ = newfile.Path(inst.VFS()) // Prefill the fullpath
	}

	f, err := fs.CreateFile(newfile, file)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, res.Body)
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}
