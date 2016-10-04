// Package files is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package files

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/spf13/afero"
)

// DefaultContentType is used for files uploaded with no content-type
const DefaultContentType = "application/octet-stream"

// Upload is the handler for uploading a file
//
// swagger:route POST /files/:folder-id files uploadFile
//
// This will be used to upload a file
func Upload(c *gin.Context) {
	doctype := c.Query("Type")
	if doctype != "io.cozy.files" {
		err := fmt.Errorf("Invalid Type in the query-string")
		c.AbortWithError(422, err)
		return
	}

	name := c.Query("Name")
	if name == "" {
		err := fmt.Errorf("Missing Name in the query-string")
		c.AbortWithError(422, err)
		return
	}

	tags := strings.Split(c.Query("Tags"), ",")
	executable := c.Query("Executable") == "true"
	folderID := c.Param("folder-id")

	contentType := c.ContentType()
	if contentType == "" {
		contentType = DefaultContentType
	}

	instanceInterface, exists := c.Get("instance")
	if !exists {
		err := fmt.Errorf("No instance found")
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	instance := instanceInterface.(*middlewares.Instance)
	storage, err := instance.GetStorageProvider()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	fmt.Printf("%s:\n\t- %s\n\t- %v\n\t- %v\n", name, contentType, tags, executable)
	path := folderID + "/" + name
	err = afero.SafeWriteReader(*storage, path, c.Request.Body)
	c.Request.Body.Close()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	data := []byte{'O', 'K'}
	c.Data(http.StatusCreated, jsonapi.ContentType, data)
}

// Routes sets the routing for the files service
func Routes(router *gin.RouterGroup) {
	router.POST("/:folder-id", Upload)
}
