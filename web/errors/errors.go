package errors

import (
	"fmt"
	"net/http"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/labstack/echo"
)

// ErrorHandler is the default error handler of our server. It always write a
// jsonapi compatible error.
func ErrorHandler(err error, c echo.Context) {
	var je *jsonapi.Error
	var ce *couchdb.Error
	var he *echo.HTTPError
	var ok bool

	res := c.Response()
	req := c.Request()

	if he, ok = err.(*echo.HTTPError); ok {
		if !res.Committed {
			if c.Request().Method == http.MethodHead {
				c.NoContent(he.Code)
			} else {
				c.String(he.Code, fmt.Sprintf("%v", he.Message))
			}
		}
		if config.IsDevRelease() {
			log.Errorf("[HTTP %s %s] %s", req.Method, req.URL.Path, err)
		}
		return
	}

	if os.IsExist(err) {
		je = jsonapi.Conflict(err)
	} else if os.IsNotExist(err) {
		je = jsonapi.NotFound(err)
	} else if ce, ok = err.(*couchdb.Error); ok {
		je = &jsonapi.Error{
			Status: ce.StatusCode,
			Title:  ce.Name,
			Detail: ce.Reason,
		}
	} else if je, ok = err.(*jsonapi.Error); !ok {
		je = &jsonapi.Error{
			Status: http.StatusInternalServerError,
			Title:  "Unqualified error",
			Detail: err.Error(),
		}
	}

	if !res.Committed {
		if c.Request().Method == http.MethodHead {
			c.NoContent(je.Status)
		} else {
			jsonapi.DataError(c, je)
		}
	}

	if config.IsDevRelease() {
		log.Errorf("[HTTP %s %s] %s", req.Method, req.URL.Path, err)
	}
}
