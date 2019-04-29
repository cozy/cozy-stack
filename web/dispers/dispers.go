// Package data provide simple CRUD operation on couchdb doc
package dispers

import (
	"net/http"

	"github.com/cozy/echo"
  "github.com/cozy/cozy-stack/pkg/dispers"
)

// list every data on which one can train a ML model
func allData(c echo.Context) error {
  return c.JSON(http.StatusCreated, echo.Map{
    "data": dispers.SupportedData,
  })
}

// mostly just to prevent couchdb crash on replications
func dispersAPIWelcome(c echo.Context) error {
	return c.JSON(http.StatusOK, echo.Map{
		"message": dispers.DataSayHello(),
	})
}

// Routes sets the routing for the data service
func Routes(router *echo.Group) {
	router.GET("/", dispersAPIWelcome)
	router.GET("/_all_data", allData)
}
