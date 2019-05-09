// Package data provide simple CRUD operation on couchdb doc
package dispers

import (
	"net/http"
	"encoding/json"

	"github.com/cozy/echo"
  "github.com/cozy/cozy-stack/pkg/dispers"
  "github.com/cozy/cozy-stack/pkg/dispers/utils"
  "github.com/cozy/cozy-stack/pkg/prefixer"
  "github.com/cozy/cozy-stack/pkg/couchdb"
	//"github.com/cozy/cozy-stack/pkg/instance/lifecycle"
)

func Index(c echo.Context) error {

		out := "Hello ! You reach correctly the learning part of the Cozy Learning Server."
		return c.String(http.StatusOK, out)
}

func IndexBis(c echo.Context) error {

		out := ""
		actor := c.Param("actor");
		switch  actor {
			case "conductor":
				out = "Hello ! I'm ruling the game !"
			case "conceptindexor":
				out = "Hello ! I will do Concept Indexor's dishes !"
			case "target":
				out = "Hello ! I will do Target's dishes !"
			case "dataaggregator":
				out = "Hello ! I will do Data Aggregator's dishes !"
			case "targetfinder":
				out = "Hello ! I will do Target Finder's dishes !"
			default:
				return nil
		}

		return c.String(http.StatusOK, out)
}

func allData(c echo.Context) error {

	var supportedData = []string{
		"iris", "bank.label",
	}

  return c.JSON(http.StatusCreated, echo.Map{
    "data": supportedData,
  })
}

func showSubscription(c echo.Context) error {
	/*domain := c.Param("domain")

	instance, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}
	prefix := instance.DBPrefix()

	liste := dispers.GetSubscriptions(domain, prefix)
	// fonction dans pkg pour récupérer la liste des inscriptions
	return c.JSON(http.StatusOK, echo.Map{"prefix": prefix,
																				"subscription": liste})
	*/
	return nil
}

func showTraining(c echo.Context) error {
	// TODO : Prévoir sûrement un token pour mettre des droits d'accès
	id := c.Param("idtrain")
	return c.JSON(http.StatusOK, dispers.GetTrainingState(id))
}

func createTraining(c echo.Context) error {

	var mytraining dispers.Training

	if err := json.NewDecoder(c.Request().Body).Decode(&mytraining); err != nil {
		return c.JSON(http.StatusOK, echo.Map{"outcome": "error"})
	}

	mytraining.State = "Training"
	querydoc := dispers.NewQueryDoc("", "", mytraining, utils.NewMetadata("creation", "creation du training", "aujourd'hui", true))

	if err := couchdb.CreateDoc(prefixer.ConductorPrefixer, querydoc); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, echo.Map{
		"ok":   true,
		"id":   querydoc.ID(),
		"rev":  querydoc.Rev(),
		"type": querydoc.DocType(),
	})
}

func hashConcept(c echo.Context) error {

	concept := c.Param("concept")
	return c.JSON(http.StatusCreated, dispers.HashMeThat(concept))
}

func addConcept(c echo.Context) error {

	concept := c.Param("concept")
	return c.JSON(http.StatusCreated, dispers.AddConcept(concept))
}

func selectAdresses(c echo.Context) error {

	var listsOfAdresses dispers.InputTF

	if err := json.NewDecoder(c.Request().Body).Decode(&listsOfAdresses); err != nil {
		return c.JSON(http.StatusOK, echo.Map{"outcome": "error",
																					"message": err})
	}

	finallist := dispers.SelectAdresses(listsOfAdresses)

	return c.JSON(http.StatusCreated, echo.Map{
		"ok":   true,
		"adresses": finallist,
		"metadata" : "blablabla",
	})
}

func getQueriesAndTokens(c echo.Context) error {

	var localQuery dispers.InputT

	if err := json.NewDecoder(c.Request().Body).Decode(&localQuery); err != nil {
		return c.JSON(http.StatusOK, echo.Map{"outcome": "error",
																					"message": err})
	}

	tokens := dispers.GetTokens(localQuery)

	return c.JSON(http.StatusCreated, echo.Map{
		"ok":   true,
		"tokens": tokens,
		"metadata" : "blablabla",
	})
}

func launchAggr(c echo.Context) error {

	var inputDA dispers.InputDA

	if err := json.NewDecoder(c.Request().Body).Decode(&inputDA); err != nil {
		return c.JSON(http.StatusOK, echo.Map{"outcome": "error",
																					"message": err})
	}

	myDA := dispers.NewDataAggregation(inputDA)

	return c.JSON(http.StatusCreated, echo.Map{
		"ok":   true,
		"id": 	myDA.DocID,
		"metadata" : "blablabla",
	})
}

func getStateAggr(c echo.Context) error {
	// TODO : Prévoir sûrement un token pour mettre des droits d'accès
	id := c.Param("id")
	return c.JSON(http.StatusOK, dispers.GetStateOrGetResult(id))
}

// Routes sets the routing for the dispers service
func Routes(router *echo.Group) {
	// API's Index
	router.GET("/", Index)
	router.GET("/:actor", IndexBis)

	// Subscriptions
	router.GET("/conductor/subscribe/_all_data", allData)
	//router.POST("/conductor/:domain/subscription", subscribe)
	//router.DELETE("/conductor/:domain/subscription", unsubscribe)

	// Trainings (used by the querier)
	router.GET("/conductor/training/:idtrain", showTraining)
	router.POST("/conductor/training", createTraining)
	//router.DELETE("/conductor/training", deleteTraining)

	router.POST("/conceptindexor/hash/:concept", hashConcept)
	router.POST("/conceptindexor/addconcept/:concept", addConcept) // used to add a concept to his list and generate SymCk

	router.POST("/targetfinder/adresses", selectAdresses)

	router.POST("/target/gettokens", getQueriesAndTokens)

	router.GET("/dataaggregator/aggregate/:id", getStateAggr)
	router.POST("/dataaggregator/aggregate", launchAggr)


}
