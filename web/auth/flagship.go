package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// CreateSessionCode is the handler for creating a session code by the flagship
// app.
func CreateSessionCode(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	switch canCreateSessionCode(c, inst) {
	case allowedToCreateSessionCode:
		// OK
	case need2FAToCreateSessionCode:
		twoFactorToken, err := lifecycle.SendTwoFactorPasscode(inst)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusForbidden, echo.Map{
			"error":            "two factor needed",
			"two_factor_token": string(twoFactorToken),
		})
	default:
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "Not authorized",
		})
	}

	code, err := inst.CreateSessionCode()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	req := c.Request()
	var ip string
	if forwardedFor := req.Header.Get(echo.HeaderXForwardedFor); forwardedFor != "" {
		ip = strings.TrimSpace(strings.SplitN(forwardedFor, ",", 2)[0])
	}
	if ip == "" {
		ip = strings.Split(req.RemoteAddr, ":")[0]
	}
	inst.Logger().WithField("nspace", "loginaudit").
		Infof("New session_code created from %s at %s", ip, time.Now())

	return c.JSON(http.StatusCreated, echo.Map{
		"session_code": code,
	})
}

type sessionCodeParameters struct {
	Passphrase     string `json:"passphrase"`
	TwoFactorToken string `json:"two_factor_token"`
	TwoFactorCode  string `json:"two_factor_passcode"`
}

type canCreateSessionCodeResult int

const (
	allowedToCreateSessionCode canCreateSessionCodeResult = iota
	cannotCreateSessionCode
	need2FAToCreateSessionCode
)

func canCreateSessionCode(c echo.Context, inst *instance.Instance) canCreateSessionCodeResult {
	pdoc, err := middlewares.GetPermission(c)
	if err == nil && pdoc.Permissions.IsMaximal() {
		return allowedToCreateSessionCode
	}

	var args sessionCodeParameters
	if err := c.Bind(&args); err != nil {
		return cannotCreateSessionCode
	}
	if err := lifecycle.CheckPassphrase(inst, []byte(args.Passphrase)); err != nil {
		return cannotCreateSessionCode
	}

	if inst.HasAuthMode(instance.TwoFactorMail) {
		token := []byte(args.TwoFactorToken)
		if ok := inst.ValidateTwoFactorPasscode(token, args.TwoFactorCode); !ok {
			return need2FAToCreateSessionCode
		}
	}
	return allowedToCreateSessionCode
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
	inst := middlewares.GetInstance(c)
	client, err := oauth.FindClient(inst, c.Param("client-id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": "Client not found",
		})
	}

	err = limits.CheckRateLimit(inst, limits.ConfirmFlagshipType)
	if limits.IsLimitReachedOrExceeded(err) {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": inst.Translate("Confirm Flagship Invalid code"),
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

	if err := client.SetFlagship(inst); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error,
		})
	}
	return c.NoContent(http.StatusNoContent)
}
