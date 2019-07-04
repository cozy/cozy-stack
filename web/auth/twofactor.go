package auth

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/limits"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func renderTwoFactorForm(c echo.Context, i *instance.Instance, code int, credsError string, redirect *url.URL, twoFactorToken []byte, longRunSession bool) error {
	title := i.Translate("Login Two factor title")

	redirectQuery := redirect.Query()
	var clientScope string
	if clientScopes := redirectQuery["scope"]; len(clientScopes) > 0 {
		clientScope = clientScopes[0]
	}

	oauth := i.HasDomain(redirect.Host) && redirect.Path == "/auth/authorize" && clientScope != oauth.ScopeLogin
	return c.Render(code, "twofactor.html", echo.Map{
		"CozyUI":           middlewares.CozyUI(i),
		"ThemeCSS":         middlewares.ThemeCSS(i),
		"Domain":           i.ContextualDomain(),
		"ContextName":      i.ContextName,
		"Locale":           i.Locale,
		"Title":            title,
		"CredentialsError": credsError,
		"Redirect":         redirect.String(),
		"LongRunSession":   longRunSession,
		"TwoFactorToken":   string(twoFactorToken),
		"CSRF":             c.Get("csrf"),
		"OAuth":            oauth,
		"Favicon":          middlewares.Favicon(i),
	})
}

// twoFactorForm handles a GET request
func twoFactorForm(c echo.Context) error {
	var credsError string
	inst := middlewares.GetInstance(c)

	redirect, err := checkRedirectParam(c, inst.DefaultRedirection())
	if err != nil {
		return err
	}

	twoFactorToken, err := lifecycle.SendTwoFactorPasscode(inst)
	if err != nil {
		return err
	}

	longRunSession, err := strconv.ParseBool(c.FormValue("long-run-session"))
	if err != nil {
		longRunSession = true
	}

	return renderTwoFactorForm(c, inst, http.StatusOK, credsError, redirect, twoFactorToken, longRunSession)
}

// twoFactor handles a POST request
func twoFactor(c echo.Context) error {
	wantsJSON := c.Request().Header.Get(echo.HeaderAccept) == echo.MIMEApplicationJSON

	inst := middlewares.GetInstance(c)
	redirect, err := checkRedirectParam(c, inst.DefaultRedirection())
	if err != nil {
		return err
	}

	// Retreiving data from request
	token := []byte(c.FormValue("two-factor-token"))
	passcode := c.FormValue("two-factor-passcode")
	generateTrustedDeviceToken, _ := strconv.ParseBool(c.FormValue("two-factor-generate-trusted-device-token"))
	longRunSession, _ := strconv.ParseBool(c.FormValue("long-run-session"))

	correctPasscode := inst.ValidateTwoFactorPasscode(token, passcode)

	if correctPasscode {
		var sessionID string
		// Generate a session
		if sessionID, err = SetCookieForNewSession(c, longRunSession); err != nil {
			return err
		}

		var clientID string
		if inst.HasDomain(redirect.Host) && redirect.Path == "/auth/authorize" {
			// NOTE: the login scope is used by external clients for authentication.
			// Typically, these clients are used for internal purposes, like
			// authenticating to an external system via the cozy. For these clients
			// we do not push a "client" notification, we only store a new login
			// history.
			if redirect.Query().Get("scope") != oauth.ScopeLogin {
				clientID = redirect.Query().Get("client_id")
			}
		}

		if err = session.StoreNewLoginEntry(inst, sessionID, clientID, c.Request(), true); err != nil {
			inst.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
		}

		// logged-in
		redirect = AddCodeToRedirect(redirect, inst.ContextualDomain(), sessionID)

		// Check if the user trusts its device
		var generatedTrustedDeviceToken []byte
		if generateTrustedDeviceToken {
			generatedTrustedDeviceToken, _ = inst.GenerateTwoFactorTrustedDeviceSecret(c.Request())
		}
		if wantsJSON {
			result := echo.Map{"redirect": redirect.String()}
			if len(generatedTrustedDeviceToken) > 0 {
				result["two_factor_trusted_device_token"] = string(generatedTrustedDeviceToken)
			}
			return c.JSON(http.StatusOK, result)
		}
	}

	// Handle 2FA failed
	if !correctPasscode {
		errorMessage := inst.Translate(TwoFactorErrorKey)
		errCheckRateLimit := limits.CheckRateLimit(inst, limits.TwoFactorType)
		if errCheckRateLimit == limits.ErrRateLimitExceeded {
			if err := TwoFactorRateExceeded(inst); err != nil {
				inst.Logger().WithField("nspace", "auth").Warning(err)
				errorMessage = inst.Translate(TwoFactorExceededErrorKey)
			}
		}
		// Renders either the passcode page or a JSON message
		if wantsJSON {
			return c.JSON(http.StatusUnauthorized, echo.Map{
				"error": errorMessage,
			})
		}
		return renderTwoFactorForm(c, inst, http.StatusUnauthorized, errorMessage, redirect, token, longRunSession)
	}

	return c.Redirect(http.StatusSeeOther, redirect.String())
}
