// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents.
package settings

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type apiDiskUsage struct {
	Used     int64  `json:"used,string"`
	Quota    int64  `json:"quota,string,omitempty"`
	Files    int64  `json:"files,string"`
	Trash    *int64 `json:"trash,string,omitempty"`
	Versions int64  `json:"versions,string"`
}

func (j *apiDiskUsage) ID() string                             { return consts.DiskUsageID }
func (j *apiDiskUsage) Rev() string                            { return "" }
func (j *apiDiskUsage) DocType() string                        { return consts.Settings }
func (j *apiDiskUsage) Clone() couchdb.Doc                     { return j }
func (j *apiDiskUsage) SetID(_ string)                         {}
func (j *apiDiskUsage) SetRev(_ string)                        {}
func (j *apiDiskUsage) Relationships() jsonapi.RelationshipMap { return nil }
func (j *apiDiskUsage) Included() []jsonapi.Object             { return nil }
func (j *apiDiskUsage) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/settings/disk-usage"}
}

// Settings objects permissions are only on ID
func (j *apiDiskUsage) Fetch(field string) []string { return nil }

// checkAccessToDiskUsage validates the access control for the disk-usage. It
// checks if there is an explicit permission on this document, but also allow
// every request from the logged-in user as this route is used by the cozy-bar
// from all the client-side apps. And there is a third case where it is
// allowed: when an anonymous user comes from a shared by link directory with
// write access.
func checkAccessToDiskUsage(c echo.Context, result *apiDiskUsage) error {
	if err := middlewares.Allow(c, permission.GET, result); err == nil {
		return nil
	}
	if middlewares.IsLoggedIn(c) && middlewares.HasWebAppToken(c) {
		return nil
	}
	return middlewares.CanWriteToAnyDirectory(c)
}

func diskUsage(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	var result apiDiskUsage

	if err := checkAccessToDiskUsage(c, &result); err != nil {
		return err
	}

	fs := instance.VFS()
	if c.QueryParam("include") == "trash" {
		if trash, err := fs.TrashUsage(); err == nil {
			result.Trash = &trash
		}
	}

	versions, err := fs.VersionsUsage()
	if err != nil {
		return err
	}

	files, err := fs.FilesUsage()
	if err != nil {
		return err
	}

	used := files + versions
	quota := fs.DiskQuota()

	result.Used = used
	result.Quota = quota
	result.Files = files
	result.Versions = versions
	return jsonapi.Data(c, http.StatusOK, &result, nil)
}
