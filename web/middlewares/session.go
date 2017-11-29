package middlewares

import (
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/labstack/echo"
)

const sessionKey = "session"

// IsLoggedIn returns true if the context has a valid session cookie.
func IsLoggedIn(c echo.Context) bool {
	_, ok := GetSession(c)
	return ok
}

// GetSession returns the sessions associated with the given context.
func GetSession(c echo.Context) (*sessions.Session, bool) {
	if session, ok := c.Get(sessionKey).(*sessions.Session); ok {
		return session, true
	}
	i, ok := GetInstanceSafe(c)
	if !ok {
		return nil, false
	}
	session, err := sessions.FromCookie(c, i)
	if err != nil {
		return nil, false
	}
	c.Set(sessionKey, session)
	return session, true
}
