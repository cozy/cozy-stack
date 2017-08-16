package settings

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

type apiOauthClient struct{ *oauth.Client }

func (c *apiOauthClient) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Client)
}

// Links is used to generate a JSON-API link for the client - see
// jsonapi.Object interface
func (c *apiOauthClient) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/settings/clients/" + c.ID()}
}

// Relationships is used to generate the parent relationship in JSON-API format
// - see jsonapi.Object interface
func (c *apiOauthClient) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the jsonapi.Object interface
func (c *apiOauthClient) Included() []jsonapi.Object {
	return []jsonapi.Object{}
}

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
		objs[i] = jsonapi.Object(&apiOauthClient{d})
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
