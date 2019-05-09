// Package auth provides register and login handlers
package auth

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/session"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	"github.com/cozy/echo/middleware"
)

const (
	// CredentialsErrorKey is the key for translating the message showed to the
	// user when he/she enters incorrect credentials
	CredentialsErrorKey = "Login Credentials error"
	// TwoFactorErrorKey is the key for translating the message showed to the
	// user when he/she enters incorrect two factor secret
	TwoFactorErrorKey = "Login Two factor error"
	// TwoFactorExceededErrorKey is the key for translating the message showed to the
	// user when there were too many attempts
	TwoFactorExceededErrorKey = "Login Two factor attempts error"
)

func renderError(c echo.Context, code int, msg string) error {
	instance := middlewares.GetInstance(c)
	return c.Render(code, "error.html", echo.Map{
		"CozyUI":      middlewares.CozyUI(instance),
		"ThemeCSS":    middlewares.ThemeCSS(instance),
		"Domain":      instance.ContextualDomain(),
		"ContextName": instance.ContextName,
		"Error":       msg,
	})
}

// Home is the handler for /
// It redirects to the login page is the user is not yet authentified
// Else, it redirects to its home application (or onboarding)
func Home(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	if session, ok := middlewares.GetSession(c); ok {
		redirect := instance.DefaultRedirection()
		redirect = AddCodeToRedirect(redirect, instance.ContextualDomain(), session.ID())
		cookie, err := session.ToCookie()
		if err != nil {
			return err
		}
		c.SetCookie(cookie)
		return c.Redirect(http.StatusSeeOther, redirect.String())
	}

	if len(instance.RegisterToken) > 0 && !instance.OnboardingFinished {
		if !middlewares.CheckRegisterToken(c, instance) {
			return c.Render(http.StatusOK, "need_onboarding.html", echo.Map{
				"ThemeCSS":    middlewares.ThemeCSS(instance),
				"Domain":      instance.ContextualDomain(),
				"ContextName": instance.ContextName,
				"Locale":      instance.Locale,
			})
		}
		return c.Redirect(http.StatusSeeOther, instance.PageURL("/auth/passphrase", c.QueryParams()))
	}

	var params url.Values
	if jwt := c.QueryParam("jwt"); jwt != "" {
		params = url.Values{"jwt": {jwt}}
	}
	return c.Redirect(http.StatusSeeOther, instance.PageURL("/auth/login", params))
}

// AddCodeToRedirect adds a code to a redirect URL to transfer the session.
// With the nested subdomains structure, the cookie set on the main domain can
// also be used to authentify the user on the apps subdomain. But with the flat
// subdomains structure, a new cookie is needed. To transfer the session, we
// add a code parameter to the redirect URL that can be exchanged to the
// cookie. The code can be used only once, is valid only one minute, and is
// specific to the app (it can't be used by another app).
func AddCodeToRedirect(redirect *url.URL, domain, sessionID string) *url.URL {
	// TODO add rate-limiting on the number of session codes generated
	if config.GetConfig().Subdomains == config.FlatSubdomains {
		redirect = utils.CloneURL(redirect)
		if redirect.Host != domain {
			q := redirect.Query()
			q.Set("code", session.BuildCode(sessionID, redirect.Host).Value)
			redirect.RawQuery = q.Encode()
			return redirect
		}
	}
	return redirect
}

// SetCookieForNewSession creates a new session and sets the cookie on echo context
func SetCookieForNewSession(c echo.Context, longRunSession bool) (string, error) {
	instance := middlewares.GetInstance(c)
	session, err := session.New(instance, longRunSession)
	if err != nil {
		return "", err
	}
	cookie, err := session.ToCookie()
	if err != nil {
		return "", err
	}
	c.SetCookie(cookie)
	return session.ID(), nil
}

