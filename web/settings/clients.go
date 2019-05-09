package settings

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
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

	clients, err := oauth.GetAll(instance, false)
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

	var claims permission.Claims
	err := crypto.ParseJWT(tok, func(token *jwt.Token) (interface{}, error) {
		return instance.PickKey(token.Claims.(*permission.Claims).Audience)
	}, &claims)
	if err != nil {
		return permission.ErrInvalidToken
	}

	// check if the claim is valid
	if claims.Issuer != instance.Domain {
		return permission.ErrInvalidToken
	}
	if claims.Expired() {
		return permission.ErrExpiredToken
	}
	if claims.Audience != consts.AccessTokenAudience {
		return permission.ErrInvalidToken
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
