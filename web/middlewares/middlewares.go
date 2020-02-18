package middlewares

import (
	"github.com/labstack/echo/v4"
)

// Compose can be used to compose a list of middlewares together with a main
// handler function. It returns a new handler that should be the composition of
// all the middlwares with the initial handler.
func Compose(handler echo.HandlerFunc, mws ...echo.MiddlewareFunc) echo.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}
