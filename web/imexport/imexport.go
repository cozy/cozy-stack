package imexport

import (
	"net/http"

	"github.com/cozy/echo"
)

func imexport(c echo.Context) error {
	return c.JSON(http.StatusOK, echo.Map{
		"message": "bienvenu sur la super page",
	})
}

// Routes sets the routing for export
func Routes(router *echo.Group) {
	router.GET("/", imexport)
	router.HEAD("/", imexport)

}
