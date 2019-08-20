package instances

import (
	"errors"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/labstack/echo/v4"
)

func getDebug(c echo.Context) error {
	domain := c.Param("domain")
	log := logger.WithDomain(domain)
	if !logger.IsDebug(log) {
		return jsonapi.NotFound(errors.New("Debug is disabled on this domain"))
	}
	res := map[string]bool{domain: true}
	return c.JSON(http.StatusOK, res)
}

func enableDebug(c echo.Context) error {
	domain := c.Param("domain")
	ttl, err := time.ParseDuration(c.QueryParam("TTL"))
	if err != nil {
		ttl = 24 * time.Hour
	}
	if err := logger.AddDebugDomain(domain, ttl); err != nil {
		return wrapError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func disableDebug(c echo.Context) error {
	domain := c.Param("domain")
	if err := logger.RemoveDebugDomain(domain); err != nil {
		return wrapError(err)
	}
	return c.NoContent(http.StatusNoContent)
}
