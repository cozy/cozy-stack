package registry

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/registry"
	"github.com/cozy/cozy-stack/web/middlewares"
	webpermissions "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

func proxyReq(c echo.Context) error {
	i := middlewares.GetInstance(c)
	pdoc, err := webpermissions.GetPermission(c)
	if err != nil || pdoc.Type != permissions.TypeWebapp {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	registries, err := i.Registries()
	if err != nil {
		return err
	}
	req := c.Request()
	r, err := registry.Proxy(req, registries, registry.WithCache)
	if err != nil {
		return err
	}
	defer r.Close()
	return c.Stream(http.StatusOK, echo.MIMEApplicationJSON, r)
}

func proxyListReq(c echo.Context) error {
	i := middlewares.GetInstance(c)
	pdoc, err := webpermissions.GetPermission(c)
	if err != nil || pdoc.Type != permissions.TypeWebapp {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	registries, err := i.Registries()
	if err != nil {
		return err
	}
	req := c.Request()
	list, err := registry.ProxyList(req, registries)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, list)
}

// Routes sets the routing for the registry
func Routes(router *echo.Group) {
	router.GET("", proxyListReq)
	router.GET("/", proxyListReq)
	router.GET("/:app", proxyReq)
	router.GET("/:app/:version", proxyReq)
	router.GET("/:app/:channel/latest", proxyReq)
}
