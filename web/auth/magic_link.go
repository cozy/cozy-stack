package auth

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/config/config"
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
