// Package nextcloud is a client library for NextCloud. It only supports files
// via Webdav for the moment.
package nextcloud

import (
	"encoding/json"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/safehttp"
	"github.com/cozy/cozy-stack/pkg/webdav"
)

type File struct {
	DocID     string `json:"id,omitempty"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Size      uint64 `json:"size,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Class     string `json:"class,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	ETag      string `json:"etag,omitempty"`
}

func (f *File) ID() string                             { return f.DocID }
func (f *File) Rev() string                            { return "" }
func (f *File) DocType() string                        { return consts.NextCloudFiles }
func (f *File) SetID(id string)                        { f.DocID = id }
func (f *File) SetRev(id string)                       {}
func (f *File) Clone() couchdb.Doc                     { panic("nextcloud.File should not be cloned") }
func (f *File) Included() []jsonapi.Object             { return nil }
func (f *File) Relationships() jsonapi.RelationshipMap { return nil }
func (f *File) Links() *jsonapi.LinksList              { return nil }

var _ jsonapi.Object = (*File)(nil)

type NextCloud struct {
	inst      *instance.Instance
	accountID string
	webdav    *webdav.Client
}

func New(inst *instance.Instance, accountID string) (*NextCloud, error) {
	var doc couchdb.JSONDoc
	err := couchdb.GetDoc(inst, consts.Accounts, accountID, &doc)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}
	account.Decrypt(doc)

	if doc.M == nil || doc.M["account_type"] != "nextcloud" {
		return nil, ErrInvalidAccount
	}
	auth, ok := doc.M["auth"].(map[string]interface{})
	if !ok {
		return nil, ErrInvalidAccount
	}
	ncURL, _ := auth["url"].(string)
	if ncURL == "" {
		return nil, ErrInvalidAccount
	}
	u, err := url.Parse(ncURL)
	if err != nil {
		return nil, ErrInvalidAccount
	}
	username, _ := auth["login"].(string)
	password, _ := auth["password"].(string)
	logger := inst.Logger().WithNamespace("nextcloud")
	webdav := &webdav.Client{
		Scheme:   u.Scheme,
		Host:     u.Host,
		Username: username,
		Password: password,
		Logger:   logger,
	}
	nc := &NextCloud{
		inst:      inst,
		accountID: accountID,
		webdav:    webdav,
	}
	if err := nc.fillBasePath(&doc); err != nil {
		return nil, err
	}
	return nc, nil
}

func (nc *NextCloud) Download(path string) (*webdav.Download, error) {
	return nc.webdav.Get(path)
}

func (nc *NextCloud) Mkdir(path string) error {
	return nc.webdav.Mkcol(path)
}

func (nc *NextCloud) Delete(path string) error {
	return nc.webdav.Delete(path)
}

func (nc *NextCloud) ListFiles(path string) ([]jsonapi.Object, error) {
	items, err := nc.webdav.List(path)
	if err != nil {
		return nil, err
	}

	var files []jsonapi.Object
	for _, item := range items {
		var mime, class string
		if item.Type == "file" {
			mime, class = vfs.ExtractMimeAndClass(item.ContentType)
		}
		file := &File{
			DocID:     item.ID,
			Type:      item.Type,
			Name:      item.Name,
			Size:      item.Size,
			Mime:      mime,
			Class:     class,
			UpdatedAt: item.LastModified,
			ETag:      item.ETag,
		}
		files = append(files, file)
	}
	return files, nil
}

func (nc *NextCloud) fillBasePath(accountDoc *couchdb.JSONDoc) error {
	userID, _ := accountDoc.M["webdav_user_id"].(string)
	if userID != "" {
		nc.webdav.BasePath = "/remote.php/dav/files/" + userID
		return nil
	}

	userID, err := nc.fetchUserID()
	if err != nil {
		return err
	}
	nc.webdav.BasePath = "/remote.php/dav/files/" + userID

	// Try to persist the userID to avoid fetching it for every WebDAV request
	accountDoc.M["webdav_user_id"] = userID
	accountDoc.Type = consts.Accounts
	account.Encrypt(*accountDoc)
	_ = couchdb.UpdateDoc(nc.inst, accountDoc)
	return nil
}

// https://docs.nextcloud.com/server/latest/developer_manual/client_apis/OCS/ocs-status-api.html#fetch-your-own-status
func (nc *NextCloud) fetchUserID() (string, error) {
	logger := nc.webdav.Logger
	u := url.URL{
		Scheme: nc.webdav.Scheme,
		Host:   nc.webdav.Host,
		User:   url.UserPassword(nc.webdav.Username, nc.webdav.Password),
		Path:   "/ocs/v2.php/apps/user_status/api/v1/user_status",
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "cozy-stack "+build.Version+" ("+runtime.Version()+")")
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Accept", "application/json")
	start := time.Now()
	res, err := safehttp.ClientWithKeepAlive.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		logger.Warnf("user_status %s: %s (%s)", u.Host, err, elapsed)
		return "", err
	}
	defer res.Body.Close()
	logger.Infof("user_status %s: %d (%s)", u.Host, res.StatusCode, elapsed)
	if res.StatusCode != 200 {
		return "", webdav.ErrInvalidAuth
	}
	var payload OCSPayload
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		logger.Warnf("cannot fetch NextCloud userID: %s", err)
		return "", err
	}
	return payload.OCS.Data.UserID, nil
}

type OCSPayload struct {
	OCS struct {
		Data struct {
			UserID string `json:"userId"`
		} `json:"data"`
	} `json:"ocs"`
}
