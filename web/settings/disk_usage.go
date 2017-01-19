// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents. For example, it has a route for getting a CSS
// with some CSS variables that can be used as a theme.
package settings

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

type apiDiskUsage struct {
	Used int64 `json:"used,string"`
}

func (j *apiDiskUsage) ID() string                             { return consts.DiskUsageID }
func (j *apiDiskUsage) Rev() string                            { return "" }
func (j *apiDiskUsage) DocType() string                        { return consts.Settings }
func (j *apiDiskUsage) SetID(_ string)                         {}
func (j *apiDiskUsage) SetRev(_ string)                        {}
func (j *apiDiskUsage) Relationships() jsonapi.RelationshipMap { return nil }
func (j *apiDiskUsage) Included() []jsonapi.Object             { return nil }
func (j *apiDiskUsage) SelfLink() string                       { return "/settings/disk-usage" }

// Settings objects permissions are only on ID
func (j *apiDiskUsage) Valid(k, f string) bool { return false }

func diskUsage(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	used, err := vfs.DiskUsage(instance)
	if err != nil {
		return err
	}

	var result = &apiDiskUsage{used}

	if err = permissions.Allow(c, permissions.GET, result); err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, result, nil)
}
