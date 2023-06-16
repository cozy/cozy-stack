// Package status is here just to say that the API is up and that it can
// access the CouchDB databases, for debugging and monitoring purposes.
package status

import (
	"net/http"
	"sync"

	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/cache"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/labstack/echo/v4"
)

// Status responds with the status of the service
func Status(c echo.Context) error {
	cacheState := "healthy"
	couch := "healthy"
	fs := "healthy"

	latencies := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(3)

	ctx := c.Request().Context()

	go func() {
		if lat, err := cache.CheckStatus(ctx); err == nil {
			mu.Lock()
			latencies["cache"] = lat.String()
			mu.Unlock()
		} else {
			cacheState = err.Error()
		}
		wg.Done()
	}()

	go func() {
		if lat, err := couchdb.CheckStatus(ctx); err == nil {
			mu.Lock()
			latencies["couchdb"] = lat.String()
			mu.Unlock()
		} else {
			couch = err.Error()
		}
		wg.Done()
	}()

	go func() {
		if lat, err := dynamic.CheckStatus(ctx); err == nil {
			mu.Lock()
			latencies["fs"] = lat.String()
			mu.Unlock()
		} else {
			fs = err.Error()
		}
		wg.Done()
	}()

	wg.Wait()
	code := http.StatusOK
	status := "OK"
	if cacheState != "healthy" || couch != "healthy" || fs != "healthy" {
		code = http.StatusBadGateway
		status = "KO"
	}

	return c.JSON(code, echo.Map{
		"cache":   cacheState,
		"couchdb": couch,
		"fs":      fs,
		"status":  status,
		"latency": latencies,
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
