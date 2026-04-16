// Package nextcloud is a client library for NextCloud. It only supports files
// via Webdav for the moment.
package nextcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/safehttp"
	"github.com/cozy/cozy-stack/pkg/webdav"
	"github.com/labstack/echo/v4"
)

type OperationKind int

const (
	MoveOperation OperationKind = iota
	CopyOperation
)

type File struct {
	DocID       string `json:"id,omitempty"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Size        uint64 `json:"size,omitempty"`
	Mime        string `json:"mime,omitempty"`
	Class       string `json:"class,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	ETag        string `json:"etag,omitempty"`
	RestorePath string `json:"restore_path,omitempty"`
	url         string
}

func (f *File) ID() string                             { return f.DocID }
func (f *File) Rev() string                            { return "" }
func (f *File) DocType() string                        { return consts.NextCloudFiles }
func (f *File) SetID(id string)                        { f.DocID = id }
func (f *File) SetRev(id string)                       {}
func (f *File) Clone() couchdb.Doc                     { panic("nextcloud.File should not be cloned") }
func (f *File) Included() []jsonapi.Object             { return nil }
func (f *File) Relationships() jsonapi.RelationshipMap { return nil }
func (f *File) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{
		Self: f.url,
	}
}

var _ jsonapi.Object = (*File)(nil)

type NextCloud struct {
	inst        *instance.Instance
	accountID   string
	userID      string
	installRoot string
	webdav      *webdav.Client
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
	installRoot := normalizeInstallRoot(u.Path)
	logger := inst.Logger().WithNamespace("nextcloud")
	webdav := &webdav.Client{
		Scheme:   u.Scheme,
		Host:     u.Host,
		Username: username,
		Password: password,
		BasePath: installRoot + "/remote.php/dav",
		Logger:   logger,
	}
	nc := &NextCloud{
		inst:        inst,
		accountID:   accountID,
		installRoot: installRoot,
		webdav:      webdav,
	}
	if err := nc.fillUserID(&doc); err != nil {
		return nil, err
	}
	return nc, nil
}

func (nc *NextCloud) Download(path string) (*webdav.Download, error) {
	return nc.webdav.Get("/files/" + nc.userID + "/" + path)
}

// Size returns the recursive byte total of the resource at path, as
// reported by Nextcloud's cached `oc:size` property. Works on the account
// root (pass an empty string or "/") and on any sub-folder. Equivalent to
// a single Depth:0 PROPFIND on the server, not a tree walk, so the cost
// is constant regardless of how many files the folder contains.
func (nc *NextCloud) Size(path string) (uint64, error) {
	return nc.webdav.Size("/files/" + nc.userID + "/" + path)
}

func (nc *NextCloud) Upload(path, mime string, contentLength int64, body io.Reader) error {
	headers := map[string]string{
		echo.HeaderContentType: mime,
	}
	path = "/files/" + nc.userID + "/" + path
	return nc.webdav.Put(path, contentLength, headers, body)
}

func (nc *NextCloud) Mkdir(path string) error {
	return nc.webdav.Mkcol("/files/" + nc.userID + "/" + path)
}

func (nc *NextCloud) Delete(path string) error {
	return nc.webdav.Delete("/files/" + nc.userID + "/" + path)
}

func (nc *NextCloud) Move(oldPath, newPath string) error {
	oldPath = "/files/" + nc.userID + "/" + oldPath
	newPath = "/files/" + nc.userID + "/" + newPath
	return nc.webdav.Move(oldPath, newPath)
}

func (nc *NextCloud) Copy(oldPath, newPath string) error {
	oldPath = "/files/" + nc.userID + "/" + oldPath
	newPath = "/files/" + nc.userID + "/" + newPath
	return nc.webdav.Copy(oldPath, newPath)
}

func (nc *NextCloud) Restore(path string) error {
	path = "/trashbin/" + nc.userID + "/" + path
	dst := "/trashbin/" + nc.userID + "/restore/" + filepath.Base(path)
	return nc.webdav.Move(path, dst)
}

func (nc *NextCloud) DeleteTrash(path string) error {
	return nc.webdav.Delete("/trashbin/" + nc.userID + "/" + path)
}

func (nc *NextCloud) EmptyTrash() error {
	return nc.webdav.Delete("/trashbin/" + nc.userID + "/trash")
}

