package settings

import (
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
