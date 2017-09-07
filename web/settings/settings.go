// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents.
package settings

import "github.com/labstack/echo"

// Routes sets the routing for the settings service
func Routes(router *echo.Group) {
	router.GET("/disk-usage", diskUsage)

	router.POST("/passphrase", registerPassphrase)
	router.PUT("/passphrase", updatePassphrase)

	router.GET("/instance", getInstance)
	router.PUT("/instance", updateInstance)

	router.GET("/clients", listClients)
	router.DELETE("/clients/:id", revokeClient)
	router.POST("/synchronized", synchronized)

	router.GET("/onboarded", onboarded)
	router.GET("/context", context)
}
