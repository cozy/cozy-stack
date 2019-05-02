// Package data provide simple CRUD operation on couchdb doc
package dispers

import (
	"net/http"

	"github.com/cozy/echo"
  "github.com/cozy/cozy-stack/pkg/dispers"
	"github.com/cozy/cozy-stack/pkg/instance/lifecycle"
)

// list every data on which one can train a ML model
func allData(c echo.Context) error {
  return c.JSON(http.StatusCreated, echo.Map{
    "data": dispers.SupportedData,
  })
}

func showSubscription(c echo.Context) error {
	domain := c.Param("domain")

	instance, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}

	prefix := instance.DBPrefix()
	liste := dispers.GetSubscriptions(domain, prefix)

	// fonction dans pkg pour récupérer la liste des inscriptions

	return c.JSON(http.StatusOK, echo.Map{"prefix": prefix,
																				"subscription": liste})
}

//func subscribe(c echo.Context)

/*func dispersAPIWelcome(c echo.Context) error {
	return c.JSON(http.StatusOK, echo.Map{
		"message": dispers.DataSayHello(),
	})
}*/

func Index(c echo.Context) error {
		return c.String(http.StatusOK,"Hello ! You reach correctly the learning part of the Cozy stack.")
}

func CIndex(c echo.Context) error {
    return c.String(http.StatusOK, "Hello ! I'm ruling the game !")
}

func CIIndex(c echo.Context) error {
    return c.String(http.StatusOK, "Hello ! I will do Concept Indexor's dishes !")
}

func DIndex(c echo.Context) error {
    return c.String(http.StatusOK, "Hello ! I will do Data's dishes !")
}

func DAIndex(c echo.Context) error {
    return c.String(http.StatusOK, "Hello ! I will do Data Aggregator's dishes !")
}

func TFIndex(c echo.Context) error {
    return c.String(http.StatusOK, "Hello ! I will do Target Finder's dishes !")
}

// Routes sets the routing for the dispers service
func Routes(router *echo.Group) {
	router.GET("/", Index)

	router.GET("/conceptindexor", CIIndex)
	//router.GET("/conceptindexor/hash", getHashConcept)
	//router.POST("/conceptindexor/hash", hashConcept)

	router.GET("/conductor", CIndex)
	router.GET("/conductor/subscribe/_all_data", allData)
	router.GET("/conductor/:domain/subscription", showSubscription)
	//router.POST("/conductor/:domain/subscription", subscribe)
	//router.DELETE("/conductor/:domain/subscription", unsubscribe)
	//router.GET("/conductor/training", showTraining)
	//router.POST("/conductor/training", createTraining)
	//router.DELETE("/conductor/training", deleteTraining)

	router.GET("/data", DIndex)
	//router.GET("/data/collect", DIndex)
	//router.POST("/data/collect", CIIndex)

	router.GET("/dataaggregator", DAIndex)
	//router.POST("/dataaggregator/aggregate", CIIndex)

	router.GET("/targetfinder", TFIndex)
	//router.GET("/targetfinder/adresses", CIIndex)
	//router.POST("/targetfinder/adresses", CIIndex)
}
