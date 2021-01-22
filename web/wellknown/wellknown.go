package wellknown

import (
	"net/http"

	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// ChangePassword is an handler that redirects to the settings page that can be
// used by a user to change their password.
// See https://w3c.github.io/webappsec-change-password-url/
func ChangePassword(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	return c.Redirect(http.StatusFound, inst.ChangePasswordURL())
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	router.GET("/change-password", ChangePassword)
	router.HEAD("/change-password", ChangePassword)
}
