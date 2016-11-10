package couchdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/config"
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

// Database is the type passed to every function in couchdb package
// for now it is just a string with the database prefix.
type Database interface {
	Prefix() string
}

// SimpleDatabase implements the Database interface
type simpleDB struct{ prefix string }

// Prefix implements the Database interface on simpleDB
func (sdb *simpleDB) Prefix() string { return sdb.prefix + "/" }

// SimpleDatabasePrefix returns a Database from a prefix, useful for test
func SimpleDatabasePrefix(prefix string) Database {
	return &simpleDB{prefix}
}

// GlobalDB is the prefix used for stack-scoped db
var GlobalDB = SimpleDatabasePrefix("global")

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

var couchdbClient = &http.Client{}

func escapeCouchdbName(name string) string {
	name = strings.Replace(name, ".", "-", -1)
	name = strings.Replace(name, ":", "-", -1)
	return strings.ToLower(name)
}

func makeDBName(db Database, doctype string) string {
	// @TODO This should be better analysed
	dbname := escapeCouchdbName(db.Prefix() + doctype)
	return url.QueryEscape(dbname)
}

func dbNameHasPrefix(dbname, dbprefix string) (bool, string) {
	dbprefix = escapeCouchdbName(dbprefix)
	if !strings.HasPrefix(dbname, dbprefix) {
		return false, ""
	}
	return true, strings.Replace(dbname, dbprefix, "", 1)
}

func docURL(db Database, doctype, id string) string {
	return makeDBName(db, doctype) + "/" + url.QueryEscape(id)
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

	if log.GetLevel() == log.DebugLevel {
		log.Debugf("[couchdb request] %s %s %s", method, path, string(reqjson))
	}

	req, err := http.NewRequest(method, config.CouchURL()+path, bytes.NewReader(reqjson))
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			err = newIOReadError(err)
		} else {
			err = newCouchdbError(resp.StatusCode, body)
		}
		log.Debugf("[couchdb error] %s", err.Error())
		return err
	}

	if resbody == nil {
		return nil
	}

	if log.GetLevel() == log.DebugLevel {
		var data []byte
		data, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		log.Debugf("[couchdb response] %s", string(data))
		err = json.Unmarshal(data, &resbody)
	} else {
		err = json.NewDecoder(resp.Body).Decode(&resbody)
	}

	return err
}

func fixErrorNoDatabaseIsWrongDoctype(err error) {
	if IsNoDatabaseError(err) {
		err.(*Error).Reason = "wrong_doctype"
	}
}

// GetDoc fetch a document by its docType and ID, out is filled with
// the document by json.Unmarshal-ing
func GetDoc(db Database, doctype, id string, out Doc) error {
	err := makeRequest("GET", docURL(db, doctype, id), nil, out)
	fixErrorNoDatabaseIsWrongDoctype(err)
	return err
}

// CreateDB creates the necessary database for a doctype
func CreateDB(db Database, doctype string) error {
	return makeRequest("PUT", makeDBName(db, doctype), nil, nil)
}

// DeleteDB destroy the database for a doctype
func DeleteDB(db Database, doctype string) error {
	return makeRequest("DELETE", makeDBName(db, doctype), nil, nil)
}

// DeleteAllDBs will remove all the couchdb doctype databases for
// a couchdb.DB.
func DeleteAllDBs(db Database) error {

	dbprefix := db.Prefix()

	if dbprefix == "" || dbprefix[len(dbprefix)-1] != '/' {
		return fmt.Errorf("You need to provide the database prefix name ending with /")
	}

	dbsList := make([]string, 0)
	err := makeRequest("GET", "_all_dbs", nil, &dbsList)
	if err != nil {
		return err
	}

	for _, doctypedb := range dbsList {
		hasPrefix, doctype := dbNameHasPrefix(doctypedb, dbprefix)
		if !hasPrefix {
			continue
		}
		if err = DeleteDB(db, doctype); err != nil {
			return err
		}
	}

	return nil
}

// ResetDB destroy and recreate the database for a doctype
func ResetDB(db Database, doctype string) (err error) {
	err = DeleteDB(db, doctype)
	if err != nil && !IsNoDatabaseError(err) {
		return err
	}
	return CreateDB(db, doctype)
}

// Delete destroy a document by its doctype and ID .
// If the document's current rev does not match the one passed,
// a CouchdbError(409 conflict) will be returned.
// This functions returns the tombstone revision as string
func Delete(db Database, doctype, id, rev string) (tombrev string, err error) {
	var res updateResponse
	qs := url.Values{"rev": []string{rev}}
	url := docURL(db, doctype, id) + "?" + qs.Encode()
	err = makeRequest("DELETE", url, nil, &res)
	fixErrorNoDatabaseIsWrongDoctype(err)
	if err == nil {
		tombrev = res.Rev
	}
	return
}

