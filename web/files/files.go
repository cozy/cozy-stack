// Package files is the HTTP frontend of the vfs package. It exposes
// an HTTP api to manipulate the filesystem and offer all the
// possibilities given by the vfs.
package files

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/vfs"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

// DefaultContentType is used for files uploaded with no content-type
const DefaultContentType = "application/octet-stream"

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

	mime, class := extractMimeAndClass(c.ContentType())

	var givenMD5 []byte
	if md5Str := header.Get("Content-MD5"); md5Str != "" {
		givenMD5, err = parseMD5Hash(md5Str)
	}
	if err != nil {
		c.AbortWithError(http.StatusUnprocessableEntity, err)
		return
	}

	size, err := parseContentLength(header.Get("Content-Length"))
	if err != nil {
		c.AbortWithError(http.StatusUnprocessableEntity, err)
		return
	}

	d, err := vfs.NewDocAttributes(
		c.Query("Type"),
		c.Query("Name"),
		c.Param("folder-id"),
		mime,
		class,
		size,
		givenMD5,
		parseTags(c.Query("Tags")),
		c.Query("Executable") == "true",
	)

	if err != nil {
		c.AbortWithError(errors.HTTPStatus(err), err)
		return
	}

	var doc jsonapi.JSONApier
	switch d.DocType() {
	case vfs.FileDocType:
		doc, err = vfs.CreateFileAndUpload(d, storage, dbPrefix, c.Request.Body)
	case vfs.FolderDocType:
		doc, err = vfs.CreateDirectory(d, storage, dbPrefix)
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

func parseTags(str string) (tags []string) {
	for _, tag := range strings.Split(str, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
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
		return nil, fmt.Errorf("Given Content-MD5 is invalid")
	}

	givenMD5, err := base64.StdEncoding.DecodeString(md5B64)
	if err != nil || len(givenMD5) != 16 {
		return nil, fmt.Errorf("Given Content-MD5 is invalid")
	}

	return givenMD5, nil
}

func parseContentLength(contentLength string) (size int64, err error) {
	if contentLength == "" {
		size = -1
		return
	}

	size, err = strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		err = fmt.Errorf("Invalid content length")
	}
	return
}

func extractMimeAndClass(contentType string) (mime, class string) {
	if contentType == "" {
		contentType = DefaultContentType
	}

	charsetIndex := strings.Index(contentType, ";")
	if charsetIndex >= 0 {
		mime = contentType[:charsetIndex]
	} else {
		mime = contentType
	}

	// @TODO improve for specific mime types
	slashIndex := strings.Index(contentType, "/")
	if slashIndex >= 0 {
		class = contentType[:slashIndex]
	} else {
		class = contentType
	}

	return
}
