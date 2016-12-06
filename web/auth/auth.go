// Package auth provides register and login handlers
package auth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/apps"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

func redirectSuccessLogin(c echo.Context, slug string) error {
	instance := middlewares.GetInstance(c)
	session, err := NewSession(instance)
	if err != nil {
		return jsonapi.InternalServerError(err)
	}
	c.SetCookie(session.ToCookie())
	return c.Redirect(http.StatusSeeOther, instance.SubDomain(slug))
}

func register(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	passphrase := []byte(c.FormValue("passphrase"))
	registerToken := []byte(c.FormValue("registerToken"))
	if err := instance.RegisterPassphrase(passphrase, registerToken); err != nil {
		return jsonapi.BadRequest(err)
	}

	return redirectSuccessLogin(c, apps.OnboardingSlug)
}

func loginForm(c echo.Context) error {
	if IsLoggedIn(c) {
		instance := middlewares.GetInstance(c)
		return c.Redirect(http.StatusSeeOther, instance.SubDomain(apps.HomeSlug))
	}

	redirect, err := checkRedirectParam(c)
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "login.html", echo.Map{
		"InvalidPassphrase": false,
		"HasRedirect":       redirect != "",
		"Redirect":          redirect,
	})
}

func login(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if IsLoggedIn(c) {
		return c.Redirect(http.StatusSeeOther, instance.SubDomain(apps.HomeSlug))
	}

	redirect, err := checkRedirectParam(c)
	if err != nil {
		return err
	}

	pass := c.FormValue("passphrase")
	if err := instance.CheckPassphrase([]byte(pass)); err != nil {
		return c.Render(http.StatusUnauthorized, "login.html", echo.Map{
			"InvalidPassphrase": true,
			"HasRedirect":       redirect != "",
			"Redirect":          redirect,
		})
	}

	return redirectSuccessLogin(c, apps.HomeSlug)
}

func logout(c echo.Context) error {
	// TODO check that a valid CtxToken is given to protect against CSRF attacks
	instance := middlewares.GetInstance(c)
	session, err := GetSession(c)
	if err == nil {
		c.SetCookie(session.Delete(instance))
	}
	return c.Redirect(http.StatusSeeOther, instance.PageURL("/auth/login"))
}

// checkRedirectParam returns the optional redirect query parameter. If not
// empty, we check that the redirect is a subdomain of the cozy-instance.
func checkRedirectParam(c echo.Context) (string, error) {
	redirect := c.FormValue("redirect")
	if redirect == "" {
		return "", nil
	}

	u, err := url.Parse(redirect)
	if err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest,
			"bad url: could not parse")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", echo.NewHTTPError(http.StatusBadRequest,
			"bad url: bad scheme")
	}

	instance := middlewares.GetInstance(c)
	parts := strings.SplitN(u.Host, ".", 2)
	if len(parts) != 2 || parts[1] != instance.Domain {
		return "", echo.NewHTTPError(http.StatusBadRequest,
			"bad url: should be subdomain")
	}

	// to protect against stealing authorization code with redirection, the
	// fragment is always overriden.
	u.Fragment = ""

	return u.String(), nil
}

// IsLoggedIn returns true if the context has a valid session cookie.
func IsLoggedIn(c echo.Context) bool {
	_, err := GetSession(c)
	return err == nil
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	router.POST("/register", register)

	router.GET("/auth/login", loginForm)
	router.POST("/auth/login", login)
	router.DELETE("/auth/login", logout)
}
