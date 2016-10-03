// Package data provide simple CRUD operation on couchdb doc
package data

import (
	"fmt"
	"net/http"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

// @TODO test only, to be removed
func transformOutput(doc map[string]interface{}){

}


// get a doc by its type and id
//
// It returns the doc
func GetDoc(c *gin.Context) {
	// @TODO this should be extracted to a middleware
	instance, exists := c.Get("instance")
	if !exists {
		err := fmt.Errorf("No instance found")
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	prefix := instance.(*middlewares.Instance).GetDatabasePrefix()
	var out interface{}

	reqerr := couchdb.GetDoc(prefix, c.Param("doctype"), c.Param("docid"),	&out)
	if reqerr != nil {
		coucherr, ok := reqerr.(*couchdb.CouchdbError)
		if ok {
			c.AbortWithError(coucherr.StatusCode, coucherr)
		}else{
			c.AbortWithError(http.StatusInternalServerError, reqerr)
		}
		return
	}
	transformOutput(out.(map[string]interface{}))
	c.JSON(200, out);
}


// Routes sets the routing for the status service
func Routes(router *gin.RouterGroup) {
	// router.POST("/*type", CreateDoc)
	router.GET("/:doctype/:docid", GetDoc)
	// router.DELETE("/*type/:id", DeleteDoc)
}
