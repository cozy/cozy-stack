package middlewares

import (
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/echo"
)

const sessionKey = "session"

// LoadSession is a middlewares that loads the session and stores it the
// request context.
func LoadSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i, ok := GetInstanceSafe(c)
		if ok {
			session, err := sessions.FromCookie(c, i)
			if err == nil {
				c.Set(sessionKey, session)
			}
		}
		return next(c)
	}
}

// LoadAppSession is a middlewares that loads the session, from an application
// subdmail, and stores it the request context.
func LoadAppSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i, ok := GetInstanceSafe(c)
		if ok {
			slug := c.Get("slug").(string)
			session, err := sessions.FromAppCookie(c, i, slug)
			if err == nil {
				c.Set(sessionKey, session)
			}
		}
		return next(c)
	}
}

// IsLoggedIn returns true if the context has a valid session cookie.
func IsLoggedIn(c echo.Context) bool {
	_, ok := GetSession(c)
	return ok
}

// GetSession returns the sessions associated with the given context.
func GetSession(c echo.Context) (session *sessions.Session, ok bool) {
	v := c.Get(sessionKey)
	if v != nil {
		session, ok = v.(*sessions.Session)
	}
	return session, ok
}
