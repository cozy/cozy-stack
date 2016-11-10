package couchdb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// This file contains error handling code for couchdb request
// Possible errors in connecting to couchdb
// 503 Service Unavailable when the stack cant connect to couchdb or when
// 		 couchdb response is interrupted mid-stream
// 500 When the viper provided configuration does not allow us to properly
// 		 call http.newRequest, ie. wrong couchdbURL config

// Possible native couchdb errors
// 400 Bad Request : Bad request structure. The error can indicate an error
// 		with the request URL, path or headers. Differences in the supplied MD5
// 		hash and content also trigger this error, as this may indicate message
// 		corruption.
// 401 Unauthorized : The item requested was not available using the supplied
// 		authorization, or authorization was not supplied.
// 		{"error":"unauthorized","reason":"You are not a server admin."}
// 		{"error":"unauthorized","reason":"Name or password is incorrect."}
// 403 Forbidden : The requested item or operation is forbidden.
// 404 Not Found : The requested content could not be found. The content will
// 		include further information, as a JSON object, if available.
// 		**The structure will contain two keys, error and reason.**
//    {"error":"not_found","reason":"deleted"}
//    {"error":"not_found","reason":"missing"}
//    {"error":"not_found","reason":"no_db_file"}
// 405 Resource Not Allowed : A request was made using an invalid HTTP request
// 		type for the URL requested. For example, you have requested a PUT when a
// 		POST is required. Errors of this type can also triggered by invalid URL
// 		strings.
// 406 Not Acceptable : The requested content type is not supported by the
// 		server.
// 409 Conflict : Request resulted in an update conflict.
// 		{"error":"conflict","reason":"Document update conflict."}
// 412 Precondition Failed : The request headers from the client and the
// 		capabilities of the server do not match.
// 415 Bad Content Type : The content types supported, and the content type of
// 		the information being requested or submitted indicate that the content
// 		type is not supported.
// 416 Requested Range Not Satisfiable : The range specified in the request
// 		header cannot be satisfied by the server.
// 417 Expectation Failed : When sending documents in bulk, the bulk load
// 		operation failed.
// 500 Internal Server Error : The request was invalid, either because the
// 		supplied JSON was invalid, or invalid information was supplied as part
// 		of the request.

// Error represent an error from couchdb
type Error struct {
	StatusCode  int
	CouchdbJSON []byte `json:"-"`
	Name        string `json:"error"`
	Reason      string `json:"reason"`
	Original    error
}

func (e *Error) Error() string {
	if e.CouchdbJSON != nil {
		return fmt.Sprintf("CouchdbError %d : %s", e.StatusCode, e.CouchdbJSON)
	}
	return fmt.Sprintf("CouchdbError %d : %s(%s)", e.StatusCode, e.Name, e.Reason)
}

// JSON returns the json representation of this error
func (e *Error) JSON() map[string]interface{} {
	jsonMap := map[string]interface{}{
		"ok":     false,
		"status": strconv.Itoa(e.StatusCode),
		"error":  e.Name,
		"reason": e.Reason,
	}
	if e.Original != nil {
		jsonMap["original"] = e.Original.Error()
	}
	return jsonMap
}

// IsNoDatabaseError checks if the given error is a couch no_db_file
// error
func IsNoDatabaseError(err error) bool {
	if err == nil {
		return false
	}
	couchErr, isCouchErr := err.(*Error)
	if !isCouchErr {
		return false
	}
	return couchErr.Reason == "no_db_file" ||
		couchErr.Reason == "Database does not exist."
}

// IsNotFoundError checks if the given error is a couch not_found
// error
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	couchErr, isCouchErr := err.(*Error)
	if !isCouchErr {
		return false
	}
	return couchErr.Name == "not_found"
}

func newRequestError(originalError error) error {
	return &Error{
		StatusCode: http.StatusServiceUnavailable,
		Name:       "no_couch",
		Reason:     "wrong_config",
		Original:   originalError,
	}
}

func newConnectionError(originalError error) error {
	return &Error{
		StatusCode: http.StatusServiceUnavailable,
		Name:       "no_couch",
		Reason:     "cant_connect",
		Original:   originalError,
	}
}

func newIOReadError(originalError error) error {
	return &Error{
		StatusCode: http.StatusServiceUnavailable,
		Name:       "no_couch",
		Reason:     "hangup",
		Original:   originalError,
	}
}

func unoptimalError() error {
	return &Error{
		StatusCode: http.StatusBadRequest,
		Name:       "no_index",
		Reason:     "no matching index found, create an index",
	}
}

func newCouchdbError(statusCode int, couchdbJSON []byte) error {
	var err = &Error{
		CouchdbJSON: couchdbJSON,
	}
	parseErr := json.Unmarshal(couchdbJSON, err)
	if parseErr != nil {
		err.Name = "wrong_json"
		err.Reason = parseErr.Error()
	}
	err.StatusCode = statusCode
	return err
}
