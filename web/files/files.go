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
	"regexp"
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
	errIllegalFilename  = errors.New("Invalid filename: empty or contains one of these illegal characters: / \\ : ? * \" |")
)

var regFileName = regexp.MustCompile("[\\/\\\\:\\?\\*\"|]+")

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
	return metadata.FolderID + "/" + metadata.Name
}

// NewDocMetadata is the DocMetadata constructor. All inputs are
// validated and if wrong, an error is returned.
func NewDocMetadata(docTypeStr, name, folderID, tagsStr string, executable bool) (*DocMetadata, error) {
	docType, err := parseDocType(docTypeStr)
	if err != nil {
		return nil, err
	}

	if err = checkFileName(name); err != nil {
		return nil, err
	}

	// FolderID is not mandatory. If empty, the document is at the root
	// of the FS
	if folderID != "" {
		if err = checkFileName(folderID); err != nil {
			return nil, err
		}
	}

	tags := parseTags(tagsStr)

	return &DocMetadata{
		Type:       docType,
		Name:       name,
		FolderID:   folderID,
		Tags:       tags,
		Executable: executable,
	}, nil
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
	instance := middlewares.GetInstance(c)
	storage, err := instance.GetStorageProvider()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	metadata, err := NewDocMetadata(
		c.Query("Type"),
		c.Query("Name"),
		c.Param("folder-id"),
		c.Query("Tags"),
		c.Query("Executable") == "true",
	)

	if err != nil {
		c.AbortWithError(http.StatusUnprocessableEntity, err)
		return
	}

	contentType := c.ContentType()
	if contentType == "" {
		contentType = DefaultContentType
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
		c.AbortWithError(makeCode(err), err)
		return
	}

	data := []byte{'O', 'K'}
	c.Data(http.StatusCreated, jsonapi.ContentType, data)
}

// Routes sets the routing for the files service
func Routes(router *gin.RouterGroup) {
	router.POST("/", CreationHandler)
	router.POST("/:folder-id", CreationHandler)
}

func makeCode(err error) (code int) {
	switch err {
	case errDocAlreadyExists:
		code = http.StatusConflict
	default:
		code = http.StatusInternalServerError
	}
	return
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

func checkFileName(str string) error {
	if str == "" || regFileName.MatchString(str) {
		return errIllegalFilename
	}
	return nil
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
