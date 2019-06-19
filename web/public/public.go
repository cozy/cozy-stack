// Package public adds some public routes that can be used to give information
// to anonymous users, or to the not yet authentified cozy owner on its login
// page.
package public

import (
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/pkg/assets"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/cozy/echo"
)

// Avatar returns the default avatar currently.
func Avatar(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	f, ok := assets.Get("/images/default-avatar.png", inst.ContextName)
	if !ok {
		f, ok = assets.Get("/images/default-avatar.png", "")
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "Page not found")
		}
	}
	handler := statik.NewHandler()
	handler.ServeFile(c.Response(), c.Request(), f, true)
	return nil
}

// Routes sets the routing for the public service
func Routes(router *echo.Group) {
	cacheControl := middlewares.CacheControl(middlewares.CacheOptions{
		MaxAge: 24 * time.Hour,
	})
	router.GET("/avatar", Avatar, cacheControl, middlewares.NeedInstance)
}
