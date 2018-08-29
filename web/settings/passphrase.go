// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents.
package settings

import (
	"encoding/hex"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/echo"
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
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	passphrase := []byte(args.Passphrase)
	if err = instance.RegisterPassphrase(passphrase, registerToken); err != nil {
		return jsonapi.BadRequest(err)
	}

	longRunSession := true
	sessionID, err := auth.SetCookieForNewSession(c, longRunSession)
	if err != nil {
		return err
	}
	if err := sessions.StoreNewLoginEntry(instance, sessionID, "", c.Request(), false); err != nil {
		instance.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
	}

	return c.NoContent(http.StatusNoContent)
}

func updatePassphrase(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	session, hasSession := middlewares.GetSession(c)

	// Even if the current passphrase is needed for this request to work, we
	// enforce a valid permission to avoid having an unauthorized enpoint that
	// can be bruteforced.
	if err := middlewares.AllowWholeType(c, permissions.PUT, consts.Settings); err != nil {
		return err
	}

	args := struct {
		Current           string `json:"current_passphrase"`
		Passphrase        string `json:"new_passphrase"`
		TwoFactorPasscode string `json:"two_factor_passcode"`
		TwoFactorToken    []byte `json:"two_factor_token"`
	}{}
	err := c.Bind(&args)
	if err != nil {
		return jsonapi.BadRequest(err)
	}

	newPassphrase := []byte(args.Passphrase)
	currentPassphrase := []byte(args.Current)

	if inst.HasAuthMode(instance.TwoFactorMail) && len(args.TwoFactorToken) == 0 {
		if inst.CheckPassphrase(currentPassphrase) == nil {
			var twoFactorToken []byte
			twoFactorToken, err = inst.SendTwoFactorPasscode()
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, echo.Map{
				"two_factor_token": twoFactorToken,
			})
		}
		return instance.ErrInvalidPassphrase
	}

	err = inst.UpdatePassphrase(newPassphrase, currentPassphrase,
		args.TwoFactorPasscode, args.TwoFactorToken)
	if err != nil {
		return jsonapi.BadRequest(err)
	}

	longRunSession := true
	if hasSession {
		longRunSession = session.LongRun
	}
	if _, err = auth.SetCookieForNewSession(c, longRunSession); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
