// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents. For example, it has a route for getting a CSS
// with some CSS variables that can be used as a theme.
package settings

import (
	"encoding/hex"
	"net/http"

	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

func registerPassphrase(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	args := &struct {
		Register   string `json:"register_token"`
		Passphrase string `json:"passphrase"`
	}{}
	if err := c.Bind(&args); err != nil {
		return err
	}

	registerToken, err := hex.DecodeString(args.Register)
	if err != nil {
		return jsonapi.NewError(http.StatusBadRequest, err)
	}

	passphrase := []byte(args.Passphrase)
	if err := instance.RegisterPassphrase(passphrase, registerToken); err != nil {
		return jsonapi.BadRequest(err)
	}

	if _, err := auth.SetCookieForNewSession(c); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func updatePassphrase(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	args := &struct {
		Current    string `json:"current_passphrase"`
		Passphrase string `json:"new_passphrase"`
	}{}
	if err := c.Bind(&args); err != nil {
		return err
	}

	newPassphrase := []byte(args.Passphrase)
	currentPassphrase := []byte(args.Current)
	if err := instance.UpdatePassphrase(newPassphrase, currentPassphrase); err != nil {
		return jsonapi.BadRequest(err)
	}

	if _, err := auth.SetCookieForNewSession(c); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
