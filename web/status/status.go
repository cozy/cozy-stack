// Package status is here just to say that the API is up and that it can
// access the CouchDB databases, for debugging and monitoring purposes.
package status

import (
	"net/http"

	"github.com/cozy/cozy-stack/config"
	"github.com/gin-gonic/gin"
	"github.com/cozy/cozy-stack/checkup"
)

// Status responds with the status of the service
//
// swagger:route GET /status status showStatus
//
// It responds OK if the service is running
func Status(c *gin.Context) {
	message := "OK"

	checker := checkup.HTTPChecker{
		Name:     "CouchDB",
		URL:      config.CouchURL(),
		Attempts: 3,
	}
	couchdb, err := checker.Check()
	if err != nil || couchdb.Status() != checkup.Healthy {
		message = "KO"
	}

	c.JSON(http.StatusOK, gin.H{
		"message": message,
		"couchdb": couchdb.Status(),
	})
}

// Routes sets the routing for the status service
func Routes(router *gin.RouterGroup) {
	router.GET("/", Status)
}
