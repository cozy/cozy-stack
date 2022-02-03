package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

func registerClient(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	err := limits.CheckRateLimit(instance, limits.OAuthClientType)
	if limits.IsLimitReachedOrExceeded(err) {
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
		if err != nil || perm.Type != permission.TypeCLI {
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
	err := limits.CheckRateLimit(instance, limits.OAuthClientType)
	if limits.IsLimitReachedOrExceeded(err) {
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
	instance := middlewares.GetInstance(c)
	client, err := oauth.FindClient(instance, c.Param("client-id"))
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.NoContent(http.StatusNoContent)
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	if err := checkClientToken(c, client); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": err.Error(),
		})
	}
	if err := client.Delete(instance); err != nil {
		return c.JSON(err.Code, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func postChallenge(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	err := limits.CheckRateLimit(inst, limits.OAuthClientType)
	if limits.IsLimitReachedOrExceeded(err) {
		return echo.NewHTTPError(http.StatusNotFound, "Not found")
	}
	client := c.Get("client").(*oauth.Client)
	nonce, err := client.CreateChallenge(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, echo.Map{"nonce": nonce})
}

func postAttestation(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	client, err := oauth.FindClient(inst, c.Param("client-id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": "Client not found",
		})
	}
	var data oauth.AttestationRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}
	if err := client.Attest(inst, data); err != nil {
		inst.Logger().Infof("Cannot attest %s client: %s", client.ID(), err.Error())
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func confirmFlagship(c echo.Context) error {
	// TODO rate limiting
	inst := middlewares.GetInstance(c)
	client, err := oauth.FindClient(inst, c.Param("client-id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": "Client not found",
		})
	}

	clientID := c.Param("client-id")
	code := c.FormValue("code")
	token := []byte(c.FormValue("token"))
	if ok := oauth.CheckFlagshipCode(inst, clientID, token, code); !ok {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": inst.Translate("Confirm Flagship Invalid code"),
		})
	}

	oldClient := client.Clone().(*oauth.Client)
	client.Flagship = true
	if err := client.Update(inst, oldClient); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error,
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func checkRegistrationToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		client, err := oauth.FindClient(instance, c.Param("client-id"))
		if err != nil {
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "Client not found",
			})
		}
		if err := checkClientToken(c, client); err != nil {
			return c.JSON(http.StatusUnauthorized, echo.Map{
				"error": err.Error(),
			})
		}
		c.Set("client", client)
		return next(c)
	}
}

func checkClientToken(c echo.Context, client *oauth.Client) error {
	header := c.Request().Header.Get(echo.HeaderAuthorization)
	if !strings.HasPrefix(header, "Bearer ") {
		return errors.New("invalid_token")
	}
	token := header[len("Bearer "):]
	instance := middlewares.GetInstance(c)
	_, ok := client.ValidToken(instance, consts.RegistrationTokenAudience, token)
	if !ok {
		return errors.New("invalid_token")
	}
	return nil
}
