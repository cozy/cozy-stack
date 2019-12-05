// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents.
package settings

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type apiPassphraseParameters struct {
	Salt       string `json:"salt"`
	Kdf        int    `json:"kdf"`
	Iterations int    `json:"iterations"`
}

func (p *apiPassphraseParameters) ID() string                             { return consts.PassphraseParametersID }
func (p *apiPassphraseParameters) Rev() string                            { return "" }
func (p *apiPassphraseParameters) DocType() string                        { return consts.Settings }
func (p *apiPassphraseParameters) Clone() couchdb.Doc                     { return p }
func (p *apiPassphraseParameters) SetID(_ string)                         {}
func (p *apiPassphraseParameters) SetRev(_ string)                        {}
func (p *apiPassphraseParameters) Relationships() jsonapi.RelationshipMap { return nil }
func (p *apiPassphraseParameters) Included() []jsonapi.Object             { return nil }
func (p *apiPassphraseParameters) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/settings/passphrase"}
}

func getPassphraseParameters(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.GET, consts.Settings); err != nil {
		return err
	}
	inst := middlewares.GetInstance(c)
	settings, err := settings.Get(inst)
	if err != nil {
		return err
	}
	params := apiPassphraseParameters{
		Salt:       string(inst.PassphraseSalt()),
		Kdf:        settings.PassphraseKdf,
		Iterations: settings.PassphraseKdfIterations,
	}
	return jsonapi.Data(c, http.StatusOK, &params, nil)
}

func registerPassphrase(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	accept := c.Request().Header.Get("Accept")
	acceptHTML := strings.Contains(accept, echo.MIMETextHTML)

	args := struct {
		Register   string `json:"register_token" form:"register_token"`
		Passphrase string `json:"passphrase" form:"passphrase"`
		Hint       string `json:"hint" form:"hint"`
		Key        string `json:"key" form:"key"`
		PublicKey  string `json:"public_key" form:"public_key"`
		PrivateKey string `json:"private_key" form:"private_key"`
		Iterations int    `json:"iterations" form:"iterations"`
	}{}
	if err := c.Bind(&args); err != nil {
		return err
	}

	registerToken, err := hex.DecodeString(args.Register)
	if err != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	if args.Iterations < crypto.MinPBKDF2Iterations && args.Iterations != 0 {
		err := errors.New("The KdfIterations number is too low")
		return jsonapi.InvalidParameter("KdfIterations", err)
	}
	if args.Iterations > crypto.MaxPBKDF2Iterations {
		err := errors.New("The KdfIterations number is too high")
		return jsonapi.InvalidParameter("KdfIterations", err)
	}

	passphrase := []byte(args.Passphrase)
	err = lifecycle.RegisterPassphrase(inst, registerToken, lifecycle.PassParameters{
		Pass:       passphrase,
		Iterations: args.Iterations,
		Key:        args.Key,
		PublicKey:  args.PublicKey,
		PrivateKey: args.PrivateKey,
	})
	if err != nil {
		return jsonapi.BadRequest(err)
	}

	if args.Hint != "" {
		setting, err := settings.Get(inst)
		if err != nil {
			return err
		}
		setting.PassphraseHint = args.Hint
		if err := setting.Save(inst); err != nil {
			return err
		}
	}

	longRunSession := true
	sessionID, err := auth.SetCookieForNewSession(c, longRunSession)
	if err != nil {
		return err
	}
	if err := session.StoreNewLoginEntry(inst, sessionID, "", c.Request(), false); err != nil {
		inst.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
	}

	return finishOnboarding(c, acceptHTML)
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
		Iterations        int    `json:"iterations"`
		TwoFactorPasscode string `json:"two_factor_passcode"`
		TwoFactorToken    []byte `json:"two_factor_token"`
		Force             bool   `json:"force,omitempty"`
		Key               string `json:"key"`
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

	if args.Iterations < crypto.MinPBKDF2Iterations && args.Iterations != 0 {
		err := errors.New("The KdfIterations number is too low")
		return jsonapi.InvalidParameter("KdfIterations", err)
	}
	if args.Iterations > crypto.MaxPBKDF2Iterations {
		err := errors.New("The KdfIterations number is too high")
		return jsonapi.InvalidParameter("KdfIterations", err)
	}

	err = lifecycle.UpdatePassphrase(inst, currentPassphrase,
		args.TwoFactorPasscode, args.TwoFactorToken,
		lifecycle.PassParameters{
			Pass:       newPassphrase,
			Iterations: args.Iterations,
			Key:        args.Key,
		})
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

func updateHint(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	if err := middlewares.AllowWholeType(c, permission.PUT, consts.Settings); err != nil {
		return err
	}

	args := struct {
		Hint string `json:"hint"`
	}{}
	if err := c.Bind(&args); err != nil {
		return jsonapi.BadRequest(err)
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	setting.PassphraseHint = args.Hint
	if err := setting.Save(inst); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
