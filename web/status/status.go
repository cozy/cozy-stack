// Package status is here just to say that the API is up and that it can
// access the CouchDB databases, for debugging and monitoring purposes.
package status

import (
	"net/http"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/echo"
)

// Status responds with the status of the service
func Status(c echo.Context) error {
	checker := checkup.HTTPChecker{
		Name:     "CouchDB",
		Client:   config.GetConfig().CouchDB.Client,
		URL:      config.CouchURL().String(),
		Attempts: 3,
	}

	var message string
	couchdb, err := checker.Check()
	if err != nil || couchdb.Status() != checkup.Healthy {
		message = "KO"
	} else {
		message = "OK"
	}

	return c.JSON(http.StatusOK, echo.Map{
		"message": message,
		"couchdb": couchdb.Status(),
	})
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	router.GET("", Status)
	router.HEAD("", Status)
	router.GET("/", Status)
	router.HEAD("/", Status)
}
