// Package auth provides register and login handlers
package auth

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/session"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
	"github.com/mssola/user_agent"
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

func wantsJSON(c echo.Context) bool {
	return c.Request().Header.Get(echo.HeaderAccept) == echo.MIMEApplicationJSON
}

func renderError(c echo.Context, code int, msg string) error {
	instance := middlewares.GetInstance(c)
	return c.Render(code, "error.html", echo.Map{
		"CozyUI":      middlewares.CozyUI(instance),
		"ThemeCSS":    middlewares.ThemeCSS(instance),
		"Domain":      instance.ContextualDomain(),
		"ContextName": instance.ContextName,
		"Error":       msg,
		"Favicon":     middlewares.Favicon(instance),
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
				"Favicon":     middlewares.Favicon(instance),
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

// isTrustedDevice checks if a device of an instance is trusted
func isTrustedDevice(c echo.Context, inst *instance.Instance) bool {
	trustedDeviceToken := []byte(c.FormValue("two-factor-trusted-device-token"))
	return inst.ValidateTwoFactorTrustedDeviceSecret(c.Request(), trustedDeviceToken)
}

func renderLoginForm(c echo.Context, i *instance.Instance, code int, credsErrors string, redirect *url.URL) error {
	if !i.IsPasswordAuthenticationEnabled() {
		return c.Redirect(http.StatusSeeOther, i.PageURL("/oidc/start", nil))
	}

	publicName, err := i.PublicName()
	if err != nil {
		publicName = ""
	}

	var redirectStr string
	var hasOAuth, hasSharing bool
	if redirect != nil {
		redirectStr = redirect.String()
		redirectQuery := redirect.Query()
		var clientScope string
		if clientScopes := redirectQuery["scope"]; len(clientScopes) > 0 {
			clientScope = clientScopes[0]
		}
		if i.HasDomain(redirect.Host) {
			hasOAuth = redirect.Path == "/auth/authorize" && clientScope != oauth.ScopeLogin
			hasSharing = redirect.Path == "/auth/authorize/sharing"
		}
	}

	var title, help string
	if c.QueryParam("msg") == "passphrase-reset-requested" {
		title = i.Translate("Login Connect after reset requested title")
		help = i.Translate("Login Connect after reset requested help")
	} else if strings.Contains(redirectStr, "reconnect") {
		title = i.Translate("Login Reconnect title")
		help = i.Translate("Login Reconnect help")
	} else if hasSharing {
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

	iterations := 0
	if settings, err := settings.Get(i); err == nil {
		iterations = settings.PassphraseKdfIterations
	}

	return c.Render(code, "login.html", echo.Map{
		"TemplateTitle":    i.TemplateTitle(),
		"CozyUI":           middlewares.CozyUI(i),
		"ThemeCSS":         middlewares.ThemeCSS(i),
		"Domain":           i.ContextualDomain(),
		"ContextName":      i.ContextName,
		"Locale":           i.Locale,
		"Iterations":       iterations,
		"Salt":             string(i.PassphraseSalt()),
		"Title":            title,
		"PasswordHelp":     help,
		"CredentialsError": credsErrors,
		"Redirect":         redirectStr,
		"CSRF":             c.Get("csrf"),
		"OAuth":            hasOAuth,
		"Favicon":          middlewares.Favicon(i),
		"CryptoPolyfill":   middlewares.CryptoPolyfill(c),
	})
}

func loginForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	redirect, err := checkRedirectParam(c, nil)
	if err != nil {
		return err
	}

	sess, ok := middlewares.GetSession(c)
	if ok {
		if redirect == nil {
			redirect = instance.DefaultRedirection()
		}
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
			if redirect == nil {
				redirect = instance.DefaultRedirection()
			}
			redirect = AddCodeToRedirect(redirect, instance.ContextualDomain(), sessionID)
			return c.Redirect(http.StatusSeeOther, redirect.String())
		}
	}
	return renderLoginForm(c, instance, http.StatusOK, "", redirect)
}

// newSession generates a new session, and returns its ID
func newSession(c echo.Context, inst *instance.Instance, redirect *url.URL, longRunSession bool) (string, error) {
	var sessionID string
	var err error

	// Generate a session
	if sessionID, err = SetCookieForNewSession(c, longRunSession); err != nil {
		return "", err
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

	return sessionID, nil
}

func migrateToHashedPassphrase(inst *instance.Instance, settings *settings.Settings, passphrase []byte, iterations int) {
	salt := inst.PassphraseSalt()
	pass, masterKey := crypto.HashPassWithPBKDF2(passphrase, salt, iterations)
	hash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		inst.Logger().Errorf("Could not hash the passphrase: %s", err.Error())
		return
	}
	inst.PassphraseHash = hash
	settings.PassphraseKdfIterations = iterations
	settings.PassphraseKdf = instance.PBKDF2_SHA256
	settings.SecurityStamp = lifecycle.NewSecurityStamp()
	key, encKey, err := lifecycle.CreatePassphraseKey(masterKey)
	if err != nil {
		inst.Logger().Errorf("Could not create passphrase key: %s", err.Error())
		return
	}
	settings.Key = key
	pubKey, privKey, err := lifecycle.CreateKeyPair(encKey)
	if err != nil {
		inst.Logger().Errorf("Could not create key pair: %s", err.Error())
		return
	}
	settings.PublicKey = pubKey
	settings.PrivateKey = privKey
	if err := couchdb.UpdateDoc(couchdb.GlobalDB, inst); err != nil {
		inst.Logger().Errorf("Could not update: %s", err.Error())
	}
	if err := settings.Save(inst); err != nil {
		inst.Logger().Errorf("Could not update: %s", err.Error())
	}
}

func login(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	redirect, err := checkRedirectParam(c, inst.DefaultRedirection())
	if err != nil {
		return err
	}

	passphrase := []byte(c.FormValue("passphrase"))
	longRunSession, _ := strconv.ParseBool(c.FormValue("long-run-session"))

	var sessionID string
	sess, ok := middlewares.GetSession(c)
	if ok { // The user was already logged-in
		sessionID = sess.ID()
	} else if lifecycle.CheckPassphrase(inst, passphrase) == nil {
		ua := user_agent.New(c.Request().UserAgent())
		browser, _ := ua.Browser()
		iterations := crypto.DefaultPBKDF2Iterations
		if browser == "Edge" {
			iterations = crypto.EdgePBKDF2Iterations
		}
		settings, err := settings.Get(inst)
		// If the passphrase was not yet hashed on the client side, migrate it
		if err == nil && settings.PassphraseKdfIterations == 0 {
			migrateToHashedPassphrase(inst, settings, passphrase, iterations)
		}

		// In case the second factor authentication mode is "mail", we also
		// check that the mail has been confirmed. If not, 2FA is not
		// activated.
		// If device is trusted, skip the 2FA.
		if inst.HasAuthMode(instance.TwoFactorMail) && !isTrustedDevice(c, inst) {
			twoFactorToken, err := lifecycle.SendTwoFactorPasscode(inst)
			if err != nil {
				return err
			}
			v := url.Values{}
			v.Add("two_factor_token", string(twoFactorToken))
			v.Add("long_run_session", strconv.FormatBool(longRunSession))
			if loc := c.FormValue("redirect"); loc != "" {
				v.Add("redirect", loc)
			}

			if wantsJSON(c) {
				return c.JSON(http.StatusOK, echo.Map{
					"redirect":         inst.PageURL("/auth/twofactor", v),
					"two_factor_token": string(twoFactorToken),
				})
			}
			return c.Redirect(http.StatusSeeOther, inst.PageURL("/auth/twofactor", v))
		}
	} else { // Bad login passphrase
		errorMessage := inst.Translate(CredentialsErrorKey)
		err := limits.CheckRateLimit(inst, limits.AuthType)
		if limits.IsLimitReachedOrExceeded(err) {
			if err = LoginRateExceeded(inst); err != nil {
				inst.Logger().WithField("nspace", "auth").Warning(err)
			}
		}
		if wantsJSON(c) {
			return c.JSON(http.StatusUnauthorized, echo.Map{
				"error": errorMessage,
			})
		}
		return renderLoginForm(c, inst, http.StatusUnauthorized, errorMessage, redirect)
	}

	// Successful authentication
	// User is now logged-in, generate a new sessions
	if sessionID == "" {
		sessionID, err = newSession(c, inst, redirect, longRunSession)
		if err != nil {
			return err
		}
	}
	redirect = AddCodeToRedirect(redirect, inst.ContextualDomain(), sessionID)
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, echo.Map{
			"redirect": redirect.String(),
		})
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
		if defaultRedirect == nil {
			return defaultRedirect, nil
		}
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
	noCSRF := middlewares.CSRFWithConfig(middlewares.CSRFConfig{
		TokenLookup:    "form:csrf_token",
		CookieMaxAge:   3600, // 1 hour
		CookieHTTPOnly: true,
		CookieSecure:   !build.IsDevRelease(),
	})

	// Login/logout
	router.GET("/login", loginForm, noCSRF, middlewares.CheckOnboardingNotFinished)
	router.POST("/login", login, noCSRF, middlewares.CheckOnboardingNotFinished)
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
	router.POST("/hint", sendHint)

	// Register OAuth clients
	router.POST("/register", registerClient, middlewares.AcceptJSON, middlewares.ContentTypeJSON)
	router.GET("/register/:client-id", readClient, middlewares.AcceptJSON, checkRegistrationToken)
	router.PUT("/register/:client-id", updateClient, middlewares.AcceptJSON, middlewares.ContentTypeJSON, checkRegistrationToken)
	router.DELETE("/register/:client-id", deleteClient)

	// OAuth flow
	authorizeGroup := router.Group("/authorize", noCSRF)
	authorizeGroup.GET("", authorizeForm)
	authorizeGroup.POST("", authorize)
	authorizeGroup.GET("/sharing", authorizeSharingForm)
	authorizeGroup.POST("/sharing", authorizeSharing)

	router.POST("/access_token", accessToken)
	router.POST("/secret_exchange", secretExchange)

	// 2FA
	router.GET("/twofactor", twoFactorForm)
	router.POST("/twofactor", twoFactor)
}
