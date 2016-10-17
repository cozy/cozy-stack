package couchdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/couchdb/mango"
)

// Doc is the interface that encapsulate a couchdb document, of any
// serializable type. This interface defines method to set and get the
// ID of the document.
type Doc interface {
	ID() string
	Rev() string
	DocType() string

	SetID(id string)
	SetRev(rev string)
}

// JSONDoc is a map representing a simple json object that implements
// the Doc interface.
type JSONDoc struct {
	M    map[string]interface{}
	Type string
}

// ID returns the identifier field of the document
//   "io.cozy.event/123abc123" == doc.ID()
func (j JSONDoc) ID() string {
	id, ok := j.M["_id"].(string)
	if ok {
		return id
	}
	return ""
}

// Rev returns the revision field of the document
//   "3-1234def1234" == doc.Rev()
func (j JSONDoc) Rev() string {
	rev, ok := j.M["_rev"].(string)
	if ok {
		return rev
	}
	return ""
}

// DocType returns the document type of the document
//   "io.cozy.event" == doc.Doctype()
func (j JSONDoc) DocType() string {
	return j.Type
}

// SetID is used to set the identifier of the document
func (j JSONDoc) SetID(id string) {
	j.M["_id"] = id
}

// SetRev is used to set the revision of the document
func (j JSONDoc) SetRev(rev string) {
	j.M["_rev"] = rev
}

// MarshalJSON implements json.Marshaller by proxying to internal map
func (j JSONDoc) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.M)
}

// UnmarshalJSON implements json.Unmarshaller by proxying to internal map
func (j *JSONDoc) UnmarshalJSON(bytes []byte) error {
	err := json.Unmarshal(bytes, &j.M)
	if err != nil {
		return err
	}
	doctype, ok := j.M["_type"].(string)
	if ok {
		j.Type = doctype
	}
	delete(j.M, "_type")
	return nil
}

// ToMapWithType returns the JSONDoc internal map including its DocType
// its used in request response.
func (j *JSONDoc) ToMapWithType() map[string]interface{} {
	j.M["_type"] = j.DocType()
	return j.M
}

// Get returns the value of one of the db fields
func (j JSONDoc) Get(key string) interface{} {
	return j.M[key]
}

// CouchURL is the URL where to check if CouchDB is up
func CouchURL() string {
	return "http://localhost:5984/"
}

var couchdbClient = &http.Client{}

func makeDBName(dbprefix, doctype string) string {
	// @TODO This should be better analysed
	dbname := dbprefix + doctype
	dbname = strings.Replace(dbname, ".", "-", -1)
	dbname = strings.ToLower(dbname)
	return url.QueryEscape(dbname)
}

func docURL(dbprefix, doctype, id string) string {
	return makeDBName(dbprefix, doctype) + "/" + url.QueryEscape(id)
}

