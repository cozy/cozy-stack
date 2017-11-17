package middlewares

import (
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/labstack/echo"
)

const sessionKey = "session"

// LoadSession is a middlewares that sets logged-in to true if the context has
// a valid session cookie.
func LoadSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := GetInstance(c)
		s, err := sessions.GetSession(c, i)
		if err == nil {
			c.Set(sessionKey, s)
		}
		return next(c)
	}
}

// IsLoggedIn returns true if the context has a valid session cookie.
func IsLoggedIn(c echo.Context) bool {
	_, ok := c.Get(sessionKey).(*sessions.Session)
	return ok
}

// GetSession returns the sessions associated with the given context.
func GetSession(c echo.Context) (*sessions.Session, bool) {
	s, ok := c.Get(sessionKey).(*sessions.Session)
	if !ok {
		var err error
		i := GetInstance(c)
		s, err = sessions.GetSession(c, i)
		if err == nil {
			ok = true
			c.Set(sessionKey, s)
		}
	}
	return s, ok
}
