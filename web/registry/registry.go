package registry

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/registry"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type authType int

const (
	authed authType = iota
	perms
)

type clientCacheControl int

const (
	noClientCache clientCacheControl = iota
	shortClientCache
	permanentClientCache
)

func proxyReq(auth authType, clientCache clientCacheControl, proxyCacheControl registry.CacheControl) echo.HandlerFunc {
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
			if err != nil {
				return echo.NewHTTPError(http.StatusForbidden)
			}
			if pdoc.Type != permission.TypeWebapp && pdoc.Type != permission.TypeOauth {
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
		switch clientCache {
		case permanentClientCache:
			c.Response().Header().Set("Cache-Control", "max-age=31536000, immutable")
		case shortClientCache:
			c.Response().Header().Set("Cache-Control", "max-age=86400")
		}
		contentType := proxyResp.Header.Get("content-type")
		return c.Stream(proxyResp.StatusCode, contentType, proxyResp.Body)
	}
}

func proxyListReq(c echo.Context) error {
	i := middlewares.GetInstance(c)
	pdoc, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if pdoc.Type != permission.TypeWebapp && pdoc.Type != permission.TypeOauth {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	req := c.Request()
	list, err := registry.ProxyList(req, i.Registries())
	if err != nil {
		return err
	}
	maintenance, err := app.ListMaintenance()
	if err != nil {
		return err
	}
	for _, app := range list.Apps {
		slug, _ := app["slug"].(string)
		for _, item := range maintenance {
			if item["slug"] == slug {
				app["maintenance_activated"] = true
				app["maintenance_options"] = item["maintenance_options"]
			}
		}
	}
	return c.JSON(http.StatusOK, list)
}

func proxyAppReq(c echo.Context) error {
	i := middlewares.GetInstance(c)
	pdoc, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if pdoc.Type != permission.TypeWebapp && pdoc.Type != permission.TypeOauth {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	req := c.Request()
	proxyResp, err := registry.Proxy(req, i.Registries(), registry.WithCache)
	if err != nil {
		return err
	}
	defer proxyResp.Body.Close()
	var doc map[string]interface{}
	if err := json.NewDecoder(proxyResp.Body).Decode(&doc); err != nil {
		return err
	}
	opts, err := app.GetMaintenanceOptions(c.Param("app"))
	if err != nil {
		return err
	}
	if opts != nil {
		doc["maintenance_activated"] = true
		doc["maintenance_options"] = opts
	}
	return c.JSON(http.StatusOK, doc)
}

func proxyMaintenanceReq(c echo.Context) error {
	i := middlewares.GetInstance(c)
	pdoc, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if pdoc.Type != permission.TypeWebapp && pdoc.Type != permission.TypeOauth {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	req := c.Request()
	apps, err := registry.ProxyMaintenance(req, i.Registries())
	if err != nil {
		return err
	}
	maintenance, err := app.ListMaintenance()
	if err != nil {
		return err
	}
	list := make([]interface{}, 0, len(apps)+len(maintenance))
	for _, item := range maintenance {
		list = append(list, item)
	}
	for _, item := range apps {
		list = append(list, item)
	}
	return c.JSON(http.StatusOK, list)
}

// Routes sets the routing for the registry
func Routes(router *echo.Group) {
	gzip := middleware.Gzip()
	router.GET("", proxyListReq, gzip)
	router.GET("/", proxyListReq, gzip)
	router.GET("/maintenance", proxyMaintenanceReq, gzip)
	router.GET("/:app", proxyAppReq, gzip)
	router.GET("/:app/icon", proxyReq(authed, shortClientCache, registry.NoCache))
	router.GET("/:app/partnership_icon", proxyReq(authed, shortClientCache, registry.NoCache))
	router.GET("/:app/screenshots/*", proxyReq(authed, shortClientCache, registry.NoCache))
	router.GET("/:app/:version/icon", proxyReq(authed, permanentClientCache, registry.NoCache))
	router.GET("/:app/:version/partnership_icon", proxyReq(authed, permanentClientCache, registry.NoCache))
	router.GET("/:app/:version/screenshots/*", proxyReq(authed, permanentClientCache, registry.NoCache))
	router.GET("/:app/:version", proxyReq(perms, permanentClientCache, registry.WithCache))
	router.GET("/:app/:channel/latest", proxyReq(perms, noClientCache, registry.WithCache))
}
