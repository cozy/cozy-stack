// Package public adds some public routes that can be used to give information
// to anonymous users, or to the not yet authentified cozy owner on its login
// page.
package public

import (
	"net/http"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/pkg/assets"
	"github.com/cozy/cozy-stack/pkg/initials"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/labstack/echo/v4"
)

// Avatar returns the default avatar currently.
func Avatar(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	switch c.QueryParam("fallback") {
	case "404":
		// Nothing
	case "initials":
		publicName, err := inst.PublicName()
		if err != nil {
			publicName = strings.Split(inst.Domain, ".")[0]
		}
		img, mime, err := initials.Image(publicName)
		if err == nil {
			return c.Blob(http.StatusOK, mime, img)
		}
	default:
		f, ok := assets.Get("/images/default-avatar.png", inst.ContextName)
		if ok {
			handler := statik.NewHandler()
			handler.ServeFile(c.Response(), c.Request(), f, true)
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusNotFound, "Page not found")
}

// Prelogin returns information that could be useful to show a login page (like
// in the flagship app).
func Prelogin(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	publicName, err := inst.PublicName()
	if err != nil {
		publicName = ""
	}
	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, echo.Map{
		"name":          publicName,
		"Kdf":           setting.PassphraseKdf,
		"KdfIterations": setting.PassphraseKdfIterations,
	})
}

// Routes sets the routing for the public service
func Routes(router *echo.Group) {
	cacheControl := middlewares.CacheControl(middlewares.CacheOptions{
		MaxAge: 24 * time.Hour,
	})
	router.GET("/avatar", Avatar, cacheControl)
	router.GET("/prelogin", Prelogin)
}
