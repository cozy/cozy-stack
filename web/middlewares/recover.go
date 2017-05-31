package middlewares

import (
	"fmt"
	"runtime"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

type (
	// RecoverConfig defines the config for Recover middleware.
	RecoverConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper middleware.Skipper

		// Size of the stack to be printed.
		// Optional. Default value 4KB.
		StackSize int `json:"stack_size"`
	}
)

var (
	// DefaultRecoverConfig is the default Recover middleware config.
	DefaultRecoverConfig = RecoverConfig{
		Skipper:   middleware.DefaultSkipper,
		StackSize: 4 << 10, // 4 KB
	}
)

// Recover returns a middleware which recovers from panics anywhere in the chain
// and handles the control to the centralized HTTPErrorHandler.
func Recover() echo.MiddlewareFunc {
	return RecoverWithConfig(DefaultRecoverConfig)
}

// RecoverWithConfig returns a Recover middleware with config.
// See: `Recover()`.
func RecoverWithConfig(config RecoverConfig) echo.MiddlewareFunc {
	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultRecoverConfig.Skipper
	}
	if config.StackSize == 0 {
		config.StackSize = DefaultRecoverConfig.StackSize
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
					stack := make([]byte, config.StackSize)
					length := runtime.Stack(stack, false)
					log := logger.WithDomain(c.Request().Host).WithField("panic", true)
					log.Errorf("PANIC RECOVER %s: %s", err.Error(), stack[:length])
					c.Error(err)
				}
			}()
			return next(c)
		}
	}
}
