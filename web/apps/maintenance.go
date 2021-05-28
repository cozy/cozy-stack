package apps

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/labstack/echo/v4"
)

type apiMaintenance struct {
	couchdb.JSONDoc
}

// Links is part of the Manifest interface
func (man *apiMaintenance) Links() *jsonapi.LinksList { return nil }

// Relationships is part of the Manifest interface
func (man *apiMaintenance) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the Manifest interface
func (man *apiMaintenance) Included() []jsonapi.Object { return nil }

// apiMaintenance is a jsonapi.Object
var _ jsonapi.Object = (*apiMaintenance)(nil)

func listMaintenance(c echo.Context) error {
	list, err := app.ListMaintenance()
	if err != nil {
		return err
	}
	objs := make([]jsonapi.Object, len(list))
	for i, item := range list {
		doc := couchdb.JSONDoc{
			Type: consts.KonnectorsMaintenance,
			M:    item,
		}
		objs[i] = &apiMaintenance{doc}
	}
	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

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
	router.GET("/maintenance", listMaintenance)
	router.PUT("/maintenance/:slug", activateMaintenance)
	router.DELETE("/maintenance/:slug", deactivateMaintenance)
}
