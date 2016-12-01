// Package auth provides register and login handlers
package auth

import (
	"net/http"

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
	// TODO: use ParseMultipartForm with a smaller maxMemory (default is 32Mo)
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
	return c.Render(http.StatusOK, "login.html", echo.Map{
		"InvalidPassphrase": false,
	})
}

func login(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if IsLoggedIn(c) {
		return c.Redirect(http.StatusSeeOther, instance.SubDomain(apps.HomeSlug))
	}
	pass := c.FormValue("passphrase")
	if err := instance.CheckPassphrase([]byte(pass)); err != nil {
		return c.Render(http.StatusUnauthorized, "login.html", echo.Map{
			"InvalidPassphrase": true,
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

// IsLoggedIn returns true if the context has a valid session cookie.
func IsLoggedIn(c echo.Context) bool {
	_, err := GetSession(c)
	return err == nil
}

// Routes sets the routing for the status service
func Routes(group *echo.Group) {
	group.POST("/register", register)

	group.GET("/auth/login", loginForm)
	group.POST("/auth/login", login)
	group.DELETE("/auth/login", logout)
}
