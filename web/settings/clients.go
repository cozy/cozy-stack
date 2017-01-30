package settings

import (
	"errors"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

func listClients(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	if err := permissions.AllowWholeType(c, permissions.GET, consts.OAuthClients); err != nil {
		return err
	}

	clients, err := oauth.GetAll(instance)
	if err != nil {
		return err
	}

	objs := make([]jsonapi.Object, len(clients))
	for i, d := range clients {
		objs[i] = jsonapi.Object(d)
	}
	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

func revokeClient(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	if err := permissions.AllowWholeType(c, permissions.DELETE, consts.OAuthClients); err != nil {
		return err
	}

	client, err := oauth.FindClient(instance, c.Param("id"))
	if err != nil {
		return err
	}

	if err := client.Delete(instance); err != nil {
		return errors.New(err.Error)
	}
	return c.NoContent(http.StatusNoContent)
}
