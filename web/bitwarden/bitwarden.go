// Package bitwarden exposes an API compatible with the Bitwarden Open-Soure apps.
package bitwarden

import (
	"net/http"

	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// Prelogin tells to the client how many KDF iterations it must apply when
// hashing the master password.
func Prelogin(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	return c.JSON(http.StatusOK, echo.Map{
		"Kdf":           inst.PassphraseKdf,
		"KdfIterations": inst.PassphraseKdfIterations,
	})
}

// Routes sets the routing for the Bitwarden-like API
func Routes(router *echo.Group) {
	api := router.Group("/api")
	api.POST("/accounts/prelogin", Prelogin)
}
