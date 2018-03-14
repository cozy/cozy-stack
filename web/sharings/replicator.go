package sharings

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	perm "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

// RevsDiff is part of the replicator
func RevsDiff(c echo.Context) error {
	return c.String(http.StatusOK, "OK")
}

// replicatorRoutes sets the routing for the replicator
func replicatorRoutes(router *echo.Group) {
	group := router.Group("", checkSharingPermissions)
	group.POST("/:sharing-id/revs_diff", RevsDiff, checkSharingPermissions)
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
		return nil
	}
}
