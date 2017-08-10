package middlewares

import (
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/labstack/echo"
)

const loggedInKey = "logged-in"

// LoadSession is a middlewares that sets logged-in to true if the context has
// a valid session cookie.
func LoadSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := GetInstance(c)
		_, err := sessions.GetSession(c, i)
		c.Set(loggedInKey, err == nil)
		return next(c)
	}
}

// IsLoggedIn returns true if the context has a valid session cookie.
func IsLoggedIn(c echo.Context) bool {
	logged, ok := c.Get(loggedInKey).(bool)
	return ok && logged
}
