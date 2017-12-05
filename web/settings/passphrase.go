// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents.
package settings

import (
	"encoding/hex"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
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
	if err = instance.RegisterPassphrase(passphrase, registerToken); err != nil {
		return jsonapi.BadRequest(err)
	}

	sessionID, err := auth.SetCookieForNewSession(c)
	if err != nil {
		return err
	}
	if err := sessions.StoreNewLoginEntry(instance, sessionID, "", c.Request(), false); err != nil {
		instance.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
	}

	return c.NoContent(http.StatusNoContent)
}

func updatePassphrase(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	// Even if the current passphrase is needed for this request to work, we
	// enforce a valid permission to avoid having an unauthorized enpoint that
	// can be bruteforced.
	if err := permissions.AllowWholeType(c, permissions.GET, consts.Settings); err != nil {
		return err
	}

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