func renderLoginForm(c echo.Context, i *instance.Instance, code int, credsErrors string, redirect *url.URL) error {
	if !i.IsPasswordAuthenticationEnabled() {
		return c.Redirect(http.StatusSeeOther, i.PageURL("/oidc/start", nil))
	}

	var title, help string

	publicName, err := i.PublicName()
	if err != nil {
		publicName = ""
	}

	redirectStr := redirect.String()
	redirectQuery := redirect.Query()

	var clientScope string
	if clientScopes := redirectQuery["scope"]; len(clientScopes) > 0 {
		clientScope = clientScopes[0]
	}

	oauth := i.HasDomain(redirect.Host) && redirect.Path == "/auth/authorize" && clientScope != oauth.ScopeLogin

	if c.QueryParam("msg") == "passphrase-reset-requested" {
		title = i.Translate("Login Connect after reset requested title")
		help = i.Translate("Login Connect after reset requested help")
	} else if strings.Contains(redirectStr, "reconnect") {
		title = i.Translate("Login Reconnect title")
		help = i.Translate("Login Reconnect help")
	} else if i.HasDomain(redirect.Host) && redirect.Path == "/auth/authorize/sharing" {
		title = i.Translate("Login Connect from sharing title", publicName)
		help = i.Translate("Login Connect from sharing help")
	} else {
		if publicName == "" {
			title = i.Translate("Login Welcome")
		} else {
			title = i.Translate("Login Welcome name", publicName)
		}
		help = i.Translate("Login Password help")
	}

	return c.Render(code, "login.html", echo.Map{
		"TemplateTitle":    i.TemplateTitle(),
		"CozyUI":           middlewares.CozyUI(i),
		"ThemeCSS":         middlewares.ThemeCSS(i),
		"Domain":           i.ContextualDomain(),
		"ContextName":      i.ContextName,
		"Locale":           i.Locale,
		"Title":            title,
		"PasswordHelp":     help,
		"CredentialsError": credsErrors,
		"Redirect":         redirectStr,
		"TwoFactorForm":    false,
		"TwoFactorToken":   "",
		"CSRF":             c.Get("csrf"),
		"OAuth":            oauth,
	})
}

func renderTwoFactorForm(c echo.Context, i *instance.Instance, code int, redirect *url.URL, twoFactorToken []byte, longRunSession bool) error {
	title := i.Translate("Login Two factor title")

	redirectQuery := redirect.Query()
	var clientScope string
	if clientScopes := redirectQuery["scope"]; len(clientScopes) > 0 {
		clientScope = clientScopes[0]
	}

	oauth := i.HasDomain(redirect.Host) && redirect.Path == "/auth/authorize" && clientScope != oauth.ScopeLogin
	return c.Render(code, "login.html", echo.Map{
		"CozyUI":           middlewares.CozyUI(i),
		"ThemeCSS":         middlewares.ThemeCSS(i),
		"Domain":           i.ContextualDomain(),
		"ContextName":      i.ContextName,
		"Locale":           i.Locale,
		"Title":            title,
		"PasswordHelp":     "",
		"CredentialsError": nil,
		"Redirect":         redirect.String(),
		"LongRunSession":   longRunSession,
		"TwoFactorForm":    true,
		"TwoFactorToken":   string(twoFactorToken),
		"CSRF":             c.Get("csrf"),
		"OAuth":            oauth,
	})
}

func loginForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	redirect, err := checkRedirectParam(c, instance.DefaultRedirection())
	if err != nil {
		return err
	}

	sess, ok := middlewares.GetSession(c)
	if ok {
		redirect = AddCodeToRedirect(redirect, instance.ContextualDomain(), sess.ID())
		cookie, err := sess.ToCookie()
		if err != nil {
			return err
		}
		c.SetCookie(cookie)
		return c.Redirect(http.StatusSeeOther, redirect.String())
	}
	// Delegated JWT
	if token := c.QueryParam("jwt"); token != "" {
		err := session.CheckDelegatedJWT(instance, token)
		if err != nil {
			instance.Logger().Warningf("Delegated token check failed: %s", err)
		} else {
			sessionID, err := SetCookieForNewSession(c, false)
			if err != nil {
				return err
			}
			if err = session.StoreNewLoginEntry(instance, sessionID, "", c.Request(), true); err != nil {
				instance.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
			}
			redirect = AddCodeToRedirect(redirect, instance.ContextualDomain(), sessionID)
			return c.Redirect(http.StatusSeeOther, redirect.String())
		}
	}
	return renderLoginForm(c, instance, http.StatusOK, "", redirect)
}

