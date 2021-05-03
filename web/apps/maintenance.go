package apps

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/labstack/echo/v4"
)

func activateMaintenance(c echo.Context) error {
	slug := c.Param("slug")
	var doc couchdb.JSONDoc
	if _, err := jsonapi.Bind(c.Request().Body, &doc); err != nil {
		return err
	}
	if err := app.ActivateMaintenance(slug, doc.M); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func deactivateMaintenance(c echo.Context) error {
	slug := c.Param("slug")
	if err := app.DeactivateMaintenance(slug); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// AdminRoutes sets the routing for the admin interface to configure
// maintenance for the konnectors.
func AdminRoutes(router *echo.Group) {
	router.PUT("/maintenance/:slug", activateMaintenance)
	router.DELETE("/maintenance/:slug", deactivateMaintenance)
}
