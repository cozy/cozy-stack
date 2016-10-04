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

func (metadata *FileMetadata) Path() string {
	return escapeSlash(metadata.FolderID) + "/" + escapeSlash(metadata.Name)
}

func escapeSlash(str string) string {
	return strings.Replace(str, "/", "\\/", -1)
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

func checkParentFolderID(storage afero.Fs, folderID string) (bool, error) {
	if folderID == "" {
		return true, nil
	}

	exists, err := afero.DirExists(storage, folderID)
	if err != nil {
		return false, err
	}

	if !exists {
		return false, nil
	}

	return true, nil
}

// Upload is the method for uploading a file
//
// This will be used to upload a file
// @TODO(bruno): wip
func Upload(metadata *FileMetadata, storage afero.Fs, body io.ReadCloser) error {
	path := metadata.Path()

	defer body.Close()
	return afero.SafeWriteReader(storage, path, body)
}

// CreateDirectory is the method for creating a new directory
//
// @TODO(pierre): wip
func CreateDirectory(metadata *FileMetadata, storage afero.Fs) error {
	path := metadata.Path()

	exists, err := afero.DirExists(storage, path)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("Directory already exists")
	}

	return storage.Mkdir(path, 0777)
}

// Handle all POST requests on /:folder-id and given the Type
// parameter of the request, it will either upload a new file or
// create a new directory.
//
// swagger:route POST /files/:folder-id files uploadFileOrCreateDir
func FolderPostHandler(c *gin.Context) {
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
		Name:       c.Query("Name"),
		DocType:    c.Query("Type"),
		Executable: c.Query("Executable") == "true",
		FolderID:   c.Param("folder-id"),
		Tags:       tags,
	}

	contentType := c.ContentType()
	if contentType == "" {
		contentType = DefaultContentType
	}

	if metadata.Name == "" {
		err := fmt.Errorf("Missing Name in the query-string")
		c.AbortWithError(http.StatusUnprocessableEntity, err)
		return
	}

	exists, err = checkParentFolderID(storage, metadata.FolderID)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	if !exists {
		err := fmt.Errorf("Parent folder with given FolderID does not exist")
		c.AbortWithError(http.StatusNotFound, err)
		return
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
