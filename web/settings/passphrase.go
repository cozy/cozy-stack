// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents.
package settings

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func registerPassphrase(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	accept := c.Request().Header.Get("Accept")
	acceptHTML := strings.Contains(accept, echo.MIMETextHTML)

	args := struct {
		Register   string `json:"register_token" form:"register_token"`
		Passphrase string `json:"passphrase" form:"passphrase"`
	}{}
	if err := c.Bind(&args); err != nil {
		return err
	}

	registerToken, err := hex.DecodeString(args.Register)
	if err != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	passphrase := []byte(args.Passphrase)
	if err = lifecycle.RegisterPassphrase(inst, passphrase, registerToken); err != nil {
		return jsonapi.BadRequest(err)
	}

	longRunSession := true
	sessionID, err := auth.SetCookieForNewSession(c, longRunSession)
	if err != nil {
		return err
	}
	if err := session.StoreNewLoginEntry(inst, sessionID, "", c.Request(), false); err != nil {
		inst.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
	}

	if acceptHTML {
		return finishOnboarding(c)
	}
	return c.NoContent(http.StatusNoContent)
}

func updatePassphrase(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	session, hasSession := middlewares.GetSession(c)

	// Even if the current passphrase is needed for this request to work, we
	// enforce a valid permission to avoid having an unauthorized enpoint that
	// can be bruteforced.
	if err := middlewares.AllowWholeType(c, permission.PUT, consts.Settings); err != nil {
		return err
	}

	args := struct {
		Current           string `json:"current_passphrase"`
		Passphrase        string `json:"new_passphrase"`
		TwoFactorPasscode string `json:"two_factor_passcode"`
		TwoFactorToken    []byte `json:"two_factor_token"`
		Force             bool   `json:"force,omitempty"`
	}{}
	err := c.Bind(&args)
	if err != nil {
		return jsonapi.BadRequest(err)
	}

	// If we want to force the update
	if args.Force {
		p, err := middlewares.GetPermission(c)
		if err != nil {
			return err
		}

		if p.Type == permission.TypeCLI { // We limit the authorization only to CLI
			err := lifecycle.ForceUpdatePassphrase(inst, []byte(args.Passphrase))
			if err != nil {
				return err
			}
		} else {
			err = fmt.Errorf("You must have a CLI audience to force change the password")
			return jsonapi.BadRequest(err)
		}
		return c.NoContent(http.StatusNoContent)
	}

	// Else, we keep going on the standard checks (2FA, current passphrase, ...)
	newPassphrase := []byte(args.Passphrase)
	currentPassphrase := []byte(args.Current)

	if inst.HasAuthMode(instance.TwoFactorMail) && len(args.TwoFactorToken) == 0 {
		if lifecycle.CheckPassphrase(inst, currentPassphrase) == nil {
			var twoFactorToken []byte
			twoFactorToken, err = lifecycle.SendTwoFactorPasscode(inst)
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, echo.Map{
				"two_factor_token": twoFactorToken,
			})
		}
		return instance.ErrInvalidPassphrase
	}

	err = lifecycle.UpdatePassphrase(inst, newPassphrase, currentPassphrase,
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
