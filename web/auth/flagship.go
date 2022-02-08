package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// CreateSessionCode is the handler for creating a session code by the flagship
// app.
func CreateSessionCode(c echo.Context) error {
	pdoc, err := middlewares.GetPermission(c)
	if err != nil || !pdoc.Permissions.IsMaximal() {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "Not authorized",
		})
	}

	inst := middlewares.GetInstance(c)
	code, err := inst.CreateSessionCode()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	req := c.Request()
	var ip string
	if forwardedFor := req.Header.Get(echo.HeaderXForwardedFor); forwardedFor != "" {
		ip = strings.TrimSpace(strings.SplitN(forwardedFor, ",", 2)[0])
	}
	if ip == "" {
		ip = strings.Split(req.RemoteAddr, ":")[0]
	}
	inst.Logger().WithField("nspace", "loginaudit").
		Infof("New session_code created from %s at %s", ip, time.Now())

	return c.JSON(http.StatusCreated, echo.Map{
		"session_code": code,
	})
}