func login(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	wantsJSON := c.Request().Header.Get("Accept") == "application/json"

	redirect, err := checkRedirectParam(c, inst.DefaultRedirection())
	if err != nil {
		return err
	}

	successfulAuthentication := false
	twoFactorToken := []byte(c.FormValue("two-factor-token"))
	twoFactorPasscode := c.FormValue("two-factor-passcode")
	twoFactorTrustedDeviceToken := []byte(c.FormValue("two-factor-trusted-device-token"))
	twoFactorGenerateTrustedDeviceToken, _ := strconv.ParseBool(c.FormValue("two-factor-generate-trusted-device-token"))
	passphrase := []byte(c.FormValue("passphrase"))
	longRunSession, _ := strconv.ParseBool(c.FormValue("long-run-session"))

	var twoFactorGeneratedTrustedDeviceToken []byte
	var errCheckRateLimit error

	twoFactorRequest := len(twoFactorToken) > 0 && twoFactorPasscode != ""
	passphraseRequest := len(passphrase) > 0

	var sessionID string
	sess, ok := middlewares.GetSession(c)
	if ok { // The user was already logged-in
		sessionID = sess.ID()
	} else if twoFactorRequest {
		successfulAuthentication = inst.ValidateTwoFactorPasscode(
			twoFactorToken, twoFactorPasscode)

		if successfulAuthentication && twoFactorGenerateTrustedDeviceToken {
			twoFactorGeneratedTrustedDeviceToken, _ =
				inst.GenerateTwoFactorTrustedDeviceSecret(c.Request())
		}
		// Handle 2FA failed
		if !successfulAuthentication {
			errCheckRateLimit = limits.CheckRateLimit(inst, limits.TwoFactorType)
			if errCheckRateLimit != nil {
				if err = TwoFactorRateExceeded(inst); err != nil {
					inst.Logger().WithField("nspace", "auth").Warning(err)
				}
			}
		}
	} else if passphraseRequest {
		if lifecycle.CheckPassphrase(inst, passphrase) == nil {
			// In case the second factor authentication mode is "mail", we also
			// check that the mail has been confirmed. If not, 2FA is not actived.
			if inst.HasAuthMode(instance.TwoFactorMail) {

				successfulAuthentication = inst.ValidateTwoFactorTrustedDeviceSecret(
					c.Request(), twoFactorTrustedDeviceToken)

				if len(twoFactorTrustedDeviceToken) > 0 && !successfulAuthentication {
					// If the token is bad, maybe the password had been changed, and
					// the token is now expired. We are going to empty it and ask a
					// regeneration
					twoFactorTrustedDeviceToken = []byte{}
				}

				if len(twoFactorTrustedDeviceToken) == 0 {
					twoFactorToken, err = lifecycle.SendTwoFactorPasscode(inst)
					if err != nil {
						return err
					}
					if wantsJSON {
						return c.JSON(http.StatusOK, echo.Map{
							"redirect":         redirect.String(),
							"two_factor_token": string(twoFactorToken),
						})
					}
					return renderTwoFactorForm(c, inst, http.StatusOK, redirect, twoFactorToken, longRunSession)
				}
			} else {
				successfulAuthentication = true
			}
		} else { // Bad login passphrase
			if err := limits.CheckRateLimit(inst, limits.AuthType); err != nil {
				if err = LoginRateExceeded(inst); err != nil {
					inst.Logger().WithField("nspace", "auth").Warning(err)
				}
			}
		}
	}

	if successfulAuthentication {
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
	}

	// not logged-in
	if sessionID == "" {
		var errorMessage string
		if twoFactorRequest {
			if errCheckRateLimit != nil {
				errorMessage = inst.Translate(TwoFactorExceededErrorKey)
			} else {
				errorMessage = inst.Translate(TwoFactorErrorKey)
			}
		} else {
			errorMessage = inst.Translate(CredentialsErrorKey)
		}
		if wantsJSON {
			return c.JSON(http.StatusUnauthorized, echo.Map{
				"error": errorMessage,
			})
		}
		return renderLoginForm(c, inst, http.StatusUnauthorized,
			errorMessage, redirect)
	}

	// logged-in
	redirect = AddCodeToRedirect(redirect, inst.ContextualDomain(), sessionID)
	if wantsJSON {
		result := echo.Map{"redirect": redirect.String()}
		if len(twoFactorGeneratedTrustedDeviceToken) > 0 {
			result["two_factor_trusted_device_token"] = string(twoFactorGeneratedTrustedDeviceToken)
		}
		return c.JSON(http.StatusOK, result)
	}

	return c.Redirect(http.StatusSeeOther, redirect.String())
}

func logout(c echo.Context) error {
	res := c.Response()
	origin := c.Request().Header.Get(echo.HeaderOrigin)
	res.Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
	res.Header().Set(echo.HeaderAccessControlAllowCredentials, "true")

	instance := middlewares.GetInstance(c)
	if !middlewares.AllowLogout(c) {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "The user can logout only from client-side apps",
		})
	}

	session, ok := middlewares.GetSession(c)
	if ok {
		c.SetCookie(session.Delete(instance))
	}

	return c.NoContent(http.StatusNoContent)
}

