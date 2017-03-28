package intents

import "github.com/labstack/echo"

func createIntent(c echo.Context) error {
	return nil
}

// Routes sets the routing for the intents service
func Routes(router *echo.Group) {
	router.POST("", createIntent)
}
