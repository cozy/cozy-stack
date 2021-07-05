package auth

import (
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

func passphraseResetForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	hasHint := false
	if setting, err := settings.Get(instance); err == nil {
		hasHint = setting.PassphraseHint != ""
	}
	hasCiphers := true
	if resp, err := couchdb.NormalDocs(instance, consts.BitwardenCiphers, 0, 1, "", false); err == nil {
		hasCiphers = resp.Total > 0
	}
	passwordAuth := instance.IsPasswordAuthenticationEnabled()
	return c.Render(http.StatusOK, "passphrase_reset.html", echo.Map{
		"Domain":      instance.ContextualDomain(),
		"ContextName": instance.ContextName,
		"Locale":      instance.Locale,
		"Title":       instance.TemplateTitle(),
		"Favicon":     middlewares.Favicon(instance),
		"CSRF":        c.Get("csrf"),
		"Redirect":    c.QueryParam("redirect"),
		"HasHint":     hasHint,
		"HasCiphers":  hasCiphers,
		"CozyPass":    !passwordAuth,
	})
}

func passphraseForm(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	registerToken := c.QueryParams().Get("registerToken")
	if inst.OnboardingFinished {
		redirect := inst.DefaultRedirection()
		return c.Redirect(http.StatusSeeOther, redirect.String())
	}

	if registerToken == "" || !middlewares.CheckRegisterToken(c, inst) {
		return c.Render(http.StatusOK, "need_onboarding.html", echo.Map{
			"Domain":       inst.ContextualDomain(),
			"ContextName":  inst.ContextName,
			"Locale":       inst.Locale,
			"Title":        inst.TemplateTitle(),
			"Favicon":      middlewares.Favicon(inst),
			"SupportEmail": "contact@cozycloud.cc",
		})
	}

	cryptoPolyfill := middlewares.CryptoPolyfill(c)
	iterations := crypto.DefaultPBKDF2Iterations
	if cryptoPolyfill {
		iterations = crypto.EdgePBKDF2Iterations
	}
	matomo := config.GetConfig().Matomo
	if matomo.URL != "" {
		middlewares.AppendCSPRule(c, "img", matomo.URL)
	}

	return c.Render(http.StatusOK, "passphrase_choose.html", echo.Map{
		"Domain":         inst.ContextualDomain(),
		"ContextName":    inst.ContextName,
		"Locale":         inst.Locale,
		"Title":          inst.TemplateTitle(),
		"Favicon":        middlewares.Favicon(inst),
		"Action":         "/settings/passphrase",
		"Iterations":     iterations,
		"Salt":           string(inst.PassphraseSalt()),
		"RegisterToken":  registerToken,
		"CryptoPolyfill": cryptoPolyfill,
		"MatomoURL":      matomo.URL,
		"MatomoSiteID":   matomo.SiteID,
		"MatomoAppID":    matomo.OnboardingAppID,
	})
}

func sendHint(c echo.Context) error {
	i := middlewares.GetInstance(c)
	if err := limits.CheckRateLimit(i, limits.SendHintByMail); err == nil {
		if err := lifecycle.SendHint(i); err != nil {
			return err
		}
	}
	var u url.Values
	if redirect := c.FormValue("redirect"); redirect != "" {
		u = url.Values{"redirect": {redirect}}
	}
	return c.Render(http.StatusOK, "error.html", echo.Map{
		"Domain":       i.ContextualDomain(),
		"ContextName":  i.ContextName,
		"Locale":       i.Locale,
		"Title":        i.TemplateTitle(),
		"Favicon":      middlewares.Favicon(i),
		"Inverted":     true,
		"Illustration": "/images/mail-sent.svg",
		"ErrorTitle":   "Hint sent Title",
		"Error":        "Hint sent Body",
		"ErrorDetail":  "Hint sent Detail",
		"SupportEmail": i.SupportEmailAddress(),
		"Button":       "Hint sent Login Button",
		"ButtonURL":    i.PageURL("/auth/login", u),
	})
}

