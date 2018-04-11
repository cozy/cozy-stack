package middlewares

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/echo"
)

// NeedInstance is an echo middleware which will display an error
// if there is no instance.
func NeedInstance(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Get("instance") != nil {
			return next(c)
		}
		i, err := instance.Get(c.Request().Host)
		if err != nil {
			var errHTTP *echo.HTTPError
			switch err {
			case instance.ErrNotFound:
				errHTTP = echo.NewHTTPError(http.StatusNotFound, err)
			case instance.ErrIllegalDomain:
				errHTTP = echo.NewHTTPError(http.StatusBadRequest, err)
			default:
				errHTTP = echo.NewHTTPError(http.StatusInternalServerError, err)
			}
			errHTTP.Inner = err
			return errHTTP
		}
		c.Set("instance", i)
		return next(c)
	}
}

// GetInstance will return the instance linked to the given echo
// context or panic if none exists
func GetInstance(c echo.Context) *instance.Instance {
	return c.Get("instance").(*instance.Instance)
}

// GetInstanceSafe will return the instance linked to the given echo
// context
func GetInstanceSafe(c echo.Context) (*instance.Instance, bool) {
	i := c.Get("instance")
	if i == nil {
		return nil, false
	}
	inst, ok := i.(*instance.Instance)
	return inst, ok
}
