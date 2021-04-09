package office

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"

	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

// Status list is described on https://api.onlyoffice.com/editors/callback#status
const (
	// StatusReadyForSaving is used when the file should be saved after being
	// edited.
	StatusReadyForSaving = 2
	// StatusForceSaveRequested is used when the file has been modified and
	// should be saved, even if the document is still opened and can be edited
	// by users.
	StatusForceSaveRequested = 6
)

// CallbackParameters is a struct for the parameters sent by the document
// server to the stack.
// Cf https://api.onlyoffice.com/editors/callback
type CallbackParameters struct {
	Key    string `json:"key"`
	Status int    `json:"status"`
	URL    string `json:"url"`
	Token  string `json:"-"` // From the Authorization header
}

var docserverClient = &http.Client{
	Timeout: 60 * time.Second,
}

type callbackClaims struct {
	Payload struct {
		Key    string `json:"key"`
		Status int    `json:"status"`
		URL    string `json:"url"`
	} `json:"payload"`
}

// Valid is part of the jwt.Claims interface
func (c *callbackClaims) Valid() error { return nil }

// Callback will manage the callback from the document server.
func Callback(inst *instance.Instance, params CallbackParameters) error {
	cfg := getConfig(inst.ContextName)
	if err := checkToken(cfg, params); err != nil {
		return err
	}

	switch params.Status {
	case StatusReadyForSaving:
		return finalSaveFile(inst, params.Key, params.URL)
	case StatusForceSaveRequested:
		return forceSaveFile(inst, params.Key, params.URL)
	default:
		return nil
	}
}

func checkToken(cfg *config.Office, params CallbackParameters) error {
	if cfg == nil || cfg.OutboxSecret == "" {
		return nil
	}

	var claims callbackClaims
	err := crypto.ParseJWT(params.Token, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, permission.ErrInvalidToken
		}
		return []byte(cfg.OutboxSecret), nil
	}, &claims)
	if err != nil {
		return permission.ErrInvalidToken
	}
	if params.URL != claims.Payload.URL || params.Key != claims.Payload.Key || params.Status != claims.Payload.Status {
		return permission.ErrInvalidToken
	}
	return nil
}

func finalSaveFile(inst *instance.Instance, key, downloadURL string) error {
	id, rev, err := GetStore().GetDoc(inst, key)
	if err != nil || id == "" || rev == "" {
		return errors.New("Invalid key")
	}

	_, err = saveFile(inst, id, rev, downloadURL)
	if err == nil {
		_ = GetStore().RemoveDoc(inst, key)
	}
	return err
}

func forceSaveFile(inst *instance.Instance, key, downloadURL string) error {
	id, rev, err := GetStore().GetDoc(inst, key)
	if err != nil || id == "" || rev == "" {
		return errors.New("Invalid key")
	}

	rev, err = saveFile(inst, id, rev, downloadURL)
	if err == nil {
		_ = GetStore().UpdateDoc(inst, key, id, rev)
	}
	return err
}

// saveFile saves the file with content from the given URL and returns the new revision.
func saveFile(inst *instance.Instance, id, rev, downloadURL string) (string, error) {
	fs := inst.VFS()
	file, err := fs.FileByID(id)
	if err != nil {
		return "", err
	}
	if !isOfficeDocument(file) {
		return "", ErrInvalidFile
	}

	res, err := docserverClient.Get(downloadURL)
	if err != nil {
		return "", err
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
	if newfile.CozyMetadata == nil {
		newfile.CozyMetadata = vfs.NewCozyMetadata(inst.PageURL("/", nil))
	}
	newfile.UpdatedAt = time.Now()
	newfile.CozyMetadata.UpdatedAt = newfile.UpdatedAt

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
		return "", err
	}
	_, err = io.Copy(f, res.Body)
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return newfile.Rev(), err
}
