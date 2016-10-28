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
	Data     *json.RawMessage `json:"data,omitempty"`
	Errors   ErrorList        `json:"errors,omitempty"`
	Links    *LinksList       `json:"links,omitempty"`
	Included []interface{}    `json:"included,omitempty"`
}

// Data can be called to send an answer with a JSON-API document containing a
// single object as data
func Data(c *gin.Context, statusCode int, o Object, links *LinksList) {
	var included []interface{}
	for _, o := range o.Included() {
		data, err := MarshalObject(o)
		if err != nil {
			AbortWithError(c, InternalServerError(err))
			return
		}
		included = append(included, &data)
	}
	data, err := MarshalObject(o)
	if err != nil {
		AbortWithError(c, InternalServerError(err))
		return
	}
	doc := Document{
		Data:     &data,
		Links:    links,
		Included: included,
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
//
// TODO could be nice to have AbortWithErrors(c *gin.Context, errors ErrorList)
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

// Bind is used to unmarshal an input JSONApi document. It binds an
// incoming request to a attribute type.
func Bind(req *http.Request, attrs interface{}) (*ObjectMarshalling, error) {
	decoder := json.NewDecoder(req.Body)
	var doc *Document
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	var obj *ObjectMarshalling
	if err := json.Unmarshal(*doc.Data, &obj); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(*obj.Attributes, &attrs); err != nil {
		return nil, err
	}
	return obj, nil
}
