package auth

import (
	"crypto/subtle"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

func sendMagicLink(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	err := config.GetRateLimiter().CheckRateLimit(inst, limits.MagicLinkType)
	if limits.IsLimitReachedOrExceeded(err) {
		return echo.NewHTTPError(http.StatusTooManyRequests, "Too many requests")
	}

	redirect := c.FormValue("redirect")
	if err := lifecycle.SendMagicLink(inst, redirect); err != nil {
		return err
	}
	return c.Render(http.StatusOK, "error.html", echo.Map{
		"Domain":       inst.ContextualDomain(),
		"ContextName":  inst.ContextName,
		"Locale":       inst.Locale,
		"Title":        inst.TemplateTitle(),
		"Favicon":      middlewares.Favicon(inst),
		"Inverted":     true,
		"Illustration": "/images/mail-sent.svg",
		"ErrorTitle":   "Magic link has been sent Title",
		"Error":        "Magic link has been sent Body",
		"ErrorDetail":  "Magic link has been sent Detail",
		"SupportEmail": inst.SupportEmailAddress(),
	})
}

func loginWithMagicLink(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	redirect, err := checkRedirectParam(c, inst.DefaultRedirection())
	if err != nil {
		return err
	}

	if _, ok := middlewares.GetSession(c); ok {
		return c.Redirect(http.StatusSeeOther, redirect.String())
	}

	code := c.QueryParam("code")
	if err := lifecycle.CheckMagicLink(inst, code); err != nil {
		err := config.GetRateLimiter().CheckRateLimit(inst, limits.MagicLinkType)
		if limits.IsLimitReachedOrExceeded(err) {
			return echo.NewHTTPError(http.StatusTooManyRequests, "Too many requests")
		}
		return renderError(c, http.StatusBadRequest, "Error Invalid magic link")
	}

	if inst.HasAuthMode(instance.TwoFactorMail) {
		// TODO 2FA
		return renderError(c, http.StatusBadRequest, "Error Invalid magic link")
	}

	err = newSession(c, inst, redirect, session.NormalRun, "magic_link")
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, redirect.String())
}

type magicLinkFlagshipParameters struct {
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	Code              string `json:"magic_code"`
	TwoFactorPasscode string `json:"two_factor_passcode"`
	TwoFactorToken    string `json:"two_factor_token"`
}

func magicLinkFlagship(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	var args magicLinkFlagshipParameters
	if err := c.Bind(&args); err != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	if err := lifecycle.CheckMagicLink(inst, args.Code); err != nil {
		err := config.GetRateLimiter().CheckRateLimit(inst, limits.MagicLinkType)
		if limits.IsLimitReachedOrExceeded(err) {
			return echo.NewHTTPError(http.StatusTooManyRequests, "Too many requests")
		}
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": inst.Translate(CredentialsErrorKey),
		})
	}

	if inst.HasAuthMode(instance.TwoFactorMail) {
		// TODO 2FA
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": inst.Translate(CredentialsErrorKey),
		})
	}

	client, err := oauth.FindClient(inst, args.ClientID)
	if err != nil {
		if couchErr, isCouchErr := couchdb.IsCouchError(err); isCouchErr && couchErr.StatusCode >= 500 {
			return err
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "the client must be registered",
		})
	}
	if subtle.ConstantTimeCompare([]byte(args.ClientSecret), []byte(client.ClientSecret)) == 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid client_secret",
		})
	}

	if !client.Flagship {
		return ReturnSessionCode(c, http.StatusAccepted, inst)
	}

	if client.Pending {
		client.Pending = false
		client.ClientID = ""
		_ = couchdb.UpdateDoc(inst, client)
		client.ClientID = client.CouchID
	}

	out := AccessTokenReponse{
		Type:  "bearer",
		Scope: "*",
	}
	out.Refresh, err = client.CreateJWT(inst, consts.RefreshTokenAudience, out.Scope)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate refresh token",
		})
	}
	out.Access, err = client.CreateJWT(inst, consts.AccessTokenAudience, out.Scope)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate access token",
		})
	}
	return c.JSON(http.StatusOK, out)
}
