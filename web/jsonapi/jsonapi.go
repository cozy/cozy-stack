// Package jsonapi is for using the JSON-API format: parsing, serialization,
// checking the content-type, etc.
package jsonapi

// ContentType is the official mime-type for JSON-API
const ContentType = "application/vnd.api+json"

// JSONApier is a temporary interface to describe how to serialize an
// object on the api side.
// @TODO: proper jsonapi handling. See issue #10
type JSONApier interface {
	ToJSONApi() ([]byte, error)
}
