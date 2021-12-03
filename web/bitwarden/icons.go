package bitwarden

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/pkg/assets"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/labstack/echo/v4"
)

func serveDefaultIcon(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	f, ok := assets.Get("/images/default-bitwarden-icon.png", inst.ContextName)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "Page not found")
	}
	handler := statik.NewHandler()
	handler.ServeFile(c.Response(), c.Request(), f, true)
	return nil
}

// GetIcon returns an icon for the given domain, to be used by the bitwarden
// clients.
func GetIcon(c echo.Context) error {
	domain := c.Param("domain")
	ico, err := bitwarden.GetIcon(domain)
	if err == nil {
		return c.Blob(http.StatusOK, ico.Mime, ico.Body)
	}

	inst := middlewares.GetInstance(c)
	inst.Logger().WithNamespace("bitwarden").Debugf("Error for icon %s: %s", domain, err)
	if c.QueryParam("fallback") == "404" {
		return echo.NewHTTPError(http.StatusNotFound, "Page not found")
	}
	return serveDefaultIcon(c)
}
