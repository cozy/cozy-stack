package middlewares

import (
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/gin-gonic/gin"
)

// ErrorHandler returns a gin middleware to handle the errors
func ErrorHandler() gin.HandlerFunc {
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