func (nc *NextCloud) ListFiles(path string) ([]jsonapi.Object, error) {
	items, err := nc.webdav.List("/files/" + nc.userID + "/" + path)
	if err != nil {
		return nil, err
	}

	var files []jsonapi.Object
	for _, item := range items {
		var mime, class string
		if item.Type == "file" {
			mime, class = vfs.ExtractMimeAndClassFromFilename(item.Name)
		}
		file := &File{
			DocID:     item.ID,
			Type:      item.Type,
			Name:      item.Name,
			Path:      "/" + filepath.Join(path, filepath.Base(item.Href)),
			Size:      item.Size,
			Mime:      mime,
			Class:     class,
			UpdatedAt: item.LastModified,
			ETag:      item.ETag,
			url:       nc.buildURL(item, path),
		}
		files = append(files, file)
	}
	return files, nil
}

func (nc *NextCloud) ListTrashed(path string) ([]jsonapi.Object, error) {
	path = "/trash/" + path
	items, err := nc.webdav.List("/trashbin/" + nc.userID + path)
	if err != nil {
		return nil, err
	}

	var files []jsonapi.Object
	for _, item := range items {
		var mime, class string
		if item.Type == "file" {
			mime, class = vfs.ExtractMimeAndClassFromFilename(item.TrashedName)
		}
		file := &File{
			DocID:       item.ID,
			Type:        item.Type,
			Name:        item.TrashedName,
			Path:        filepath.Join(path, filepath.Base(item.Href)),
			Size:        item.Size,
			Mime:        mime,
			Class:       class,
			UpdatedAt:   item.LastModified,
			ETag:        item.ETag,
			RestorePath: item.RestorePath,
			url:         nc.buildTrashedURL(item, path),
		}
		files = append(files, file)
	}
	return files, nil
}

func (nc *NextCloud) Downstream(path, dirID string, kind OperationKind, cozyMetadata *vfs.FilesCozyMetadata, failOnConflict bool) (*vfs.FileDoc, error) {
	path = "/files/" + nc.userID + "/" + path
	dl, err := nc.webdav.Get(path)
	if err != nil {
		return nil, err
	}
	defer dl.Content.Close()

	size, _ := strconv.Atoi(dl.Length)
	mime, class := vfs.ExtractMimeAndClass(dl.Mime)
	doc, err := vfs.NewFileDoc(
		filepath.Base(path),
		dirID,
		int64(size),
		nil, // md5sum
		mime,
		class,
		time.Now(),
		false, // executable
		false, // trashed
		false, // encrypted
		nil,   // tags
	)
	if err != nil {
		return nil, err
	}
	doc.CozyMetadata = cozyMetadata

	fs := nc.inst.VFS()
	exists, err := fs.GetIndexer().DirChildExists(doc.DirID, doc.DocName)
	if err != nil {
		return nil, err
	}
	if exists {
		if failOnConflict {
			return nil, vfs.ErrConflict
		}
		doc.DocName = vfs.ConflictName(fs, doc.DirID, doc.DocName, true)
	}

	file, err := fs.CreateFile(doc, nil)
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(file, dl.Content)
	if cerr := file.Close(); err == nil && cerr != nil {
		return nil, cerr
	}
	if err != nil {
		return nil, err
	}

	if kind == MoveOperation {
		_ = nc.webdav.Delete(path)
	}
	return doc, nil
}

func (nc *NextCloud) Upstream(path, from string, kind OperationKind) error {
	path = "/files/" + nc.userID + "/" + path
	fs := nc.inst.VFS()
	doc, err := fs.FileByID(from)
	if err != nil {
		return err
	}
	f, err := fs.OpenFile(doc)
	if err != nil {
		return err
	}
	defer f.Close()

	headers := map[string]string{
		echo.HeaderContentType: doc.Mime,
	}
	if err := nc.webdav.Put(path, doc.ByteSize, headers, f); err != nil {
		return err
	}
	if kind == MoveOperation {
		_ = fs.DestroyFile(doc)
	}
	return nil
}

func (nc *NextCloud) fillUserID(accountDoc *couchdb.JSONDoc) error {
	userID, _ := accountDoc.M["webdav_user_id"].(string)
	if userID != "" {
		nc.userID = userID
		return nil
	}

	userID, err := nc.fetchUserID()
	if err != nil {
		return err
	}
	nc.userID = userID

	// Try to persist the userID to avoid fetching it for every WebDAV request
	accountDoc.M["webdav_user_id"] = userID
	accountDoc.Type = consts.Accounts
	account.Encrypt(*accountDoc)
	_ = couchdb.UpdateDoc(nc.inst, accountDoc)
	return nil
}

