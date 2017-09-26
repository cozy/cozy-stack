package registry

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/registry"
	"github.com/cozy/cozy-stack/web/middlewares"
	webpermissions "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

func proxyReq(cacheControl registry.CacheControl) echo.HandlerFunc {
	return func(c echo.Context) error {
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
		resp, err := registry.Proxy(req, registries, cacheControl)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		contentType := resp.Header.Get("content-type")
		return c.Stream(resp.StatusCode, contentType, resp.Body)
	}
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
	router.GET("/:app", proxyReq(registry.WithCache))
	router.GET("/:app/icon", proxyReq(registry.NoCache))
	router.GET("/:app/screenshots/:filename", proxyReq(registry.NoCache))
	router.GET("/:app/:version", proxyReq(registry.WithCache))
	router.GET("/:app/:channel/latest", proxyReq(registry.WithCache))
}
