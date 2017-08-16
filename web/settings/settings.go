// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents. For example, it has a route for getting a CSS
// with some CSS variables that can be used as a theme.
package settings

import "github.com/labstack/echo"

// Routes sets the routing for the settings service
func Routes(router *echo.Group) {
	router.GET("/theme.css", ThemeCSS)
	router.GET("/disk-usage", diskUsage)

	router.POST("/passphrase", registerPassphrase)
	router.PUT("/passphrase", updatePassphrase)

	router.GET("/instance", getInstance)
	router.PUT("/instance", updateInstance)

	router.GET("/clients", listClients)
	router.DELETE("/clients/:id", revokeClient)

	router.GET("/onboarded", onboarded)
	router.GET("/context", context)
}