func logoutOthers(c echo.Context) error {
	res := c.Response()
	origin := c.Request().Header.Get(echo.HeaderOrigin)
	res.Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
	res.Header().Set(echo.HeaderAccessControlAllowCredentials, "true")

	instance := middlewares.GetInstance(c)
	if !middlewares.AllowLogout(c) {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "The user can logout only from client-side apps",
		})
	}

	sess, ok := middlewares.GetSession(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "Could not retrieve session",
		})
	}
	if err := session.DeleteOthers(instance, sess.ID()); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func logoutPreflight(c echo.Context) error {
	req := c.Request()
	res := c.Response()
	origin := req.Header.Get(echo.HeaderOrigin)

	res.Header().Add(echo.HeaderVary, echo.HeaderOrigin)
	res.Header().Add(echo.HeaderVary, echo.HeaderAccessControlRequestMethod)
	res.Header().Add(echo.HeaderVary, echo.HeaderAccessControlRequestHeaders)
	res.Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
	res.Header().Set(echo.HeaderAccessControlAllowMethods, echo.DELETE)
	res.Header().Set(echo.HeaderAccessControlAllowCredentials, "true")
	res.Header().Set(echo.HeaderAccessControlMaxAge, middlewares.MaxAgeCORS)
	if h := req.Header.Get(echo.HeaderAccessControlRequestHeaders); h != "" {
		res.Header().Set(echo.HeaderAccessControlAllowHeaders, h)
	}

	return c.NoContent(http.StatusNoContent)
}

// checkRedirectParam returns the optional redirect query parameter. If not
// empty, we check that the redirect is a subdomain of the cozy-instance.
func checkRedirectParam(c echo.Context, defaultRedirect *url.URL) (*url.URL, error) {
	redirect := c.FormValue("redirect")
	if redirect == "" {
		redirect = defaultRedirect.String()
	}

	u, err := url.Parse(redirect)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest,
			"bad url: could not parse")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, echo.NewHTTPError(http.StatusBadRequest,
			"bad url: bad scheme")
	}

	instance := middlewares.GetInstance(c)
	if !instance.HasDomain(u.Host) {
		instanceHost, appSlug, _ := middlewares.SplitHost(u.Host)
		if !instance.HasDomain(instanceHost) || appSlug == "" {
			return nil, echo.NewHTTPError(http.StatusBadRequest,
				"bad url: should be subdomain")
		}
		return u, nil
	}

	// To protect against stealing authorization code with redirection, the
	// fragment is always overridden. Most browsers keep URI fragments upon
	// redirects, to make sure to override them, we put an empty one.
	//
	// see: oauthsecurity.com/#provider-in-the-middle
	// see: 7.4.2 OAuth2 in Action
	u.Fragment = "="
	return u, nil
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	noCSRF := middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "form:csrf_token",
		CookieMaxAge:   3600, // 1 hour
		CookieHTTPOnly: true,
		CookieSecure:   !build.IsDevRelease(),
	})

	// Login/logout
	router.GET("/login", loginForm, noCSRF)
	router.POST("/login", login, noCSRF)
	router.DELETE("/login/others", logoutOthers)
	router.OPTIONS("/login/others", logoutPreflight)
	router.DELETE("/login", logout)
	router.OPTIONS("/login", logoutPreflight)

	// Passphrase
	router.GET("/passphrase_reset", passphraseResetForm, noCSRF)
	router.POST("/passphrase_reset", passphraseReset, noCSRF)
	router.GET("/passphrase_renew", passphraseRenewForm, noCSRF)
	router.POST("/passphrase_renew", passphraseRenew, noCSRF)
	router.GET("/passphrase", passphraseForm, noCSRF)

	// Register OAuth clients
	router.POST("/register", registerClient, middlewares.AcceptJSON, middlewares.ContentTypeJSON)
	router.GET("/register/:client-id", readClient, middlewares.AcceptJSON, checkRegistrationToken)
	router.PUT("/register/:client-id", updateClient, middlewares.AcceptJSON, middlewares.ContentTypeJSON, checkRegistrationToken)
	router.DELETE("/register/:client-id", deleteClient, checkRegistrationToken)

	// OAuth flow
	authorizeGroup := router.Group("/authorize", noCSRF)
	authorizeGroup.GET("", authorizeForm)
	authorizeGroup.POST("", authorize)
	authorizeGroup.GET("/sharing", authorizeSharingForm)
	authorizeGroup.POST("/sharing", authorizeSharing)
	authorizeGroup.GET("/app", authorizeAppForm)
	authorizeGroup.POST("/app", authorizeApp)

	router.POST("/access_token", accessToken)
	router.POST("/secret_exchange", secretExchange)
}
