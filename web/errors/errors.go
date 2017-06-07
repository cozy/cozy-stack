package errors

import (
	"fmt"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
)

// ErrorHandler is the default error handler of our server. It always write a
// jsonapi compatible error.
func ErrorHandler(err error, c echo.Context) {
	var je *jsonapi.Error
	var ce *couchdb.Error
	var he *echo.HTTPError
	var ok bool

	var log *logrus.Entry
	inst, ok := c.Get("instance").(*instance.Instance)
	if ok {
		log = inst.Logger()
	} else {
		log = logger.WithNamespace("http")
	}

	res := c.Response()
	req := c.Request()

	if he, ok = err.(*echo.HTTPError); ok {
		// #nosec
		if !res.Committed {
			if c.Request().Method == http.MethodHead {
				c.NoContent(he.Code)
			} else {
				c.String(he.Code, fmt.Sprintf("%v", he.Message))
			}
		}
		if config.IsDevRelease() {
			log.Errorf("[http] %s %s %s", req.Method, req.URL.Path, err)
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

	// #nosec
	if !res.Committed {
		if c.Request().Method == http.MethodHead {
			c.NoContent(je.Status)
		} else {
			jsonapi.DataError(c, je)
		}
	}

	if config.IsDevRelease() {
		log.Errorf("[http] %s %s %s", req.Method, req.URL.Path, err)
	}
}
