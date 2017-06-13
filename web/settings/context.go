package settings

import (
	"net/http"

	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func onboarded(c echo.Context) error {
	i := middlewares.GetInstance(c)
	if !middlewares.IsLoggedIn(c) {
		return c.Redirect(http.StatusSeeOther, i.PageURL("/auth/login", nil))
	}
	redirect := i.OnboardedRedirection().String()
	return c.Redirect(http.StatusSeeOther, redirect)
}
