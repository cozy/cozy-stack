package middlewares

import (
	"github.com/cozy/cozy-stack/instance"
	"github.com/labstack/echo"
)

// NeedInstance is a echo middleware which will display an error
// if there is no instance.
func NeedInstance(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Get("instance") != nil {
			return next(c)
		}
		i, err := instance.Get(c.Request().Host)
		if err != nil {
			return err
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
