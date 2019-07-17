// Package status is here just to say that the API is up and that it can
// access the CouchDB databases, for debugging and monitoring purposes.
package status

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/echo"
)

// Status responds with the status of the service
func Status(c echo.Context) error {
	status := "OK"
	couch := "healthy"
	if err := couchdb.CheckStatus(); err != nil {
		status = "KO"
		couch = err.Error()
	}

	code := http.StatusOK
	if status != "OK" {
		code = http.StatusBadGateway
	}
	return c.JSON(code, echo.Map{
		"couchdb": couch,
		"status":  status,
		"message": status, // Legacy, kept for compatibility
	})
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	router.GET("", Status)
	router.HEAD("", Status)
	router.GET("/", Status)
	router.HEAD("/", Status)
}
