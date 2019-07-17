// Package status is here just to say that the API is up and that it can
// access the CouchDB databases, for debugging and monitoring purposes.
package status

import (
	"net/http"
	"sync"

	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/echo"
)

// Status responds with the status of the service
func Status(c echo.Context) error {
	cache := "healthy"
	couch := "healthy"
	fs := "healthy"

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		cfg := config.GetConfig()
		if err := cfg.CacheStorage.CheckStatus(); err != nil {
			cache = err.Error()
		}
		wg.Done()
	}()

	go func() {
		if err := couchdb.CheckStatus(); err != nil {
			couch = err.Error()
		}
		wg.Done()
	}()

	go func() {
		if err := dynamic.CheckStatus(); err != nil {
			fs = err.Error()
		}
		wg.Done()
	}()

	wg.Wait()
	code := http.StatusOK
	status := "OK"
	if cache != "healthy" || couch != "healthy" || fs != "healthy" {
		code = http.StatusBadGateway
		status = "KO"
	}

	return c.JSON(code, echo.Map{
		"cache":   cache,
		"couchdb": couch,
		"fs":      fs,
		"status":  status,
		"message": status, // Legacy, kept for compatibility
	})
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	router.GET("", Status)
	router.HEAD("", Status)
	router.GET("/", Status)
	router.HEAD("/", Status)
}
