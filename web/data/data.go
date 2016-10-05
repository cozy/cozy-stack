// Package data provide simple CRUD operation on couchdb doc
package data

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

func abortNoInstance(c *gin.Context) {
	err := fmt.Errorf("No instance found")
	c.AbortWithError(http.StatusInternalServerError, err)
}

func makeCode(err error) int {
	coucherr, iscoucherr := err.(*couchdb.Error)
	if iscoucherr {
		return coucherr.StatusCode
	}

	return http.StatusInternalServerError
}

func validDoctype(doctype string) bool {
	// TODO extends me to verificate characters allowed in db name.
	return doctype != ""
}

// GetDoc get a doc by its type and id
func GetDoc(c *gin.Context) {
	// @TODO this should be extracted to a middleware
	instance, exists := c.Get("instance")
	if !exists {
		abortNoInstance(c)
		return
	}

	prefix := instance.(*middlewares.Instance).GetDatabasePrefix()
	var out couchdb.Doc

	reqerr := couchdb.GetDoc(prefix, c.Param("doctype"), c.Param("docid"), out)
	if reqerr != nil {
		coucherr, iscoucherr := reqerr.(*couchdb.Error)
		if iscoucherr {
			c.AbortWithError(coucherr.StatusCode, coucherr)
		} else {
			c.AbortWithError(http.StatusInternalServerError, reqerr)
		}
		return
	}
	c.JSON(200, out)
}

// CreateDoc create doc from the json passed as body
func CreateDoc(c *gin.Context) {
	instance := c.MustGet("instance").(*middlewares.Instance)
	doctype := c.Param("doctype")
	if !validDoctype(doctype) {
		err := fmt.Errorf("Invalid document type.")
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	var doc couchdb.Doc
	err := c.BindJSON(&doc)
	if err != nil {
		err := fmt.Errorf("invalid body")
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	prefix := instance.GetDatabasePrefix()

	reqerr := couchdb.CreateDoc(prefix, c.Param("doctype"), doc)
	if reqerr != nil {
		c.AbortWithError(makeCode(reqerr), reqerr)
		return
	}
	c.JSON(200, gin.H{
		"ok":   true,
		"id":   doc["_id"],
		"rev":  doc["_rev"],
		"data": doc,
	})

}

// Routes sets the routing for the status service
func Routes(router *gin.RouterGroup) {
	router.GET("/:doctype/:docid", GetDoc)
	router.POST("/:doctype", CreateDoc)
	// router.DELETE("/:doctype/:docid", DeleteDoc)
}
