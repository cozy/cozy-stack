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

// JSONApier is a temporary interface to describe how to serialize an
// object on the api side.
// @TODO: proper jsonapi handling. See issue #10
type JSONApier interface {
	ToJSONApi() ([]byte, error)
}

// AbortWithError can be called to abort the current http request/response
// processing, and send an error in the JSON-API format
func AbortWithError(c *gin.Context, e *Error) {
	doc := map[string]interface{}{
		"errors": []*Error{e},
	}
	body, err := json.Marshal(doc)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.Data(e.Status, ContentType, body)
	c.Abort()
}