func (nc *NextCloud) buildURL(item webdav.Item, path string) string {
	u := &url.URL{
		Scheme:   nc.webdav.Scheme,
		Host:     nc.webdav.Host,
		Path:     "/apps/files/files/" + item.ID,
		RawQuery: "dir=/" + path,
	}
	if item.Type == "directory" {
		if !strings.HasSuffix(u.RawQuery, "/") {
			u.RawQuery += "/"
		}
		u.RawQuery += item.Name
	}
	return u.String()
}

func (nc *NextCloud) buildTrashedURL(item webdav.Item, path string) string {
	u := &url.URL{
		Scheme:   nc.webdav.Scheme,
		Host:     nc.webdav.Host,
		Path:     "/apps/files/trashbin/" + item.ID,
		RawQuery: "dir=" + strings.TrimPrefix(path, "/trash"),
	}
	if item.Type == "directory" {
		if !strings.HasSuffix(u.RawQuery, "/") {
			u.RawQuery += "/"
		}
		u.RawQuery += item.TrashedName
	}
	return u.String()
}

// FetchUserIDWithCredentials probes the OCS cloud/user endpoint and returns
// the user ID, or webdav.ErrInvalidAuth if the credentials are rejected.
// The logger used for diagnostics is pulled from ctx via logger.FromContext,
// so callers should attach a request-scoped logger with logger.WithContext
// before calling.
//
// https://docs.nextcloud.com/server/latest/developer_manual/client_apis/OCS/ocs-api-overview.html
func FetchUserIDWithCredentials(ctx context.Context, nextcloudURL, username, password string) (string, error) {
	u, err := url.Parse(nextcloudURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", ErrInvalidAccount
	}
	return fetchUserIDFromHost(ctx, u.Scheme, u.Host, normalizeInstallRoot(u.Path), username, password)
}

const probeTimeout = 30 * time.Second

// cloudUserProbePath is OCS Core and cannot be disabled by an admin, unlike
// apps/user_status which some managed Nextcloud providers strip. Probing Core
// avoids misclassifying a stripped-optional-app install as an auth failure.
const cloudUserProbePath = "/ocs/v2.php/cloud/user"

// normalizeInstallRoot strips the trailing slash from a Nextcloud install
// root path so concatenation with absolute API paths (always starting with
// "/") produces clean URLs regardless of whether the caller passed
// https://host/nextcloud or https://host/nextcloud/.
func normalizeInstallRoot(path string) string {
	return strings.TrimSuffix(path, "/")
}

func fetchUserIDFromHost(ctx context.Context, scheme, host, installRoot, username, password string) (string, error) {
	log := logger.FromContext(ctx)
	u := url.URL{
		Scheme: scheme,
		Host:   host,
		User:   url.UserPassword(username, password),
		Path:   installRoot + cloudUserProbePath,
	}
	// Cap the probe so a hung Nextcloud server can't pin a request goroutine —
	// safehttp.ClientWithKeepAlive has handshake timeouts but no overall
	// request deadline.
	reqCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
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
		log.Warnf("cloud/user %s: %s (%s)", u.Host, err, elapsed)
		return "", err
	}
	defer res.Body.Close()
	log.Infof("cloud/user %s: %d (%s)", u.Host, res.StatusCode, elapsed)
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", webdav.ErrInvalidAuth
	default:
		return "", fmt.Errorf("unexpected status %d from nextcloud cloud/user probe", res.StatusCode)
	}
	var payload OCSPayload
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		log.Warnf("cannot fetch NextCloud userID: %s", err)
		return "", err
	}
	return payload.OCS.Data.UserID, nil
}

func (nc *NextCloud) fetchUserID() (string, error) {
	// Receiver predates ctx-threading in this package; mint a local ctx
	// carrying the webdav client's logger so the probe still surfaces
	// under the same diagnostics.
	ctx := logger.WithContext(context.Background(), nc.webdav.Logger)
	return fetchUserIDFromHost(ctx, nc.webdav.Scheme, nc.webdav.Host, nc.installRoot, nc.webdav.Username, nc.webdav.Password)
}

type OCSPayload struct {
	OCS struct {
		Data struct {
			UserID string `json:"id"`
		} `json:"data"`
	} `json:"ocs"`
}
