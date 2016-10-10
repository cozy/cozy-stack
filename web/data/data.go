// Package data provide simple CRUD operation on couchdb doc
package data

import (
	"net/http"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

func validDoctype(c *gin.Context) {
	// TODO extends me to verificate characters allowed in db name.
	doctype := c.Param("doctype")
	if doctype == "" {
		c.AbortWithError(http.StatusBadRequest, errors.InvalidDoctype(doctype))
	} else {
		c.Set("doctype", doctype)
	}
}

// GetDoc get a doc by its type and id
func getDoc(c *gin.Context) {
	instance := c.MustGet("instance").(*middlewares.Instance)
	doctype := c.MustGet("doctype").(string)
	docid := doctype + "/" + c.Param("docid")

	prefix := instance.GetDatabasePrefix()

	var out couchdb.JSONDoc
	err := couchdb.GetDoc(prefix, doctype, docid, &out)
	if err != nil {
		c.AbortWithError(errors.HTTPStatus(err), err)
		return
	}
	c.JSON(200, out)
}

// CreateDoc create doc from the json passed as body
func createDoc(c *gin.Context) {
	doctype := c.MustGet("doctype").(string)
	instance := middlewares.GetInstance(c)
	prefix := instance.GetDatabasePrefix()

	var doc couchdb.JSONDoc
	if err := c.BindJSON(&doc); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	err := couchdb.CreateDoc(prefix, doctype, doc)
	if err != nil {
		c.AbortWithError(errors.HTTPStatus(err), err)
		return
	}

	c.JSON(200, gin.H{
		"ok":   true,
		"id":   doc.ID(),
		"rev":  doc.Rev(),
		"data": doc,
	})
}

// Routes sets the routing for the status service
func Routes(router *gin.RouterGroup) {
	router.GET("/:doctype/:docid", validDoctype, getDoc)
	router.POST("/:doctype", validDoctype, createDoc)
	// router.DELETE("/:doctype/:docid", DeleteDoc)
}
