// Package jsonapi is for using the JSON-API format: parsing, serialization,
// checking the content-type, etc.
package jsonapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/labstack/echo"
)

// ContentType is the official mime-type for JSON-API
const ContentType = "application/vnd.api+json"

// Document is JSON-API document, identified by the mediatype
// application/vnd.api+json
// See http://jsonapi.org/format/#document-structure
type Document struct {
	Data     *json.RawMessage  `json:"data,omitempty"`
	Errors   ErrorList         `json:"errors,omitempty"`
	Links    *LinksList        `json:"links,omitempty"`
	Meta     *RelationshipMeta `json:"meta,omitempty"`
	Included []interface{}     `json:"included,omitempty"`
}

// WriteData can be called to write an answer with a JSON-API document
// containing a single object as data into an io.Writer.
func WriteData(w io.Writer, o Object, links *LinksList) error {
	var included []interface{}

	if inc := o.Included(); inc != nil {
		included = make([]interface{}, len(inc))
		for i, o := range inc {
			data, err := MarshalObject(o)
			if err != nil {
				return err
			}
			included[i] = &data
		}
	}

	data, err := MarshalObject(o)
	if err != nil {
		return err
	}

	doc := Document{
		Data:     &data,
		Links:    links,
		Included: included,
	}
	return json.NewEncoder(w).Encode(doc)
}

// WriteError can be called to write a JSON-API error document into an
// io.Writer.
func WriteError(w io.Writer, err *Error) error {
	doc := Document{
		Errors: ErrorList{err},
	}
	return json.NewEncoder(w).Encode(doc)
}

// Data can be called to send an answer with a JSON-API document containing a
// single object as data
func Data(c echo.Context, statusCode int, o Object, links *LinksList) error {
	resp := c.Response()
	resp.Header().Set("Content-Type", ContentType)
	resp.WriteHeader(statusCode)
	return WriteData(resp, o, links)
}

// DataList can be called to send an multiple-value answer with a
// JSON-API document contains multiple objects.
func DataList(c echo.Context, statusCode int, objs []Object, links *LinksList) error {
	return DataListWithTotal(c, statusCode, len(objs), objs, links)
}

// DataListWithTotal can be called to send a list of Object with a different
// meta:count, useful to indicate total number of results with pagination.
func DataListWithTotal(c echo.Context, statusCode, total int, objs []Object, links *LinksList) error {
	objsMarshaled := make([]json.RawMessage, len(objs))
	for i, o := range objs {
		j, err := MarshalObject(o)
		if err != nil {
			return InternalServerError(err)
		}
		objsMarshaled[i] = j
	}

	data, err := json.Marshal(objsMarshaled)
	if err != nil {
		return InternalServerError(err)
	}

	doc := Document{
		Data:  (*json.RawMessage)(&data),
		Meta:  &RelationshipMeta{Count: &total},
		Links: links,
	}

	resp := c.Response()
	resp.Header().Set("Content-Type", ContentType)
	resp.WriteHeader(statusCode)
	return json.NewEncoder(resp).Encode(doc)
}

// DataRelations can be called to send a Relations page,
// a list of ResourceIdentifier
func DataRelations(c echo.Context, statusCode int, refs []couchdb.DocReference, total int, links *LinksList, included []Object) error {
	data, err := json.Marshal(refs)
	if err != nil {
		return InternalServerError(err)
	}

	doc := Document{
		Data: (*json.RawMessage)(&data),
		Meta: &RelationshipMeta{
			Count: &total,
		},
		Links: links,
	}

	if included != nil {
		includedMarshaled := make([]interface{}, len(included))
		for i, o := range included {
			j, err := MarshalObject(o)
			if err != nil {
				return InternalServerError(err)
			}
			includedMarshaled[i] = &j
		}
		doc.Included = includedMarshaled
	}

	resp := c.Response()
	resp.Header().Set("Content-Type", ContentType)
	resp.WriteHeader(statusCode)
	return json.NewEncoder(resp).Encode(doc)
}

// DataError can be called to send an error answer with a JSON-API document
// containing a single value error.
func DataError(c echo.Context, err *Error) error {
	resp := c.Response()
	resp.Header().Set("Content-Type", ContentType)
	resp.WriteHeader(err.Status)
	return WriteError(resp, err)
}

