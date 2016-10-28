// Package data provide simple CRUD operation on couchdb doc
package data

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

func validDoctype(c *gin.Context) {
	// TODO extends me to verificate characters allowed in db name.
	doctype := c.Param("doctype")
	if doctype == "" {
		c.AbortWithError(http.StatusBadRequest, invalidDoctypeErr(doctype))
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
		c.AbortWithError(HTTPStatus(err), err)
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
	if err := binding.JSON.Bind(c.Request, &doc.M); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if doc.ID() != "" {
		err := fmt.Errorf("Cannot create a document with _id")
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	err := couchdb.CreateDoc(prefix, doc)
	if err != nil {
		c.AbortWithError(HTTPStatus(err), err)
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
	if err := binding.JSON.Bind(c.Request, &doc); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	doc.Type = c.Param("doctype")

	if (doc.ID() == "") != (doc.Rev() == "") {
		err := fmt.Errorf("You must either provide an _id and _rev in document (update) or neither (create with  fixed id).")
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if doc.ID() != "" && doc.ID() != c.Param("docid") {
		err := fmt.Errorf("document _id doesnt match url")
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	var err error
	if doc.ID() == "" {
		doc.SetID(c.Param("docid"))
		err = couchdb.CreateNamedDoc(prefix, doc)
	} else {
		err = couchdb.UpdateDoc(prefix, doc)
	}

	if err != nil {
		c.AbortWithError(HTTPStatus(err), err)
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
	revHeader := c.Request.Header.Get("If-Match")
	revQuery := c.Query("rev")
	rev := ""

	if revHeader != "" && revQuery != "" && revQuery != revHeader {
		err := fmt.Errorf("If-Match Header and rev query parameters mismatch")
		c.AbortWithError(http.StatusBadRequest, err)
		return
	} else if revHeader != "" {
		rev = revHeader
	} else if revQuery != "" {
		rev = revQuery
	} else {
		err := fmt.Errorf("delete without revision")
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	tombrev, err := couchdb.Delete(prefix, doctype, docid, rev)
	if err != nil {
		c.AbortWithError(HTTPStatus(err), err)
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
