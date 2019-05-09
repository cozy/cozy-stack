package registry

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/registry"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	"github.com/cozy/echo/middleware"
)

type authType int

const (
	authed authType = iota
	perms
)

func proxyReq(auth authType, clientPermanentCache bool, proxyCacheControl registry.CacheControl) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := middlewares.GetInstance(c)
		switch auth {
		case authed:
			if !middlewares.IsLoggedIn(c) {
				if err := middlewares.AllowWholeType(c, permission.GET, consts.Apps); err != nil {
					return echo.NewHTTPError(http.StatusForbidden)
				}
			}
		case perms:
			pdoc, err := middlewares.GetPermission(c)
			if err != nil || pdoc.Type != permission.TypeWebapp {
				return echo.NewHTTPError(http.StatusForbidden)
			}
		default:
			panic("unknown authType")
		}
		req := c.Request()
		proxyResp, err := registry.Proxy(req, i.Registries(), proxyCacheControl)
		if err != nil {
			return err
		}
		defer proxyResp.Body.Close()
		if clientPermanentCache {
			c.Response().Header().Set("Cache-Control", "max-age=31536000, immutable")
		}
		contentType := proxyResp.Header.Get("content-type")
		return c.Stream(proxyResp.StatusCode, contentType, proxyResp.Body)
	}
}

func proxyListReq(c echo.Context) error {
	i := middlewares.GetInstance(c)
	pdoc, err := middlewares.GetPermission(c)
	if err != nil || pdoc.Type != permission.TypeWebapp {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	req := c.Request()
	list, err := registry.ProxyList(req, i.Registries())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, list)
}

// Routes sets the routing for the registry
func Routes(router *echo.Group) {
	gzip := middleware.Gzip()
	router.GET("", proxyListReq, gzip)
	router.GET("/", proxyListReq, gzip)
	router.GET("/:app", proxyReq(perms, false, registry.WithCache))
	router.GET("/:app/icon", proxyReq(authed, false, registry.NoCache))
	router.GET("/:app/partnership_icon", proxyReq(authed, false, registry.NoCache))
	router.GET("/:app/screenshots/*", proxyReq(authed, false, registry.NoCache))
	router.GET("/:app/:version/icon", proxyReq(authed, true, registry.NoCache))
	router.GET("/:app/:version/partnership_icon", proxyReq(authed, true, registry.NoCache))
	router.GET("/:app/:version/screenshots/*", proxyReq(authed, true, registry.NoCache))
	router.GET("/:app/:version", proxyReq(perms, true, registry.WithCache))
	router.GET("/:app/:channel/latest", proxyReq(perms, false, registry.WithCache))
}
