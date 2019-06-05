package compat

import (
	"net/http"

	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

// Compat display a page with web browsers compatibility informations
func Compat(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	return c.Render(http.StatusOK, "compat.html", echo.Map{
		"ThemeCSS":    middlewares.ThemeCSS(instance),
		"Domain":      instance.ContextualDomain(),
		"ContextName": instance.ContextName,
		"Locale":      instance.Locale,
		"Favicon":     middlewares.Favicon(instance),
	})
}

// Routes sets the routing for the compatibility page
func Routes(router *echo.Group) {
	router.GET("", Compat)
	router.HEAD("", Compat)
	router.GET("/", Compat)
	router.HEAD("/", Compat)
}
