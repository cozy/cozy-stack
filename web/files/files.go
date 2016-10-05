// Package files is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package files

import (
	"errors"
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

// DocType is the type of document, eg. file or folder
type DocType string

const (
	// FileDocType is document type
	FileDocType DocType = "io.cozy.files"
	// FolderDocType is document type
	FolderDocType = "io.cozy.folders"
)

var (
	errDocAlreadyExists = errors.New("Directory already exists")
	errDocTypeInvalid   = errors.New("Invalid document type")
)

// DocMetadata encapsulates the few metadata linked to a document
// creation request.
type DocMetadata struct {
	Type       DocType
	Name       string
	FolderID   string
	Executable bool
	Tags       []string
}

func (metadata *DocMetadata) path() string {
	return escapeSlash(metadata.FolderID) + "/" + escapeSlash(metadata.Name)
}

// Upload is the method for uploading a file
//
// This will be used to upload a file
// @TODO
func Upload(metadata *DocMetadata, storage afero.Fs, body io.ReadCloser) error {
	if metadata.Type != FileDocType {
		return errDocTypeInvalid
	}

	path := metadata.path()

	defer body.Close()
	return afero.SafeWriteReader(storage, path, body)
}

// CreateDirectory is the method for creating a new directory
//
// @TODO
func CreateDirectory(metadata *DocMetadata, storage afero.Fs) error {
	if metadata.Type != FolderDocType {
		return errDocTypeInvalid
	}

	path := metadata.path()

	exists, err := afero.DirExists(storage, path)
	if err != nil {
		return err
	}
	if exists {
		return errDocAlreadyExists
	}

	return storage.Mkdir(path, 0777)
}

// CreationHandler handle all POST requests on /files/:folder-id
// aiming at creating a new document in the FS. Given the Type
// parameter of the request, it will either upload a new file or
// create a new directory.
//
// swagger:route POST /files/:folder-id files uploadFileOrCreateDir
func CreationHandler(c *gin.Context) {
	instance, err := middlewares.GetInstance(c)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	storage, err := instance.GetStorageProvider()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	docType, err := parseDocType(c.Query("Type"))
	if err != nil {
		c.AbortWithError(http.StatusUnprocessableEntity, err)
		return
	}

	tags := parseTags(c.Query("Tags"))

	metadata := &DocMetadata{
		Type:       docType,
		Name:       c.Query("Name"),
		Executable: c.Query("Executable") == "true",
		FolderID:   c.Param("folder-id"),
		Tags:       tags,
	}

	contentType := c.ContentType()
	if contentType == "" {
		contentType = DefaultContentType
	}

	if metadata.Name == "" {
		err = fmt.Errorf("Missing Name in the query-string")
		c.AbortWithError(http.StatusUnprocessableEntity, err)
		return
	}

	exists, err := checkParentFolderID(storage, metadata.FolderID)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	if !exists {
		err = fmt.Errorf("Parent folder with given FolderID does not exist")
		c.AbortWithError(http.StatusNotFound, err)
		return
	}

	fmt.Printf("%s:\n\t- %+v\n\t- %v\n", metadata.Name, metadata, contentType)

	switch metadata.Type {
	case FileDocType:
		err = Upload(metadata, storage, c.Request.Body)
	case FolderDocType:
		err = CreateDirectory(metadata, storage)
	}

	if err != nil {
		var code int
		switch err {
		case errDocAlreadyExists:
			code = http.StatusConflict
		default:
			code = http.StatusInternalServerError
		}
		c.AbortWithError(code, err)
		return
	}

	data := []byte{'O', 'K'}
	c.Data(http.StatusCreated, jsonapi.ContentType, data)
}

// Routes sets the routing for the files service
func Routes(router *gin.RouterGroup) {
	router.POST("/:folder-id", CreationHandler)
}

func escapeSlash(str string) string {
	return strings.Replace(str, "/", "\\/", -1)
}

func parseTags(str string) []string {
	var tags []string
	for _, tag := range strings.Split(str, ",") {
		// @TODO: more sanitization maybe ?
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func parseDocType(docType string) (DocType, error) {
	var result DocType
	var err error
	switch docType {
	case "io.cozy.files":
		result = FileDocType
	case "io.cozy.folders":
		result = FolderDocType
	default:
		err = errDocTypeInvalid
	}
	return result, err
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
