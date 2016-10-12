package errors

import (
	"fmt"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/vfs"
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
func HTTPStatus(err error) (code int) {
	switch err {
	case vfs.ErrDocAlreadyExists:
		code = http.StatusConflict
	case vfs.ErrParentDoesNotExist:
		code = http.StatusNotFound
	case vfs.ErrDocDoesNotExist:
		code = http.StatusNotFound
	case vfs.ErrContentLengthInvalid:
		code = http.StatusUnprocessableEntity
	case vfs.ErrInvalidHash:
		code = http.StatusPreconditionFailed
	case vfs.ErrContentLengthMismatch:
		code = http.StatusPreconditionFailed
	}

	if code != 0 {
		return
	}

	if os.IsNotExist(err) {
		code = http.StatusNotFound
	} else if os.IsExist(err) {
		code = http.StatusConflict
	} else if couchErr, isCouchErr := err.(*couchdb.Error); isCouchErr {
		code = couchErr.StatusCode
	} else {
		code = http.StatusInternalServerError
	}

	return
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
