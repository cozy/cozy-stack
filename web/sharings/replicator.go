package sharings

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/sharing"
	"github.com/cozy/cozy-stack/web/middlewares"
	perm "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

// RevsDiff is part of the replicator
func RevsDiff(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	var changes sharing.Changes
	if err = c.Bind(&changes); err != nil {
		return wrapErrors(err)
	}
	if changes == nil {
		return echo.NewHTTPError(http.StatusBadRequest)
	}
	missings, err := s.ComputeRevsDiff(inst, changes)
	if err != nil {
		return wrapErrors(err)
	}
	return c.JSON(http.StatusOK, missings)
}

// BulkDocs is part of the replicator
func BulkDocs(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	var docs sharing.DocsByDoctype
	if err = c.Bind(&docs); err != nil {
		return wrapErrors(err)
	}
	if docs == nil {
		return echo.NewHTTPError(http.StatusBadRequest)
	}
	err = s.ApplyBulkDocs(inst, docs)
	if err != nil {
		return wrapErrors(err)
	}
	return c.JSON(http.StatusOK, []interface{}{})
}

// replicatorRoutes sets the routing for the replicator
func replicatorRoutes(router *echo.Group) {
	group := router.Group("", checkSharingPermissions)
	group.POST("/:sharing-id/_revs_diff", RevsDiff, checkSharingPermissions)
	group.POST("/:sharing-id/_bulk_docs", BulkDocs, checkSharingPermissions)
}

func checkSharingPermissions(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sharingID := c.Param("sharing-id")
		requestPerm, err := perm.GetPermission(c)
		if err != nil {
			return err
		}
		if !requestPerm.Permissions.AllowID("GET", consts.Sharings, sharingID) {
			return echo.NewHTTPError(http.StatusForbidden)
		}
		return next(c)
	}
}
