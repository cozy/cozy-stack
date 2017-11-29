package middlewares

import (
	"github.com/cozy/cozy-stack/pkg/instance"
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
func GetSession(c echo.Context) (s *sessions.Session, ok bool) {
	s, ok = c.Get(sessionKey).(*sessions.Session)
	if ok {
		return
	}
	var i *instance.Instance
	i, ok = GetInstanceSafe(c)
	if !ok {
		return
	}
	var err error
	s, err = sessions.GetSession(c, i)
	ok = err == nil
	if ok {
		c.Set(sessionKey, s)
	}
	return
}
