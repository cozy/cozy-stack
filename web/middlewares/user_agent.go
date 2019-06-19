package middlewares

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/cozy/echo"
	"github.com/mssola/user_agent"
)

// CheckIE checks if the browser is IE
func CheckIE(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ua := user_agent.New(c.Request().UserAgent())
		browser, _ := ua.Browser()

		acceptHeader := c.Request().Header.Get(echo.HeaderAccept)

		if strings.Contains(acceptHeader, echo.MIMETextHTML) &&
			browser == "Internet Explorer" {
			instance := GetInstance(c)
			return c.Render(http.StatusOK, "compat.html", echo.Map{
				"Domain":      instance.ContextualDomain(),
				"ContextName": instance.ContextName,
				"Locale":      instance.Locale,
				"Favicon":     Favicon(instance),
			})
		}

		return next(c)
	}
}

// CheckEdge checks if the browser is Edge
func CheckEdge(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		minSupportedVersion := 17 // We support Edge versions >= 17

		ua := user_agent.New(c.Request().UserAgent())
		browser, rawVersion := ua.Browser()

		acceptHeader := c.Request().Header.Get(echo.HeaderAccept)

		// If we cannot parse the Edge version, do not block
		v, ok := parseEdgeVersion(rawVersion)
		if !ok {
			return next(c)
		}

		if strings.Contains(acceptHeader, echo.MIMETextHTML) &&
			browser == "Edge" && v < minSupportedVersion {
			instance := GetInstance(c)
			return c.Render(http.StatusOK, "compat.html", echo.Map{
				"Domain":      instance.ContextualDomain(),
				"ContextName": instance.ContextName,
				"Locale":      instance.Locale,
				"Favicon":     Favicon(instance),
			})
		}

		return next(c)
	}
}

// parseEdgeVersion returns the version of Edge
func parseEdgeVersion(rawVersion string) (int, bool) {
	splitted := strings.SplitN(rawVersion, ".", 2)
	v, err := strconv.Atoi(splitted[0])
	if err != nil {
		return -1, false
	}
	return v, true
}