// DataErrorList can be called to send an error answer with a JSON-API document
// containing multiple errors.
func DataErrorList(c echo.Context, errs ...*Error) error {
	doc := Document{
		Errors: errs,
	}
	resp := c.Response()
	resp.Header().Set("Content-Type", ContentType)
	resp.WriteHeader(errs[0].Status)
	return json.NewEncoder(resp).Encode(doc)
}

// Bind is used to unmarshal an input JSONApi document. It binds an
// incoming request to a attribute type.
func Bind(req *http.Request, attrs interface{}) (*ObjectMarshalling, error) {
	decoder := json.NewDecoder(req.Body)
	var doc *Document
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	if doc.Data == nil {
		return nil, BadJSON()
	}
	var obj *ObjectMarshalling
	if err := json.Unmarshal(*doc.Data, &obj); err != nil {
		return nil, err
	}
	if obj.Attributes != nil {
		if err := json.Unmarshal(*obj.Attributes, &attrs); err != nil {
			return nil, err
		}
	}
	return obj, nil
}

// BindRelations extracts a Relationships request ( a list of ResourceIdentifier)
func BindRelations(req *http.Request) ([]couchdb.DocReference, error) {
	var out []couchdb.DocReference
	decoder := json.NewDecoder(req.Body)
	var doc *Document
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	if doc.Data == nil {
		return nil, BadJSON()
	}
	// Attempt Unmarshaling either as ResourceIdentifier or []ResourceIdentifier
	if err := json.Unmarshal(*doc.Data, &out); err != nil {
		var ri couchdb.DocReference
		if err = json.Unmarshal(*doc.Data, &ri); err != nil {
			return nil, err
		}
		out = []couchdb.DocReference{ri}
		return out, nil
	}
	return out, nil
}

// PaginationCursorToParams transforms a Cursor into url.Values
// the url.Values contains only keys page[limit] & page[cursor]
// if the cursor is Done, the values will be empty.
func PaginationCursorToParams(cursor couchdb.Cursor) (url.Values, error) {

	v := url.Values{}

	if !cursor.HasMore() {
		return v, nil
	}

	switch c := cursor.(type) {
	case *couchdb.StartKeyCursor:
		cursorObj := []interface{}{c.NextKey, c.NextDocID}
		cursorBytes, err := json.Marshal(cursorObj)
		if err != nil {
			return nil, err
		}
		v.Set("page[limit]", strconv.Itoa(c.Limit))
		v.Set("page[cursor]", string(cursorBytes))

	case *couchdb.SkipCursor:
		v.Set("page[limit]", strconv.Itoa(c.Limit))
		v.Set("page[skip]", strconv.Itoa(c.Skip))
	}

	return v, nil
}

// ExtractPaginationCursor creates a Cursor from context Query.
func ExtractPaginationCursor(c echo.Context, defaultLimit int) (couchdb.Cursor, error) {

	var limit int

	if limitString := c.QueryParam("page[limit]"); limitString != "" {
		reqLimit, err := strconv.ParseInt(limitString, 10, 32)
		if err != nil {
			return nil, NewError(http.StatusBadRequest, "page limit is not a number")
		}
		limit = int(reqLimit)
	} else {
		limit = defaultLimit
	}

	if cursor := c.QueryParam("page[cursor]"); cursor != "" {
		var parts []interface{}
		err := json.Unmarshal([]byte(cursor), &parts)
		if err != nil {
			return nil, NewError(http.StatusBadRequest, "bad json cursor %s", cursor)
		}

		if len(parts) != 2 {
			return nil, NewError(http.StatusBadRequest, "bad cursor length %s", cursor)
		}
		nextKey := parts[0]
		nextDocID, ok := parts[1].(string)
		if !ok {
			return nil, NewError(http.StatusBadRequest, "bad cursor id %s", cursor)
		}

		return couchdb.NewKeyCursor(limit, nextKey, nextDocID), nil
	}

	if skipString := c.QueryParam("page[skip]"); skipString != "" {
		reqSkip, err := strconv.Atoi(skipString)
		if err != nil {
			return nil, NewError(http.StatusBadRequest, "page skip is not a number")
		}
		return couchdb.NewSkipCursor(limit, reqSkip), nil
	}

	return couchdb.NewKeyCursor(limit, nil, ""), nil
}
