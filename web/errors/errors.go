package errors

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/gin-gonic/gin"
)

// NoInstance is the err to be returned when there is no instance
var NoInstance = &gin.Error{
	Err: fmt.Errorf("Cannot find instance for request"),
	Meta: gin.H{
		"error":  "internal_server_error",
		"reason": "Cannot find instance for request",
	},
}

// HTTPStatus gives the http status for given error
func HTTPStatus(err error) int {
	if coucherr, iscoucherr := err.(*couchdb.Error); iscoucherr {
		return coucherr.StatusCode
	}
	return http.StatusInternalServerError
}

// Handler returns a gin middleware to handle the errors
func Handler() gin.HandlerFunc {
	return func(c *gin.Context) {

		// let the controller do its thing
		c.Next()

		errors := c.Errors.ByType(gin.ErrorTypeAny)
		if len(errors) > 0 {
			ginerr := errors.Last()
			if coucherr, iscoucherr := ginerr.Err.(*couchdb.Error); iscoucherr {
				c.JSON(-1, coucherr.JSON())
			} else {
				c.JSON(-1, ginerr.JSON())
			}
		}
	}
}

// InvalidDoctype : the passed doctype is not valid
func InvalidDoctype(doctype string) error {
	return fmt.Errorf("Invalid doctype '%s'", doctype)
}
