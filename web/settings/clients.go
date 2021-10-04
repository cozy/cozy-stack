package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
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

	if err := middlewares.AllowWholeType(c, permission.GET, consts.OAuthClients); err != nil {
		return err
	}

	bookmark := c.QueryParam("page[cursor]")
	limit, err := strconv.ParseInt(c.QueryParam("page[limit]"), 10, 64)
	if err != nil || limit < 0 || limit > consts.MaxItemsPerPageForMango {
		limit = 100
	}
	clients, bookmark, err := oauth.GetAll(instance, int(limit), bookmark)
	if err != nil {
		return err
	}

	objs := make([]jsonapi.Object, len(clients))
	for i, d := range clients {
		objs[i] = jsonapi.Object(&apiOauthClient{d})
	}

	links := &jsonapi.LinksList{}
	if bookmark != "" && len(objs) == int(limit) {
		v := url.Values{}
		v.Set("page[cursor]", bookmark)
		if limit != 100 {
			v.Set("page[limit]", fmt.Sprintf("%d", limit))
		}
		links.Next = "/settings/clients?" + v.Encode()
	}
	return jsonapi.DataList(c, http.StatusOK, objs, links)
}

func revokeClient(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	if err := middlewares.AllowWholeType(c, permission.DELETE, consts.OAuthClients); err != nil {
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

func synchronized(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	tok := middlewares.GetRequestToken(c)
	if tok == "" {
		return permission.ErrInvalidToken
	}

	claims, err := middlewares.ExtractClaims(c, instance, tok)
	if err != nil {
		return err
	}

	client, err := oauth.FindClient(instance, claims.Subject)
	if err != nil {
		return permission.ErrInvalidToken
	}

	client.SynchronizedAt = time.Now()
	if err := couchdb.UpdateDoc(instance, client); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