// DeleteDoc deletes a struct implementing the couchb.Doc interface
// The document's SetRev will be called with tombstone revision
func DeleteDoc(db Database, doc Doc) (err error) {
	doctype := doc.DocType()
	id := doc.ID()
	rev := doc.Rev()
	tombrev, err := Delete(db, doctype, id, rev)
	if err == nil {
		doc.SetRev(tombrev)
	}
	return
}

// UpdateDoc update a document. The document ID and Rev should be fillled.
// The doc SetRev function will be called with the new rev.
func UpdateDoc(db Database, doc Doc) (err error) {
	doctype := doc.DocType()
	id := doc.ID()
	rev := doc.Rev()
	if id == "" || rev == "" || doctype == "" {
		return fmt.Errorf("UpdateDoc doc argument should have doctype, id and rev")
	}

	url := docURL(db, doctype, id)
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
func CreateNamedDoc(db Database, doc Doc) (err error) {
	doctype := doc.DocType()
	id := doc.ID()

	if doc.Rev() != "" || doc.ID() == "" || doctype == "" {
		return fmt.Errorf("CreateNamedDoc should have type and id but no rev")
	}

	url := docURL(db, doctype, id)
	var res updateResponse
	err = makeRequest("PUT", url, doc, &res)
	fixErrorNoDatabaseIsWrongDoctype(err)
	if err == nil {
		doc.SetRev(res.Rev)
	}
	return err
}

// CreateNamedDocWithDB is equivalent to CreateNamedDoc but creates the database
// if it does not exist
func CreateNamedDocWithDB(db Database, doc Doc) (err error) {
	err = CreateNamedDoc(db, doc)
	if coucherr, ok := err.(*Error); ok && coucherr.Reason == "wrong_doctype" {
		err = CreateDB(db, doc.DocType())
		if err != nil {
			return err
		}
		return CreateNamedDoc(db, doc)
	}
	return err
}

func createDocOrDb(db Database, doc Doc, response interface{}) (err error) {
	doctype := doc.DocType()
	dbname := makeDBName(db, doctype)
	err = makeRequest("POST", dbname, doc, response)
	if err == nil || !IsNoDatabaseError(err) {
		return
	}

	err = CreateDB(db, doctype)
	if err == nil {
		err = makeRequest("POST", dbname, doc, response)
	}
	return
}

// CreateDoc is used to persist the given document in the couchdb
// database. The document's SetRev and SetID function will be called
// with the document's new ID and Rev.
// This function creates a database if this is the first document of its type
func CreateDoc(db Database, doc Doc) (err error) {
	var res *updateResponse

	if doc.ID() != "" {
		err = fmt.Errorf("Can not create document with a defined ID")
		return
	}

	err = createDocOrDb(db, doc, &res)
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
func DefineIndex(db Database, doctype string, index mango.Index) error {
	_, err := DefineIndexRaw(db, doctype, &index)
	return err
}

// DefineIndexRaw defines a index
func DefineIndexRaw(db Database, doctype string, index interface{}) (*IndexCreationResponse, error) {
	url := makeDBName(db, doctype) + "/_index"
	var response IndexCreationResponse
	return &response, makeRequest("POST", url, &index, &response)
}

// FindDocs returns all documents matching the passed FindRequest
// documents will be unmarshalled in the provided results slice.
func FindDocs(db Database, doctype string, req *FindRequest, results interface{}) error {
	return FindDocsRaw(db, doctype, req, results)
}

// FindDocsRaw find documents
func FindDocsRaw(db Database, doctype string, req interface{}, results interface{}) error {
	url := makeDBName(db, doctype) + "/_find"
	// prepare a structure to receive the results
	var response findResponse
	err := makeRequest("POST", url, &req, &response)
	if err != nil {
		return err
	}
	if response.Warning != "" {
		// Developer should not rely on unoptimized index.
		return unoptimalError()
	}
	return json.Unmarshal(response.Docs, results)
}

// IndexCreationResponse is the response from couchdb when we create an Index
type IndexCreationResponse struct {
	Result string `json:"result"`
	Error  string `json:"error"`
	Reason string `json:"reason"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

type updateResponse struct {
	ID  string `json:"id"`
	Rev string `json:"rev"`
	Ok  bool   `json:"ok"`
}

type findResponse struct {
	Warning string          `json:"warning"`
	Docs    json.RawMessage `json:"docs"`
}

// A FindRequest is a structure containin
type FindRequest struct {
	Selector mango.Filter  `json:"selector"`
	Limit    int           `json:"limit,omitempty"`
	Skip     int           `json:"skip,omitempty"`
	Sort     *mango.SortBy `json:"sort,omitempty"`
	Fields   []string      `json:"fields,omitempty"`
}
