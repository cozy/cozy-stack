// Package files is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package files

import (
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/spf13/afero"
)

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
	errDocAlreadyExists      = os.ErrExist
	errDocDoesNotExist       = os.ErrNotExist
	errParentDoesNotExist    = errors.New("Parent folder with given FolderID does not exist")
	errDocTypeInvalid        = errors.New("Invalid document type")
	errIllegalFilename       = errors.New("Invalid filename: empty or contains an illegal character")
	errInvalidHash           = errors.New("Invalid hash")
	errContentLengthInvalid  = errors.New("Invalid content length")
	errContentLengthMismatch = errors.New("Content length does not match")
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
	contentLength, err := parseContentLength(header.Get("Content-Length"))
	if err != nil {
		c.AbortWithError(http.StatusUnprocessableEntity, err)
		return
	}

	var doc jsonapi.JSONApier
	switch m.Type {
	case FileDocType:
		doc, err = CreateFileAndUpload(m, storage, contentType, contentLength, dbPrefix, c.Request.Body)
	case FolderDocType:
		doc, err = CreateDirectory(m, storage, dbPrefix)
	}

	if err != nil {
		c.AbortWithError(makeCode(err), err)
		return
	}

	data, err := doc.ToJSONApi()
	if err != nil {
		c.AbortWithError(makeCode(err), err)
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
		err = ServeFileContentByPath(pth, c.Request, c.Writer, storage)
	} else {
		var doc *FileDoc
		doc, err = GetFileDoc(string(FileDocType)+"/"+fileID, dbPrefix)
		if err == nil {
			err = ServeFileContent(doc, c.Request, c.Writer, storage)
		}
	}

	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
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

func makeCode(err error) (code int) {
	switch err {
	case errDocAlreadyExists:
		code = http.StatusConflict
	case errParentDoesNotExist:
		code = http.StatusNotFound
	case errDocDoesNotExist:
		code = http.StatusNotFound
	case errInvalidHash:
		code = http.StatusPreconditionFailed
	case errContentLengthMismatch:
		code = http.StatusPreconditionFailed
	default:
		couchErr, isCouchErr := err.(*couchdb.Error)
		if isCouchErr {
			code = couchErr.StatusCode
		} else {
			code = http.StatusInternalServerError
		}
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

func parseContentLength(contentLength string) (size int64, err error) {
	if contentLength == "" {
		size = -1
		return
	}

	size, err = strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		err = errContentLengthInvalid
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

// checkParentFolderID is used to generate the filepath of a new file
// or directory. It will check if the given parent folderID is well
// defined is the database and filesystem and it will generate the new
// path of the wanted file, checking if there is not colision with
// existing file.
func createNewFilePath(m *DocMetadata, storage afero.Fs, dbPrefix string) (pth string, parentDoc *DirDoc, err error) {
	folderID := m.FolderID

	var parentPath string

	if folderID == "" {
		parentPath = "/"
	} else {
		qFolderID := string(FolderDocType) + "/" + folderID
		parentDoc = &DirDoc{}

		// NOTE: we only check the existence of the folder on the db
		err = couchdb.GetDoc(dbPrefix, string(FolderDocType), qFolderID, parentDoc)
		if couchdb.IsNotFoundError(err) {
			err = errParentDoesNotExist
		}
		if err != nil {
			return
		}

		parentPath = parentDoc.Path
	}

	pth = path.Join(parentPath, m.Name)
	exists, err := afero.Exists(storage, pth)
	if err != nil {
		return
	}
	if exists {
		err = errDocAlreadyExists
		return
	}

	return
}
