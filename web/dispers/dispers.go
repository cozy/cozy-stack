package dispers

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/dispers"
	"github.com/cozy/cozy-stack/pkg/dispers/dispers"
	"github.com/cozy/echo"
)

/*
*
*
COMMON ROUTES : those 3 functions are used on route ./dispers/
*
*
*/
func index(c echo.Context) error {

	out := "Hello ! You reach correctly the learning part of the Cozy Learning Server."
	return c.String(http.StatusOK, out)
}

func indexBis(c echo.Context) error {
	out := ""
	actor := c.Param("actor")
	switch actor {
	case "conductor":
		out = "Hello ! I'm ruling the game !"
	case "conceptindexor":
		out = "Hello ! I will do Concept Indexor's dishes !"
	case "_target":
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

func getPublicKey(c echo.Context) error {
	out := ""
	actor := c.Param("actor")
	switch actor {
	case "conductor":
		out = "DxXa9Bqe7Fb5G4MgZ6dmXAr7v33mIuY9X"
	case "conceptindexor":
		out = "tboHCSnIPcvvX9BI89yKKGKp4u2Ra3zsP"
	case "target":
		out = "wCYxkY2RUgfisQDQnwN7yi9ur3gdKk782"
	case "dataaggregator":
		out = "X9IE7UQ4ZfXQ5jRsPbeJHRsWy4WZSwnjk"
	case "targetfinder":
		out = "Psy8PB5o6WL3PkccoLrF4pSfpr2dDPaxe"
	default:
		return nil
	}

	return c.JSON(http.StatusOK, echo.Map{
		"ok":  true,
		"key": out,
	})
}

/*
*
*
CONDUCTOR'S ROUTES : those functions are used on route ./dispers/conductor/
*
*
*/
func showTraining(c echo.Context) error {
	// TODO : Prévoir sûrement un token pour mettre des droits d'accès
	id := c.Param("idtrain")
	return c.JSON(http.StatusOK, enclave.GetTrainingState(id))
}

func createTraining(c echo.Context) error {

	var mytraining enclave.Training

	if err := json.NewDecoder(c.Request().Body).Decode(&mytraining); err != nil {
		return c.JSON(http.StatusOK, echo.Map{"outcome": "error"})
	}

	cond, err := enclave.NewConductor("domain.cozy.tool:8080", "cozyv585s6s68k5d4s", mytraining)

	if err != nil {
		return err
	}

	cond.Lead()

	return c.JSON(http.StatusCreated, echo.Map{
		"ok":   true,
		"id":   cond.Doc.ID(),
		"rev":  cond.Doc.Rev(),
		"type": cond.Doc.DocType(),
	})
}

/*
*
*
CONCEPT INDEXOR'S ROUTES : those functions are used on route ./dispers/conceptindexor/
*
*
*/

func allConcepts(c echo.Context) error {
	list, err := enclave.GetAllConcepts()
	return c.JSON(http.StatusCreated, echo.Map{
		"ok":       err == nil,
		"concepts": list,
	})
}

func hashConcept(c echo.Context) error {
	concept := c.Param("concept")
	hash, err := enclave.HashMeThat(concept)
	return c.JSON(http.StatusCreated, echo.Map{
		"ok":   err == nil,
		"hash": hash,
	})
}

func deleteConcept(c echo.Context) error {
	concept := c.Param("concept")
	err := enclave.DeleteConcept(concept)
	return c.JSON(http.StatusCreated, echo.Map{
		"ok": err == nil,
	})
}

/*
func addConcept(c echo.Context) error {

	concept := c.Param("concept")
	return c.JSON(http.StatusCreated, enclave.AddConcept(concept))
}
*/

/*
*
*
TARGET FINDER'S ROUTES : those functions are used on route ./dispers/targetfinder/
*
*
*/
func selectAddresses(c echo.Context) error {

	var listsOfAddresses dispers.InputTF

	if err := json.NewDecoder(c.Request().Body).Decode(&listsOfAddresses); err != nil {
		return c.JSON(http.StatusOK, echo.Map{"outcome": "error",
			"message": err})
	}

	finallist := enclave.SelectAddresses(listsOfAddresses)

	return c.JSON(http.StatusCreated, echo.Map{
		"ok":        true,
		"Addresses": finallist,
		"metadata":  "blablabla",
	})
}

/*
*
*
Target'S ROUTES : those functions are used on route ./dispers/target/
*
*
*/
func getQueriesAndTokens(c echo.Context) error {

	var localQuery dispers.InputT

	if err := json.NewDecoder(c.Request().Body).Decode(&localQuery); err != nil {
		return c.JSON(http.StatusOK, echo.Map{"outcome": "error",
			"message": err})
	}

	tokens := enclave.GetTokens(localQuery)

	return c.JSON(http.StatusCreated, echo.Map{
		"ok":       true,
		"tokens":   tokens,
		"metadata": "blablabla",
	})
}

func allData(c echo.Context) error {

	var supportedData = []string{
		"iris", "bank.label",
	}

	return c.JSON(http.StatusCreated, echo.Map{
		"data": supportedData,
	})
}

/*
*
*
DATA AGGREGATOR'S ROUTES : those functions are used on route ./dispers/dataaggregator/
*
*
*/
func launchAggr(c echo.Context) error {

	var inputDA dispers.InputDA

	if err := json.NewDecoder(c.Request().Body).Decode(&inputDA); err != nil {
		return c.JSON(http.StatusOK, echo.Map{"outcome": "error",
			"message": err})
	}

	myDA := enclave.NewDataAggregation(inputDA)

	return c.JSON(http.StatusCreated, echo.Map{
		"ok":       true,
		"id":       myDA.DocID,
		"metadata": "blablabla",
	})
}

func getStateAggr(c echo.Context) error {
	// TODO : Prévoir sûrement un token pour mettre des droits d'accès
	id := c.Param("id")
	return c.JSON(http.StatusOK, enclave.GetStateOrGetResult(id))
}

// Routes sets the routing for the dispers service
func Routes(router *echo.Group) {
	// API's Index
	router.GET("/", index)
	router.GET("/:actor", indexBis)
	router.GET("/:actor/publickey", getPublicKey)

	// Subscriptions
	router.GET("/conductor/training/:idtrain", showTraining) // Used by the user to know the training's state
	router.POST("/conductor/training", createTraining)       // Used by the user to launch a training
	//router.DELETE("/conductor/training", deleteTraining) // Used by the user to cancel a training
	//router.POST("/conductor/subscribe/id=:domain", subscribe)
	//router.DELETE("/conductor/subscribe/id=:domain", unsubscribe)

	router.GET("/conceptindexor/allconcepts", allConcepts)
	router.POST("/conceptindexor/hash/concept=:concept", hashConcept)     // used to hash a concept (and save the salt)
	router.DELETE("/conceptindexor/hash/concept=:concept", deleteConcept) // used to delete a salt in the database

	//router.POST("/targetfinder/listofAddresses", insertAddress)
	//router.DELETE("/targetfinder/listofAddresses", deleteAddress)
	router.POST("/targetfinder/addresses", selectAddresses)

	router.POST("/target/gettokens", getQueriesAndTokens)
	router.GET("/target/alldata", allData)

	router.GET("/dataaggregator/aggregate/:id", getStateAggr)
	router.POST("/dataaggregator/aggregate", launchAggr)

}
