package auth

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

func renderTwoFactorForm(c echo.Context, i *instance.Instance, code int, credsError string, redirect *url.URL, twoFactorToken []byte, longRunSession bool, trustedDeviceCheckBox bool) error {
	title := i.Translate("Login Two factor title")

	redirectQuery := redirect.Query()
	var clientScope string
	if clientScopes := redirectQuery["scope"]; len(clientScopes) > 0 {
		clientScope = clientScopes[0]
	}

	oauth := i.HasDomain(redirect.Host) && redirect.Path == "/auth/authorize" && clientScope != oauth.ScopeLogin
	trustedCheckbox := !oauth && trustedDeviceCheckBox

	return c.Render(code, "twofactor.html", echo.Map{
		"CozyUI":                middlewares.CozyUI(i),
		"ThemeCSS":              middlewares.ThemeCSS(i),
		"Domain":                i.ContextualDomain(),
		"ContextName":           i.ContextName,
		"Locale":                i.Locale,
		"Title":                 title,
		"CredentialsError":      credsError,
		"Redirect":              redirect.String(),
		"LongRunSession":        longRunSession,
		"TwoFactorToken":        string(twoFactorToken),
		"Favicon":               middlewares.Favicon(i),
		"TrustedDeviceCheckBox": trustedCheckbox,
	})
}

// twoFactorForm handles the twoFactor from GET request
func twoFactorForm(c echo.Context) error {
	var credsError string
	trustedDeviceCheckBox := true

	inst := middlewares.GetInstance(c)

	redirect, err := checkRedirectParam(c, inst.DefaultRedirection())
	if err != nil {
		return err
	}

	twoFactorTokenParam := c.QueryParams().Get("two_factor_token")
	if twoFactorTokenParam == "" {
		return c.JSON(http.StatusBadRequest, "Missing twoFactorToken")
	}

	twoFactorToken := []byte(twoFactorTokenParam)

	longRunSession := false
	if longRunParam := c.QueryParam("long-run-session"); longRunParam != "" {
		longRunSession, err = strconv.ParseBool(longRunParam)
		if err != nil {
			return err
		}
	}

	trustedDeviceCheckBoxParam := c.QueryParams().Get("trusted_device_checkbox")
	if trustedDeviceCheckBoxParam != "" {
		if b, err := strconv.ParseBool(trustedDeviceCheckBoxParam); err == nil {
			trustedDeviceCheckBox = b
		}
	}

	return renderTwoFactorForm(c, inst, http.StatusOK, credsError, redirect, twoFactorToken, longRunSession, trustedDeviceCheckBox)
}

// twoFactor handles a the twoFactor POST request
func twoFactor(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	redirect, err := checkRedirectParam(c, inst.DefaultRedirection())
	if err != nil {
		return err
	}

	trustedDeviceCheckBox := false
	trustedDeviceCheckBoxParam := c.QueryParams().Get("trusted_device_checkbox")
	if trustedDeviceCheckBoxParam != "" {
		if b, err := strconv.ParseBool(trustedDeviceCheckBoxParam); err != nil {
			trustedDeviceCheckBox = b
		}
	}

	// Retreiving data from request
	token := []byte(c.FormValue("two-factor-token"))
	passcode := c.FormValue("two-factor-passcode")
	generateTrustedDeviceToken, _ := strconv.ParseBool(c.FormValue("two-factor-generate-trusted-device-token"))

	longRunSession := false
	if longRunParam := c.FormValue("long-run-session"); longRunParam != "" {
		longRunSession, err = strconv.ParseBool(longRunParam)
		if err != nil {
			return err
		}
	}

	// Handle 2FA failed
	correctPasscode := inst.ValidateTwoFactorPasscode(token, passcode)
	if !correctPasscode {
		return twoFactorFailed(c, inst, redirect, token, longRunSession, trustedDeviceCheckBox)
	}

	// Generate a new session
	if err := newSession(c, inst, redirect, longRunSession); err != nil {
		return err
	}
	// Check if the user trusts its device
	var generatedTrustedDeviceToken []byte
	if generateTrustedDeviceToken {
		generatedTrustedDeviceToken, _ = inst.GenerateTwoFactorTrustedDeviceSecret(c.Request())
	}
	if wantsJSON(c) {
		result := echo.Map{"redirect": redirect.String()}
		if len(generatedTrustedDeviceToken) > 0 {
			result["two_factor_trusted_device_token"] = string(generatedTrustedDeviceToken)
		}
		return c.JSON(http.StatusOK, result)
	}

	return c.Redirect(http.StatusSeeOther, redirect.String())
}

// twoFactorFailed returns the 2FA form with an error message
func twoFactorFailed(c echo.Context, inst *instance.Instance, redirect *url.URL, token []byte, longRunSession bool, trustedDeviceCheckBox bool) error {
	errorMessage := inst.Translate(TwoFactorErrorKey)

	errCheckRateLimit := limits.CheckRateLimit(inst, limits.TwoFactorType)
	if errCheckRateLimit == limits.ErrRateLimitExceeded {
		if err := TwoFactorRateExceeded(inst); err != nil {
			inst.Logger().WithField("nspace", "auth").Warning(err)
			errorMessage = inst.Translate(TwoFactorExceededErrorKey)
		}
	}
	// Render either the passcode page or a JSON message
	if wantsJSON(c) {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": errorMessage,
		})
	}

	return renderTwoFactorForm(c, inst, http.StatusUnauthorized, errorMessage, redirect, token, longRunSession, trustedDeviceCheckBox)
}
