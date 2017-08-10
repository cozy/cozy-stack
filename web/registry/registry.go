package registry

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/registry"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func proxyReq(c echo.Context) error {
	i := middlewares.GetInstance(c)
	registries, err := i.Registries()
	if err != nil {
		return err
	}
	req := c.Request()
	r, err := registry.Proxy(req, registries)
	if err != nil {
		return err
	}
	defer r.Close()
	return c.Stream(http.StatusOK, echo.MIMEApplicationJSON, r)
}

func Routes(router *echo.Group) {
	router.GET("/", proxyReq)
	router.GET("/:app", proxyReq)
	router.GET("/:app/:version", proxyReq)
	router.GET("/:app/:channel/latest", proxyReq)
}
