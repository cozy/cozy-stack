// Package data provide simple CRUD operation on couchdb doc
package data

import (
	"fmt"
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
	instance := middlewares.GetInstance(c)
	doctype := c.MustGet("doctype").(string)
	docid := c.Param("docid")

	prefix := instance.GetDatabasePrefix()

	var out couchdb.JSONDoc
	err := couchdb.GetDoc(prefix, doctype, docid, &out)
	if err != nil {
		c.AbortWithError(errors.HTTPStatus(err), err)
		return
	}
	out.Type = doctype
	c.JSON(200, out.ToMapWithType())
}

// CreateDoc create doc from the json passed as body
func createDoc(c *gin.Context) {
	doctype := c.MustGet("doctype").(string)
	instance := middlewares.GetInstance(c)
	prefix := instance.GetDatabasePrefix()

	var doc = couchdb.JSONDoc{Type: doctype}
	if err := c.BindJSON(&doc.M); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	err := couchdb.CreateDoc(prefix, doc)
	if err != nil {
		c.AbortWithError(errors.HTTPStatus(err), err)
		return
	}

	c.JSON(201, gin.H{
		"ok":   true,
		"id":   doc.ID(),
		"rev":  doc.Rev(),
		"type": doc.DocType(),
		"data": doc.ToMapWithType(),
	})
}

func updateDoc(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	prefix := instance.GetDatabasePrefix()

	var doc couchdb.JSONDoc
	if err := c.BindJSON(&doc); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	doc.Type = c.Param("doctype")

	if doc.ID() != "" && doc.ID() != c.Param("docid") {
		err := fmt.Errorf("_id in document doesnt match url")
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	rev := c.Request.Header.Get("If-Match")
	if rev != "" {
		doc.SetRev(rev)
	}

	err := couchdb.UpdateDoc(prefix, doc)
	if err != nil {
		c.AbortWithError(errors.HTTPStatus(err), err)
		return
	}

	c.JSON(200, gin.H{
		"ok":   true,
		"id":   doc.ID(),
		"rev":  doc.Rev(),
		"type": doc.DocType(),
		"data": doc.ToMapWithType(),
	})
}

func deleteDoc(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	doctype := c.MustGet("doctype").(string)
	docid := c.Param("docid")
	prefix := instance.GetDatabasePrefix()
	rev := c.Request.Header.Get("If-Match")

	if rev == "" {
		err := fmt.Errorf("NotImplemented : delete without If-Match")
		c.AbortWithError(http.StatusNotImplemented, err)
		return
	}

	tombrev, err := couchdb.Delete(prefix, doctype, docid, rev)
	if err != nil {
		c.AbortWithError(errors.HTTPStatus(err), err)
		return
	}

	c.JSON(200, gin.H{
		"ok":      true,
		"id":      docid,
		"rev":     tombrev,
		"type":    doctype,
		"deleted": true,
	})

}

// Routes sets the routing for the status service
func Routes(router *gin.RouterGroup) {
	router.GET("/:doctype/:docid", validDoctype, getDoc)
	router.PUT("/:doctype/:docid", validDoctype, updateDoc)
	router.DELETE("/:doctype/:docid", validDoctype, deleteDoc)
	router.POST("/:doctype/", validDoctype, createDoc)
	// router.DELETE("/:doctype/:docid", DeleteDoc)
}
