package middlewares

import (
	"net/http"
	"strconv"
	"strings"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/labstack/echo/v4"
	"github.com/mssola/user_agent"
)

// Some constants for the browser names
const (
	InternetExplorer = "Internet Explorer"
	Edge             = "Edge"
	Firefox          = "Firefox"
	Chrome           = "Chrome"
	Chromium         = "Chromium"
	Opera            = "Opera"
	Safari           = "Safari"
	Android          = "Android"
)

// browser is a struct with a name and a minimal version
type browser struct {
	name       string
	minVersion *int
}

// We don't support Edge before version 17, because some webapps (like Drive)
// needs URLSearchParams.
var minEdgeVersion = 17

// We don't support Firefox before version 52
var minFirefoxVersion = 52

// We don't support Safari before version 11, as window.crypto is not
// available.
var minSafariVersion = 11

var rules = []browser{
	{
		name: InternetExplorer,
	},
	{
		name:       Edge,
		minVersion: &minEdgeVersion,
	},
	{
		name:       Firefox,
		minVersion: &minFirefoxVersion,
	},
	{
		name:       Safari,
		minVersion: &minSafariVersion,
	},
}

// CheckUserAgent is a middleware that shows an HTML page of error when a
// browser that is not supported try to load a webapp.
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

// CryptoPolyfill returns true if the browser can't use its window.crypto API
// to hash the password with PBKDF2. It is the case for Edge, but also for
// Chrome in development mode, because this API is only available in secure
// more (HTTPS).
func CryptoPolyfill(c echo.Context) bool {
	ua := user_agent.New(c.Request().UserAgent())
	browser, _ := ua.Browser()
	if browser == Edge {
		return true
	}
	if build.IsDevRelease() {
		// XXX electron is seen as Safari
		return browser == Chrome || browser == Chromium || browser == Opera || browser == Safari || browser == Android
	}
	return false
}
