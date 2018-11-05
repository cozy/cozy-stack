package middlewares

import (
	"net/http"
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
				"Domain": instance.ContextualDomain(),
				"Locale": instance.Locale,
			})
		}

		return next(c)
	}
}
