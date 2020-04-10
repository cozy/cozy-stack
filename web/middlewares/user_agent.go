package middlewares

import (
	"net/http"
	"strconv"
	"strings"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/labstack/echo/v4"
	"github.com/mssola/user_agent"
)

const maxInt = int(^uint(0) >> 1)

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
	Electron         = "Electron"
)

type iPhoneState int

const (
	iPhoneOrNotIPhone iPhoneState = iota
	notIphone
	onlyIphone
)

// browserRule is a rule for saying which browser is accepted based on the
// user-agent.
type browserRule struct {
	name       string
	iPhone     iPhoneState
	minVersion int
}

func (rule *browserRule) canApply(browser string, iPhone bool) bool {
	if rule.name != browser {
		return false
	}
	if rule.iPhone == notIphone && iPhone {
		return false
	}
	if rule.iPhone == onlyIphone && !iPhone {
		return false
	}
	return true
}

func (rule *browserRule) acceptVersion(rawVersion string) bool {
	version, ok := getMajorVersion(rawVersion)
	if !ok {
		return true
	}
	return version >= rule.minVersion
}

var rules = []browserRule{
	// We don't support IE
	{
		name:       InternetExplorer,
		iPhone:     iPhoneOrNotIPhone,
		minVersion: maxInt,
	},
	// We don't support Edge before version 17, because some webapps (like Drive)
	// needs URLSearchParams.
	{
		name:       Edge,
		iPhone:     iPhoneOrNotIPhone,
		minVersion: 17,
	},
	// We don't support Firefox before version 52, except on iOS where the
	// webkit engine is used on the version numbers are not the same.
	{
		name:       Firefox,
		iPhone:     notIphone,
		minVersion: 52,
	},
	{
		name:       Firefox,
		iPhone:     onlyIphone,
		minVersion: 7, // Firefox Focus has a lower version number than Firefox for iOS
	},
	// We don't support Safari before version 11, as window.crypto is not
	// available.
	{
		name:       Safari,
		iPhone:     iPhoneOrNotIPhone,
		minVersion: 11,
	},
}

// CheckUserAgent is a middleware that shows an HTML page of error when a
// browser that is not supported try to load a webapp.
func CheckUserAgent(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ua := user_agent.New(c.Request().UserAgent())
		browser, rawVersion := ua.Browser()
		iPhone := ua.Platform() == "iPhone"
		acceptHeader := c.Request().Header.Get(echo.HeaderAccept)

		if strings.Contains(acceptHeader, echo.MIMETextHTML) {
			for _, rule := range rules {
				if rule.canApply(browser, iPhone) {
					if rule.acceptVersion(rawVersion) {
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
// to hash the password with PBKDF2. It is the case for Edge, but also for most
// browsers in development mode, because this API is only available in secure
// more (HTTPS).
func CryptoPolyfill(c echo.Context) bool {
	ua := user_agent.New(c.Request().UserAgent())
	browser, _ := ua.Browser()
	if browser == Edge {
		return true
	}
	return build.IsDevRelease()
}
