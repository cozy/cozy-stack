// Package nextcloud is a client library for NextCloud. It only supports files
// via Webdav for the moment.
package nextcloud

import (
	"encoding/json"
	"net/http"
	"net/url"
	"runtime"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/safehttp"
	"github.com/cozy/cozy-stack/pkg/webdav"
)

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

func (nc *NextCloud) Mkdir(path string) error {
	return nc.webdav.Mkcol(path)
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
	account.Encrypt(*accountDoc)
	_ = couchdb.UpdateDoc(nc.inst, accountDoc)
	return nil
}

// https://docs.nextcloud.com/server/latest/developer_manual/client_apis/OCS/ocs-status-api.html#fetch-your-own-status
func (nc *NextCloud) fetchUserID() (string, error) {
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
	res, err := safehttp.ClientWithKeepAlive.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		nc.webdav.Logger.Warnf("cannot fetch NextCloud userID: %d", res.StatusCode)
		return "", webdav.ErrInvalidAuth
	}
	var payload OCSPayload
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		nc.webdav.Logger.Warnf("cannot fetch NextCloud userID: %s", err)
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
