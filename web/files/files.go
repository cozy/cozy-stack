// Package files is the HTTP frontend of the vfs package. It exposes
// an HTTP api to manipulate the filesystem and offer all the
// possibilities given by the vfs.
package files

import (
	"net/http"

	"github.com/cozy/cozy-stack/vfs"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

// CreationHandler handle all POST requests on /files/:folder-id
// aiming at creating a new document in the FS. Given the Type
// parameter of the request, it will either upload a new file or
// create a new directory.
//
// swagger:route POST /files/:folder-id files uploadFileOrCreateDir
func CreationHandler(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	dbPrefix := instance.GetDatabasePrefix()
	storage, err := instance.GetStorageProvider()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	header := c.Request.Header
	contentType := c.ContentType()
	contentLength := header.Get("Content-Length")

	m, err := vfs.NewDocAttributes(
		c.Query("Type"),
		c.Query("Name"),
		c.Param("folder-id"),
		c.Query("Tags"),
		header.Get("Content-MD5"),
		contentType,
		contentLength,
		c.Query("Executable") == "true",
	)

	if err != nil {
		c.AbortWithError(http.StatusUnprocessableEntity, err)
		return
	}

	var doc jsonapi.JSONApier
	switch m.DocType() {
	case vfs.FileDocType:
		doc, err = vfs.CreateFileAndUpload(m, storage, dbPrefix, c.Request.Body)
	case vfs.FolderDocType:
		doc, err = vfs.CreateDirectory(m, storage, dbPrefix)
	}

	if err != nil {
		c.AbortWithError(errors.HTTPStatus(err), err)
		return
	}

	data, err := doc.ToJSONApi()
	if err != nil {
		c.AbortWithError(errors.HTTPStatus(err), err)
		return
	}

	c.Data(http.StatusCreated, jsonapi.ContentType, data)
}

// ReadHandler handle all GET requests on /files/:file-id aiming at
// downloading a file. It serves two main purposes in this regard:
//  - downloading a file given its ID in inline mode
//  - downloading a file given its path in attachment mode on the
//    /files/download endpoint
//
// swagger:route GET /files/download files downloadFileByPath
// swagger:route GET /files/:file-id files downloadFileByID
func ReadHandler(c *gin.Context) {
	var err error

	instance := middlewares.GetInstance(c)
	dbPrefix := instance.GetDatabasePrefix()
	storage, err := instance.GetStorageProvider()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	fileID := c.Param("file-id")

	// Path /files/download is handled specifically to download file
	// form their path
	if fileID == "download" {
		pth := c.Query("path")
		err = vfs.ServeFileContentByPath(pth, c.Request, c.Writer, storage)
	} else {
		var doc *vfs.FileDoc
		doc, err = vfs.GetFileDoc(fileID, dbPrefix)
		if err == nil {
			err = vfs.ServeFileContent(doc, c.Request, c.Writer, storage)
		}
	}

	if err != nil {
		c.AbortWithError(errors.HTTPStatus(err), err)
		return
	}
}

// Routes sets the routing for the files service
func Routes(router *gin.RouterGroup) {
	router.HEAD("/:file-id", ReadHandler)
	router.GET("/:file-id", ReadHandler)

	router.POST("/", CreationHandler)
	router.POST("/:folder-id", CreationHandler)
}
