package sharings

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/sharing"
	"github.com/cozy/cozy-stack/web/middlewares"
	perm "github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/echo"
)

// RevsDiff is part of the replicator
func RevsDiff(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Debugf("Sharing was not found: %s", err)
		return wrapErrors(err)
	}
	var changes sharing.Changes
	if err = c.Bind(&changes); err != nil {
		inst.Logger().WithField("nspace", "replicator").Debugf("Changes cannot be bound: %s", err)
		return wrapErrors(err)
	}
	if changes == nil {
		inst.Logger().WithField("nspace", "replicator").Debugf("No changes")
		return echo.NewHTTPError(http.StatusBadRequest)
	}
	missings, err := s.ComputeRevsDiff(inst, changes)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Debugf("Error on compute: %s", err)
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
		inst.Logger().WithField("nspace", "replicator").Debugf("Sharing was not found: %s", err)
		return wrapErrors(err)
	}
	var docs sharing.DocsByDoctype
	if err = c.Bind(&docs); err != nil {
		inst.Logger().WithField("nspace", "replicator").Debugf("Docs cannot be bound: %s", err)
		return wrapErrors(err)
	}
	if docs == nil {
		inst.Logger().WithField("nspace", "replicator").Debugf("No bulk docs")
		return echo.NewHTTPError(http.StatusBadRequest)
	}
	err = s.ApplyBulkDocs(inst, docs)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Debugf("Error on apply: %s", err)
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
			middlewares.GetInstance(c).Logger().WithField("nspace", "replicator").Debugf("[sharing] Invalid permission: %s", err)
			return err
		}
		if !requestPerm.Permissions.AllowID("GET", consts.Sharings, sharingID) {
			middlewares.GetInstance(c).Logger().WithField("nspace", "replicator").Debugf("[sharing] Not allowed (%s)", sharingID)
			return echo.NewHTTPError(http.StatusForbidden)
		}
		return next(c)
	}
}
