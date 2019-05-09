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
	"github.com/cozy/echo"
)

type apiDiskUsage struct {
	Used  int64 `json:"used,string"`
	Quota int64 `json:"quota,string,omitempty"`
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
func (j *apiDiskUsage) Match(k, f string) bool { return false }

func diskUsage(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	var result apiDiskUsage

	// Check permissions, but also allow every request from the logged-in user
	// as this route is used by the cozy-bar from all the client-side apps
	if err := middlewares.Allow(c, permission.GET, &result); err != nil {
		if !middlewares.IsLoggedIn(c) {
			return err
		}
	}

	fs := instance.VFS()
	used, err := fs.DiskUsage()
	if err != nil {
		return err
	}

	quota := fs.DiskQuota()

	result.Used = used
	result.Quota = quota
	return jsonapi.Data(c, http.StatusOK, &result, nil)
}
