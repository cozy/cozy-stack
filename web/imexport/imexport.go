package imexport

import (
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/pkg/imexport"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func export(c echo.Context) error {
	w, err := os.Create("cozy.tar.gz")
	if err != nil {
		return err
	}
	defer w.Close()

	instance := middlewares.GetInstance(c)
	fs := instance.VFS()

	err = imexport.Tardir(w, fs)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, echo.Map{
		"message": "bienvenu sur la super page",
	})
}

// Routes sets the routing for export
func Routes(router *echo.Group) {
	router.GET("/", export)
	router.HEAD("/", export)

}
