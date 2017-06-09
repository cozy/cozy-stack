package imexport

import (
	"fmt"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/pkg/imexport"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func export(c echo.Context) error {
	fmt.Println("EXPORT")
	w, err := os.Create("cozy.tar.gz")
	if err != nil {
		return err
	}
	defer w.Close()

	fmt.Println("INSTANCE")
	instance := middlewares.GetInstance(c)
	fs := instance.VFS()

	fmt.Println("TARDIR")
	err = imexport.Tardir(w, fs)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, echo.Map{
		"message": "bienvenue sur la super page",
	})

}

// Routes sets the routing for export
func Routes(router *echo.Group) {
	fmt.Println("ROUTE")
	router.GET("/", export)
	router.HEAD("/", export)

}
