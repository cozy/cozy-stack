package middlewares

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/cozy/echo"
	"github.com/mssola/user_agent"
)

// browser
type browser struct {
	name       string
	minVersion *int
}

// We don't support Edge before version 17, because some webapps (like Drive)
// needs URLSearchParams.
var minEdgeVersion = 17

var rules = []browser{
	{
		name: "Internet Explorer",
	},
	{
		name:       "Edge",
		minVersion: &minEdgeVersion,
	},
}

func CheckUserAgent(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ua := user_agent.New(c.Request().UserAgent())
		browser, rawVersion := ua.Browser()
		acceptHeader := c.Request().Header.Get(echo.HeaderAccept)

		if strings.Contains(acceptHeader, echo.MIMETextHTML) {

			for _, rule := range rules {
				if browser == rule.name {
					version, ok := getMajorVersion(rawVersion)
					if !ok || (rule.minVersion != nil && !(version < *rule.minVersion)) {
						return next(c)
					}

					instance := GetInstance(c)
					return c.Render(http.StatusOK, "compat.html", echo.Map{
						"Domain":      instance.ContextualDomain(),
						"ContextName": instance.ContextName,
						"Locale":      instance.Locale,
						"Favicon":     Favicon(instance),
					})
				}
			}
		}
		return next(c)
	}
}

// getMajorVersion returns the major version of a browser
// 12 => 12
// 12.13 => 12
func getMajorVersion(rawVersion string) (int, bool) {
	splitted := strings.SplitN(rawVersion, ".", 2)
	v, err := strconv.Atoi(splitted[0])
	if err != nil {
		return -1, false
	}
	return v, true
}
