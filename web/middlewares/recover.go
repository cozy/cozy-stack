package middlewares

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// RecoverConfig defines the config for Recover middleware.
type RecoverConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper middleware.Skipper

	// Size of the stack to be printed.
	// Optional. Default value 4KB.
	StackSize int `json:"stack_size"`
}

// RecoverWithConfig returns a Recover middleware with config.
func RecoverWithConfig(config RecoverConfig) echo.MiddlewareFunc {
	// Defaults
	if config.Skipper == nil {
		config.Skipper = middleware.DefaultSkipper
	}
	if config.StackSize == 0 {
		config.StackSize = 4 << 10 // 4 KB
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			defer func() {
				if r := recover(); r != nil {
					var err error
					switch r := r.(type) {
					case error:
						err = r
					default:
						err = fmt.Errorf("%v", r)
					}
					// We don't want to log panic with ErrAbortHandler, as it
					// is just noise (http.Server does that too).
					// See https://golang.org/pkg/net/http/#ErrAbortHandler
					if err != http.ErrAbortHandler {
						stack := make([]byte, config.StackSize)
						length := runtime.Stack(stack, false)
						log := logger.WithDomain(c.Request().Host).WithField("panic", true)
						log.Errorf("PANIC RECOVER %s: %s", err.Error(), stack[:length])
						c.Error(err)
					}
				}
			}()
			return next(c)
		}
	}
}