func makeRequest(method, path string, reqbody interface{}, resbody interface{}) error {
	var reqjson []byte
	var err error

	if reqbody != nil {
		reqjson, err = json.Marshal(reqbody)
		if err != nil {
			return err
		}
	}

	fmt.Printf("[couchdb request] %v %v %v\n", method, path, string(reqjson))

	req, err := http.NewRequest(method, CouchURL()+path, bytes.NewReader(reqjson))
	// Possible err = wrong method, unparsable url
	if err != nil {
		return newRequestError(err)
	}
	if reqbody != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	req.Header.Add("Accept", "application/json")
	resp, err := couchdbClient.Do(req)
	// Possible err = mostly connection failure
	if err != nil {
		return newConnectionError(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	// Possible err = mostly connection failure (hangup)
	if err != nil {
		return newIOReadError(err)
	}

	fmt.Printf("[couchdb response] %v\n", string(body))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Couchdb as returned an error HTTP status code
		return newCouchdbError(resp.StatusCode, body)
	}

	if resbody == nil {
		// dont care about the return value
		return nil
	}
	err = json.Unmarshal(body, &resbody)
	return err
}

func fixErrorNoDatabaseIsWrongDoctype(err error) {
	if IsNoDatabaseError(err) {
		err.(*Error).Reason = "wrong_doctype"
	}
}

// GetDoc fetch a document by its docType and ID, out is filled with
// the document by json.Unmarshal-ing
func GetDoc(dbprefix, doctype, id string, out Doc) error {
	err := makeRequest("GET", docURL(dbprefix, doctype, id), nil, out)
	fixErrorNoDatabaseIsWrongDoctype(err)
	return err
}

// CreateDB creates the necessary database for a doctype
func CreateDB(dbprefix, doctype string) error {
	return makeRequest("PUT", makeDBName(dbprefix, doctype), nil, nil)
}

// DeleteDB destroy the database for a doctype
func DeleteDB(dbprefix, doctype string) error {
	return makeRequest("DELETE", makeDBName(dbprefix, doctype), nil, nil)
}

// ResetDB destroy and recreate the database for a doctype
func ResetDB(dbprefix, doctype string) (err error) {
	err = DeleteDB(dbprefix, doctype)
	if err != nil && !IsNoDatabaseError(err) {
		return err
	}
	return CreateDB(dbprefix, doctype)
}

// Delete destroy a document by its doctype and ID .
// If the document's current rev does not match the one passed,
// a CouchdbError(409 conflict) will be returned.
// This functions returns the tombstone revision as string
func Delete(dbprefix, doctype, id, rev string) (tombrev string, err error) {
	var res updateResponse
	qs := url.Values{"rev": []string{rev}}
	url := docURL(dbprefix, doctype, id) + "?" + qs.Encode()
	err = makeRequest("DELETE", url, nil, &res)
	fixErrorNoDatabaseIsWrongDoctype(err)
	if err == nil {
		tombrev = res.Rev
	}
	return
}

// DeleteDoc deletes a struct implementing the couchb.Doc interface
// The document's SetRev will be called with tombstone revision
func DeleteDoc(dbprefix string, doc Doc) (err error) {
	doctype := doc.DocType()
	id := doc.ID()
	rev := doc.Rev()
	tombrev, err := Delete(dbprefix, doctype, id, rev)
	if err == nil {
		doc.SetRev(tombrev)
	}
	return
}

// UpdateDoc update a document. The document ID and Rev should be fillled.
// The doc SetRev function will be called with the new rev.
func UpdateDoc(dbprefix string, doc Doc) (err error) {
	doctype := doc.DocType()
	id := doc.ID()
	rev := doc.Rev()
	if id == "" || rev == "" || doctype == "" {
		return fmt.Errorf("UpdateDoc doc argument should have doctype, id and rev")
	}

	url := docURL(dbprefix, doctype, id)
	var res updateResponse
	err = makeRequest("PUT", url, doc, &res)
	fixErrorNoDatabaseIsWrongDoctype(err)
	if err == nil {
		doc.SetRev(res.Rev)
	}
	return err
}

// CreateNamedDoc persist a document with an ID.
// if the document already exist, it will return a 409 error.
// The document ID should be fillled.
// The doc SetRev function will be called with the new rev.
func CreateNamedDoc(dbprefix string, doc Doc) (err error) {
	doctype := doc.DocType()
	id := doc.ID()

	if doc.Rev() != "" || doc.ID() == "" || doctype == "" {
		return fmt.Errorf("CreateNamedDoc should have type and id but no rev")
	}

	url := docURL(dbprefix, doctype, id)
	var res updateResponse
	err = makeRequest("PUT", url, doc, &res)
	fixErrorNoDatabaseIsWrongDoctype(err)
	if err == nil {
		doc.SetRev(res.Rev)
	}
	return err
}

func createDocOrDb(dbprefix string, doc Doc, response interface{}) (err error) {
	doctype := doc.DocType()
	db := makeDBName(dbprefix, doctype)
	err = makeRequest("POST", db, doc, response)
	if err == nil || !IsNoDatabaseError(err) {
		return
	}

	err = CreateDB(dbprefix, doctype)
	if err == nil {
		err = makeRequest("POST", db, doc, response)
	}
	return
}

// CreateDoc is used to persist the given document in the couchdb
// database. The document's SetRev and SetID function will be called
// with the document's new ID and Rev.
// This function creates a database if this is the first document of its type
func CreateDoc(dbprefix string, doc Doc) (err error) {
	var res *updateResponse

	if doc.ID() != "" {
		err = fmt.Errorf("Can not create document with a defined ID")
		return
	}

	err = createDocOrDb(dbprefix, doc, &res)
	if err != nil {
		return err
	} else if !res.Ok {
		return fmt.Errorf("CouchDB replied with 200 ok=false")
	}

	doc.SetID(res.ID)
	doc.SetRev(res.Rev)
	return nil
}

// DefineIndex define the index on the doctype database
// see query package on how to define an index
func DefineIndex(dbprefix, doctype string, index mango.IndexDefinitionRequest) error {
	url := makeDBName(dbprefix, doctype) + "/_index"
	var response indexCreationResponse
	return makeRequest("POST", url, &index, &response)
}

// FindDocs returns all documents matching the passed FindRequest
// documents will be unmarshalled in the provided results slice.
func FindDocs(dbprefix, doctype string, req *FindRequest, results interface{}) error {
	url := makeDBName(dbprefix, doctype) + "/_find"
	// prepare a structure to receive the results
	var response findResponse
	err := makeRequest("POST", url, &req, &response)
	if err != nil {
		return err
	}
	return json.Unmarshal(response.Docs, results)
}

type indexCreationResponse struct {
	Result string `json:"result"`
	Error  string `json:"error"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

type updateResponse struct {
	ID  string `json:"id"`
	Rev string `json:"rev"`
	Ok  bool   `json:"ok"`
}

type findResponse struct {
	Docs json.RawMessage `json:"docs"`
}

// A FindRequest is a structure containin
type FindRequest struct {
	Selector mango.Filter  `json:"selector"`
	Limit    int           `json:"limit,omitempty"`
	Skip     int           `json:"skip,omitempty"`
	Sort     *mango.SortBy `json:"sort,omitempty"`
	Fields   []string      `json:"fields,omitempty"`
}
