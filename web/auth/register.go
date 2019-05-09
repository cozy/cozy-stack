package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func registerClient(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if err := limits.CheckRateLimit(instance, limits.OAuthClientType); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Not found")
	}
	client := new(oauth.Client)
	if err := json.NewDecoder(c.Request().Body).Decode(client); err != nil {
		return err
	}
	// We do not allow the creation of clients allowed to have an empty scope
	// ("login" scope), except via the CLI.
	if client.AllowLoginScope {
		perm, err := middlewares.GetPermission(c)
		if err != nil {
			return err
		}
		if perm.Type != permission.TypeCLI {
			return echo.NewHTTPError(http.StatusUnauthorized,
				"Not authorized to create client with given parameters")
		}
	}
	if err := client.Create(instance); err != nil {
		return c.JSON(err.Code, err)
	}
	return c.JSON(http.StatusCreated, client)
}

func readClient(c echo.Context) error {
	client := c.Get("client").(*oauth.Client)
	client.TransformIDAndRev()
	return c.JSON(http.StatusOK, client)
}

func updateClient(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if err := limits.CheckRateLimit(instance, limits.OAuthClientType); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Not found")
	}
	client := new(oauth.Client)
	if err := json.NewDecoder(c.Request().Body).Decode(client); err != nil {
		return err
	}
	oldClient := c.Get("client").(*oauth.Client)
	if err := client.Update(instance, oldClient); err != nil {
		return c.JSON(err.Code, err)
	}
	return c.JSON(http.StatusOK, client)
}

func deleteClient(c echo.Context) error {
	client := c.Get("client").(*oauth.Client)
	instance := middlewares.GetInstance(c)
	if err := client.Delete(instance); err != nil {
		return c.JSON(err.Code, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func checkRegistrationToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		header := c.Request().Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "invalid_token",
			})
		}
		instance := middlewares.GetInstance(c)
		client, err := oauth.FindClient(instance, c.Param("client-id"))
		if err != nil {
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "Client not found",
			})
		}
		token := header[len("Bearer "):]
		_, ok := client.ValidToken(instance, consts.RegistrationTokenAudience, token)
		if !ok {
			return c.JSON(http.StatusUnauthorized, echo.Map{
				"error": "invalid_token",
			})
		}
		c.Set("client", client)
		return next(c)
	}
}
