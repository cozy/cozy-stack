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
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

func passphraseResetForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if !instance.IsPasswordAuthenticationEnabled() {
		return c.Redirect(http.StatusSeeOther, instance.PageURL("/oidc/start", nil))
	}
	hasHint := false
	if setting, err := settings.Get(instance); err == nil {
		hasHint = setting.PassphraseHint != ""
	}
	return c.Render(http.StatusOK, "passphrase_reset.html", echo.Map{
		"Title":       instance.TemplateTitle(),
		"CozyUI":      middlewares.CozyUI(instance),
		"ThemeCSS":    middlewares.ThemeCSS(instance),
		"Domain":      instance.ContextualDomain(),
		"ContextName": instance.ContextName,
		"Locale":      instance.Locale,
		"CSRF":        c.Get("csrf"),
		"Favicon":     middlewares.Favicon(instance),
		"Redirect":    c.QueryParam("redirect"),
		"HasHint":     hasHint,
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
			"Title":       inst.TemplateTitle(),
			"ThemeCSS":    middlewares.ThemeCSS(inst),
			"Domain":      inst.ContextualDomain(),
			"ContextName": inst.ContextName,
			"Locale":      inst.Locale,
			"Favicon":     middlewares.Favicon(inst),
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

	return c.Render(http.StatusOK, "passphrase_onboarding.html", echo.Map{
		"CozyUI":         middlewares.CozyUI(inst),
		"Title":          inst.TemplateTitle(),
		"ThemeCSS":       middlewares.ThemeCSS(inst),
		"Domain":         inst.ContextualDomain(),
		"ContextName":    inst.ContextName,
		"Locale":         inst.Locale,
		"Iterations":     iterations,
		"Salt":           string(inst.PassphraseSalt()),
		"RegisterToken":  registerToken,
		"Favicon":        middlewares.Favicon(inst),
		"CryptoPolyfill": cryptoPolyfill,
		"MatomoURL":      matomo.URL,
		"MatomoSiteID":   matomo.SiteID,
		"MatomoAppID":    matomo.OnboardingAppID,
	})
}

func sendHint(c echo.Context) error {
	i := middlewares.GetInstance(c)
	if err := lifecycle.SendHint(i); err != nil {
		return err
	}
	var u url.Values
	if redirect := c.FormValue("redirect"); redirect != "" {
		u = url.Values{"redirect": {redirect}}
	}
	return c.Render(http.StatusOK, "error.html", echo.Map{
		"Title":       i.TemplateTitle(),
		"CozyUI":      middlewares.CozyUI(i),
		"ThemeCSS":    middlewares.ThemeCSS(i),
		"Domain":      i.ContextualDomain(),
		"ContextName": i.ContextName,
		"ErrorTitle":  "Hint sent Title",
		"Error":       "Hint sent Body",
		"Button":      "Hint sent Login Button",
		"ButtonLink":  i.PageURL("/auth/login", u),
		"Favicon":     middlewares.Favicon(i),
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
		"Title":       i.TemplateTitle(),
		"CozyUI":      middlewares.CozyUI(i),
		"ThemeCSS":    middlewares.ThemeCSS(i),
		"Domain":      i.ContextualDomain(),
		"ContextName": i.ContextName,
		"ErrorTitle":  "Passphrase is reset Title",
		"Error":       "Passphrase is reset Body",
		"Button":      "Passphrase is reset Login Button",
		"ButtonLink":  i.PageURL("/auth/login", u),
		"Favicon":     middlewares.Favicon(i),
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

	return c.Render(http.StatusOK, "passphrase_renew.html", echo.Map{
		"Title":                inst.TemplateTitle(),
		"CozyUI":               middlewares.CozyUI(inst),
		"ThemeCSS":             middlewares.ThemeCSS(inst),
		"Domain":               inst.ContextualDomain(),
		"ContextName":          inst.ContextName,
		"Locale":               inst.Locale,
		"Iterations":           iterations,
		"Salt":                 string(inst.PassphraseSalt()),
		"PassphraseResetToken": hex.EncodeToString(token),
		"CSRF":                 c.Get("csrf"),
		"Favicon":              middlewares.Favicon(inst),
		"CryptoPolyfill":       cryptoPolyfill,
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
