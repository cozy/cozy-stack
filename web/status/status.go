// Package status is here just to say that the API is up and that it can
// access the CouchDB databases, for debugging and monitoring purposes.
package status

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sourcegraph/checkup"
)

// CouchDBURL is the URL where to check if CouchDB is up
var CouchDBURL = "http://localhost:5984/"

// Status responds OK if the service is running
func Status(c *gin.Context) {
	message := "OK"

	checker := checkup.HTTPChecker{
		Name:     "CouchDB",
		URL:      CouchDBURL,
		Attempts: 3,
	}
	couchdb, _ := checker.Check()
	if couchdb.Status() != checkup.Healthy {
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
