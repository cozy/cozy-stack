// Package files is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package files

import (
	"encoding/base64"
	"errors"
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

// DocType is the type of document, eg. file or folder
type DocType string

const (
	// FileDocType is document type
	FileDocType DocType = "io.cozy.files"
	// FolderDocType is document type
	FolderDocType = "io.cozy.folders"

	// ForbiddenFilenameChars is the list of forbidden characters in a filename.
	ForbiddenFilenameChars = "/\x00"
)

var (
	errDocAlreadyExists = errors.New("Directory or file already exists")
	errDocTypeInvalid   = errors.New("Invalid document type")
	errIllegalFilename  = errors.New("Invalid filename: empty or contains an illegal character")
	errInvalidHash      = errors.New("Invalid hash")
)

// DocMetadata encapsulates the few metadata linked to a document
// creation request.
type DocMetadata struct {
	Type       DocType
	Name       string
	FolderID   string
	Executable bool
	Tags       []string
	GivenMD5   []byte
}

func (m *DocMetadata) path() string {
	return m.FolderID + "/" + m.Name
}

// NewDocMetadata is the DocMetadata constructor. All inputs are
// validated and if wrong, an error is returned.
func NewDocMetadata(docTypeStr, name, folderID, tagsStr, md5Str string, executable bool) (m *DocMetadata, err error) {
	docType, err := parseDocType(docTypeStr)
	if err != nil {
		return
	}

	if err = checkFileName(name); err != nil {
		return
	}

	// FolderID is not mandatory. If empty, the document is at the root
	// of the FS
	if folderID != "" {
		if err = checkFileName(folderID); err != nil {
			return
		}
	}

	tags := parseTags(tagsStr)

	var givenMD5 []byte
	if md5Str != "" {
		givenMD5, err = parseMD5Hash(md5Str)
		if err != nil {
			return
		}
	}

	m = &DocMetadata{
		Type:       docType,
		Name:       name,
		FolderID:   folderID,
		Tags:       tags,
		Executable: executable,
		GivenMD5:   givenMD5,
	}

	return
}

// CreateDirectory is the method for creating a new directory
//
// @TODO
func CreateDirectory(m *DocMetadata, storage afero.Fs) error {
	if m.Type != FolderDocType {
		return errDocTypeInvalid
	}

	path := m.path()

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

	header := c.Request.Header

	m, err := NewDocMetadata(
		c.Query("Type"),
		c.Query("Name"),
		c.Param("folder-id"),
		c.Query("Tags"),
		header.Get("Content-MD5"),
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

	exists, err := checkParentFolderID(storage, m.FolderID)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	if !exists {
		err = fmt.Errorf("Parent folder with given FolderID does not exist")
		c.AbortWithError(http.StatusNotFound, err)
		return
	}

	fmt.Printf("%s:\n\t- %+v\n\t- %v\n", m.Name, m, contentType)

	switch m.Type {
	case FileDocType:
		err = Upload(m, storage, c.Request.Body)
	case FolderDocType:
		err = CreateDirectory(m, storage)
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
	case errInvalidHash:
		code = http.StatusPreconditionFailed
	default:
		code = http.StatusInternalServerError
	}
	return
}

func parseTags(str string) (tags []string) {
	for _, tag := range strings.Split(str, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return
}

func parseDocType(docType string) (result DocType, err error) {
	switch docType {
	case "io.cozy.files":
		result = FileDocType
	case "io.cozy.folders":
		result = FolderDocType
	default:
		err = errDocTypeInvalid
	}
	return
}

func parseMD5Hash(md5B64 string) ([]byte, error) {
	// Encoded md5 hash in base64 should at least have 22 caracters in
	// base64: 16*3/4 = 21+1/3
	//
	// The padding may add up to 2 characters (non useful). If we are
	// out of these boundaries we know we don't have a good hash and we
	// can bail immediatly.
	if len(md5B64) < 22 || len(md5B64) > 24 {
		return nil, errInvalidHash
	}

	givenMD5, err := base64.StdEncoding.DecodeString(md5B64)
	if err != nil || len(givenMD5) != 16 {
		return nil, errInvalidHash
	}

	return givenMD5, nil
}

func checkFileName(str string) error {
	if str == "" || strings.ContainsAny(str, ForbiddenFilenameChars) {
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
