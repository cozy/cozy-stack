// Package files is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package files

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/spf13/afero"
)

// DefaultContentType is used for files uploaded with no content-type
const DefaultContentType = "application/octet-stream"

type FileMetadata struct {
	DocType    string
	Name       string
	FolderID   string
	Executable bool
	Tags       []string
}

func extractTags(str string) []string {
	tags := make([]string, 0)
	for _, tag := range strings.Split(str, ",") {
		// @TODO: more sanitization maybe ?
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// Upload is the method for uploading a file
//
// This will be used to upload a file
// @TODO(bruno): wip
func Upload(metadata *FileMetadata, storage afero.Fs, body io.ReadCloser) error {
	path := metadata.FolderID + "/" + metadata.Name

	defer body.Close()
	return afero.SafeWriteReader(storage, path, body)
}

// CreateDirectory is the method for creating a new directory
//
// @TODO(pierre): wip
func CreateDirectory(metadata *FileMetadata, storage afero.Fs) error {
	path := metadata.FolderID + "/" + metadata.Name

	return storage.Mkdir(path, 0777)
}

// Handle all POST requests on /:folder-id and given the Type
// parameter of the request, it will either upload a new file or
// create a new directory.
//
// swagger:route POST /files/:folder-id files uploadFileOrCreateDir
func FolderPostHandler(c *gin.Context) {
	name := c.Query("Name")
	if name == "" {
		err := fmt.Errorf("Missing Name in the query-string")
		c.AbortWithError(http.StatusUnprocessableEntity, err)
		return
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

	tags := extractTags(c.Query("Tags"))

	metadata := &FileMetadata{
		Name:       name,
		DocType:    c.Query("Type"),
		Executable: c.Query("Executable") == "true",
		FolderID:   c.Param("folder-id"),
		Tags:       tags,
	}

	contentType := c.ContentType()
	if contentType == "" {
		contentType = DefaultContentType
	}

	fmt.Printf("%s:\n\t- %+v\n\t- %v\n", metadata, contentType)

	switch metadata.DocType {
	case "io.cozy.files":
		err = Upload(metadata, storage, c.Request.Body)
	case "io.cozy.folders":
		err = CreateDirectory(metadata, storage)
	default:
		err = fmt.Errorf("Invalid Type in the query-string")
		c.AbortWithError(http.StatusUnprocessableEntity, err)
		return
	}

	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	data := []byte{'O', 'K'}
	c.Data(http.StatusCreated, jsonapi.ContentType, data)
}

// Routes sets the routing for the files service
func Routes(router *gin.RouterGroup) {
	router.POST("/:folder-id", FolderPostHandler)
}
