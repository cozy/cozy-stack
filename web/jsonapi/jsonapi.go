// Package jsonapi is for using the JSON-API format: parsing, serialization,
// checking the content-type, etc.
package jsonapi

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ContentType is the official mime-type for JSON-API
const ContentType = "application/vnd.api+json"

// Document is JSON-API document, identified by the mediatype
// application/vnd.api+json
// See http://jsonapi.org/format/#document-structure
type Document struct {
	Data   *json.RawMessage `json:"data,omitempty"`
	Errors ErrorList        `json:"errors,omitempty"`
	Links  *LinksList       `json:"links,omitempty"`
	// TODO included, links
}

// Data can be called to send an answer with a JSON-API document containing a
// single object as data
func Data(c *gin.Context, statusCode int, o Object, links *LinksList) {
	data, err := MarshalObject(o)
	if err != nil {
		AbortWithError(c, InternalServerError(err))
		return
	}
	doc := Document{
		Data:  &data,
		Links: links,
	}
	body, err := json.Marshal(doc)
	if err != nil {
		AbortWithError(c, InternalServerError(err))
		return
	}
	c.Data(statusCode, ContentType, body)
}

// AbortWithError can be called to abort the current http request/response
// processing, and send an error in the JSON-API format
func AbortWithError(c *gin.Context, e *Error) {
	doc := Document{
		Errors: ErrorList{e},
	}
	body, err := json.Marshal(doc)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.Data(e.Status, ContentType, body)
	c.Abort()
}

// TODO could be nice to have AbortWithErrors(c *gin.Context, errors ErrorList)