func passphraseReset(c echo.Context) error {
	i := middlewares.GetInstance(c)
	if err := lifecycle.RequestPassphraseReset(i); err != nil && err != instance.ErrResetAlreadyRequested {
		return err
	}
	// Disconnect the user if it is logged in. The idea is that if the user
	// (maybe by accident) asks for a passphrase reset while logged in, we log
	// him out to be able to re-go through the process of logging back-in. It is
	// more a UX choice than a "security" one.
	session, ok := middlewares.GetSession(c)
	if ok {
		c.SetCookie(session.Delete(i))
	}
	var u url.Values
	if redirect := c.FormValue("redirect"); redirect != "" {
		u = url.Values{"redirect": {redirect}}
	}
	return c.Render(http.StatusOK, "error.html", echo.Map{
		"Domain":       i.ContextualDomain(),
		"ContextName":  i.ContextName,
		"Locale":       i.Locale,
		"Title":        i.TemplateTitle(),
		"Favicon":      middlewares.Favicon(i),
		"Inverted":     true,
		"Illustration": "/images/mail-sent.svg",
		"ErrorTitle":   "Passphrase is reset Title",
		"Error":        "Passphrase is reset Body",
		"ErrorDetail":  "Passphrase is reset Detail",
		"SupportEmail": i.SupportEmailAddress(),
		"Button":       "Passphrase is reset Login Button",
		"ButtonURL":    i.PageURL("/auth/login", u),
	})
}

func passphraseRenewForm(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if middlewares.IsLoggedIn(c) {
		redirect := inst.DefaultRedirection().String()
		return c.Redirect(http.StatusSeeOther, redirect)
	}

	// Check that the token is actually defined and well encoded. The actual
	// token value checking is also done on the passphraseRenew handler.
	token, err := hex.DecodeString(c.QueryParam("token"))
	if err != nil || len(token) == 0 {
		return renderError(c, http.StatusBadRequest, "Error Invalid reset token")
	}
	if err = lifecycle.CheckPassphraseRenewToken(inst, token); err != nil {
		if err == instance.ErrMissingToken {
			return renderError(c, http.StatusBadRequest, "Error Invalid reset token")
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid_token",
		})
	}

	cryptoPolyfill := middlewares.CryptoPolyfill(c)
	iterations := crypto.DefaultPBKDF2Iterations
	if cryptoPolyfill {
		iterations = crypto.EdgePBKDF2Iterations
	}

	return c.Render(http.StatusOK, "passphrase_choose.html", echo.Map{
		"Domain":         inst.ContextualDomain(),
		"ContextName":    inst.ContextName,
		"Locale":         inst.Locale,
		"Title":          inst.TemplateTitle(),
		"Favicon":        middlewares.Favicon(inst),
		"Action":         "/auth/passphrase_renew",
		"Iterations":     iterations,
		"Salt":           string(inst.PassphraseSalt()),
		"ResetToken":     hex.EncodeToString(token),
		"CSRF":           c.Get("csrf"),
		"CryptoPolyfill": cryptoPolyfill,
	})
}

func passphraseRenew(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if middlewares.IsLoggedIn(c) {
		redirect := inst.DefaultRedirection().String()
		if wantsJSON(c) {
			return c.JSON(http.StatusOK, echo.Map{"redirect": redirect})
		}
		return c.Redirect(http.StatusSeeOther, redirect)
	}
	pass := []byte(c.FormValue("passphrase"))
	iterations, _ := strconv.Atoi(c.FormValue("iterations"))
	token, err := hex.DecodeString(c.FormValue("passphrase_reset_token"))
	if err != nil {
		if wantsJSON(c) {
			return c.JSON(http.StatusUnauthorized, echo.Map{
				"error": "Invalid reset token",
			})
		}
		return renderError(c, http.StatusBadRequest, "Error Invalid reset token")
	}
	err = lifecycle.PassphraseRenew(inst, token, lifecycle.PassParameters{
		Pass:       pass,
		Iterations: iterations,
		Key:        c.FormValue("key"),
		PublicKey:  c.FormValue("public_key"),
		PrivateKey: c.FormValue("private_key"),
		Hint:       c.FormValue("hint"),
	})
	if err != nil {
		if err == instance.ErrMissingToken {
			if wantsJSON(c) {
				return c.JSON(http.StatusUnauthorized, echo.Map{
					"error": "Invalid reset token",
				})
			}
			return renderError(c, http.StatusBadRequest, "Error Invalid reset token")
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid_token",
		})
	}
	if err := bitwarden.DeleteUnrecoverableCiphers(inst); err != nil {
		inst.Logger().WithField("nspace", "bitwarden").
			Warnf("Error on ciphers deletion after password reset: %s", err)
	}

	redirect := inst.PageURL("/auth/login", nil)
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, echo.Map{"redirect": redirect})
	}
	return c.Redirect(http.StatusSeeOther, redirect)
}
