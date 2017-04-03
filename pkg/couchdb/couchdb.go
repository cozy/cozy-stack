package couchdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
	"unicode"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/google/go-querystring/query"
	"github.com/labstack/echo"
)

// InfiniteString is the unicode character "\uFFFF", useful in query as
// a upperbound for string.
const InfiniteString = string(unicode.MaxRune)

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

func rtevent(db Database, evtype string, doc realtime.Doc) {
	realtime.InstanceHub(db.Prefix()).Publish(&realtime.Event{
		Type: evtype,
		Doc:  doc,
	})
}

// GlobalDB is the prefix used for stack-scoped db
var GlobalDB = SimpleDatabasePrefix("global")

// View is the map/reduce thing in CouchDB
type View struct {
	Name    string `json:"-"`
	Doctype string `json:"-"`
	Map     string `json:"map"`
	Reduce  string `json:"reduce,omitempty"`
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
	if id == "" {
		delete(j.M, "_id")
	} else {
		j.M["_id"] = id
	}
}

// SetRev is used to set the revision of the document
func (j JSONDoc) SetRev(rev string) {
	if rev == "" {
		delete(j.M, "_rev")
	} else {
		j.M["_rev"] = rev
	}
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

// Valid implements permissions.Validable on JSONDoc
func (j JSONDoc) Valid(field, value string) bool {
	return fmt.Sprintf("%v", j.Get(field)) == value
}

var couchdbClient = &http.Client{
	Timeout: 5 * time.Second,
}

func unescapeCouchdbName(name string) string {
	return strings.Replace(name, "-", ".", -1)
}

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
		log.Debugf("[couchdb] request: %s %s %s", method, path, string(bytes.TrimSpace(reqjson)))
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
		log.Debugf("[couchdb] error: %s", err.Error())
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
		log.Debugf("[couchdb] response: %s", string(bytes.TrimSpace(data)))
		err = json.Unmarshal(data, &resbody)
	} else {
		err = json.NewDecoder(resp.Body).Decode(&resbody)
	}

	return err
}

// DBStatus responds with informations on the database: size, number of
// documents, sequence numbers, etc.
func DBStatus(db Database, doctype string) (*DBStatusResponse, error) {
	var out DBStatusResponse
	return &out, makeRequest("GET", makeDBName(db, doctype), nil, &out)
}

// AllDoctypes returns a list of all the doctypes that have a database
// on a given instance
func AllDoctypes(db Database) ([]string, error) {
	var dbs []string
	if err := makeRequest("GET", "/_all_dbs", nil, &dbs); err != nil {
		return nil, err
	}
	prefix := escapeCouchdbName(db.Prefix())
	var doctypes []string
	for _, dbname := range dbs {
		parts := strings.SplitAfter(dbname, "/")
		if len(parts) == 2 && parts[0] == prefix {
			doctype := unescapeCouchdbName(parts[1])
			doctypes = append(doctypes, doctype)
		}
	}
	return doctypes, nil
}

// GetDoc fetch a document by its docType and ID, out is filled with
// the document by json.Unmarshal-ing
func GetDoc(db Database, doctype, id string, out Doc) error {
	var err error
	id, err = validateDocID(id)
	if err != nil {
		return err
	}
	return makeRequest("GET", docURL(db, doctype, id), nil, out)
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

	var dbsList []string
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
func ResetDB(db Database, doctype string) error {
	err := DeleteDB(db, doctype)
	if err != nil && !IsNoDatabaseError(err) {
		return err
	}
	return CreateDB(db, doctype)
}

// DeleteDoc deletes a struct implementing the couchb.Doc interface
// If the document's current rev does not match the one passed,
// a CouchdbError(409 conflict) will be returned.
// The document's SetRev will be called with tombstone revision
func DeleteDoc(db Database, doc Doc) error {
	id, err := validateDocID(doc.ID())
	if err != nil {
		return err
	}

	var res updateResponse
	qs := url.Values{"rev": []string{doc.Rev()}}
	url := docURL(db, doc.DocType(), id) + "?" + qs.Encode()
	err = makeRequest("DELETE", url, nil, &res)
	if err != nil {
		return err
	}
	doc.SetRev(res.Rev)
	rtevent(db, realtime.EventDelete, doc)
	return nil
}

// UpdateDoc update a document. The document ID and Rev should be filled.
// The doc SetRev function will be called with the new rev.
func UpdateDoc(db Database, doc Doc) error {
	id, err := validateDocID(doc.ID())
	if err != nil {
		return err
	}
	doctype := doc.DocType()
	if id == "" || doc.Rev() == "" || doctype == "" {
		return fmt.Errorf("UpdateDoc doc argument should have doctype, id and rev")
	}
	url := docURL(db, doctype, id)
	var res updateResponse
	err = makeRequest("PUT", url, doc, &res)
	if err != nil {
		return err
	}
	doc.SetRev(res.Rev)
	rtevent(db, realtime.EventUpdate, doc)
	return nil
}

// CreateNamedDoc persist a document with an ID.
// if the document already exist, it will return a 409 error.
// The document ID should be fillled.
// The doc SetRev function will be called with the new rev.
func CreateNamedDoc(db Database, doc Doc) error {
	id, err := validateDocID(doc.ID())
	if err != nil {
		return err
	}
	doctype := doc.DocType()
	if doc.Rev() != "" || id == "" || doctype == "" {
		return fmt.Errorf("CreateNamedDoc should have type and id but no rev")
	}
	url := docURL(db, doctype, id)
	var res updateResponse
	err = makeRequest("PUT", url, doc, &res)
	if err != nil {
		return err
	}
	doc.SetRev(res.Rev)
	rtevent(db, realtime.EventCreate, doc)
	return nil
}

// CreateNamedDocWithDB is equivalent to CreateNamedDoc but creates the database
// if it does not exist
func CreateNamedDocWithDB(db Database, doc Doc) error {
	err := CreateNamedDoc(db, doc)
	if IsNoDatabaseError(err) {
		err = CreateDB(db, doc.DocType())
		if err != nil {
			return err
		}
		return CreateNamedDoc(db, doc)
	}
	return err
}

func createDocOrDb(db Database, doc Doc, response interface{}) error {
	doctype := doc.DocType()
	dbname := makeDBName(db, doctype)
	err := makeRequest("POST", dbname, doc, response)
	if err == nil || !IsNoDatabaseError(err) {
		return err
	}
	err = CreateDB(db, doctype)
	if err == nil {
		err = makeRequest("POST", dbname, doc, response)
	}
	return err
}

// CreateDoc is used to persist the given document in the couchdb
// database. The document's SetRev and SetID function will be called
// with the document's new ID and Rev.
// This function creates a database if this is the first document of its type
func CreateDoc(db Database, doc Doc) error {
	var res *updateResponse

	if doc.ID() != "" {
		return newDefinedIDError()
	}

	err := createDocOrDb(db, doc, &res)
	if err != nil {
		return err
	} else if !res.Ok {
		return fmt.Errorf("CouchDB replied with 200 ok=false")
	}

	doc.SetID(res.ID)
	doc.SetRev(res.Rev)
	rtevent(db, realtime.EventCreate, doc)
	return nil
}

// DefineViews creates a design doc with some views
func DefineViews(db Database, views []*View) error {
	// group views by doctype
	grouped := make(map[string]map[string]*View)
	for _, v := range views {
		g, ok := grouped[v.Doctype]
		if !ok {
			g = make(map[string]*View)
			grouped[v.Doctype] = g
		}
		g[v.Name] = v
	}
	for doctype, views := range grouped {
		url := makeDBName(db, doctype) + "/_design/" + doctype
		doc := struct {
			Lang  string           `json:"language"`
			Views map[string]*View `json:"views"`
		}{
			"javascript",
			views,
		}
		err := makeRequest("PUT", url, &doc, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

// ExecView executes the specified view function
func ExecView(db Database, view *View, req *ViewRequest, results interface{}) error {
	viewurl := fmt.Sprintf("%s/_design/%s/_view/%s", makeDBName(db, view.Doctype), view.Doctype, view.Name)
	// Keys request
	if req.Keys != nil {
		return makeRequest("POST", viewurl, req, &results)
	}
	v, err := req.Values()
	if err != nil {
		return err
	}
	viewurl += "?" + v.Encode()
	return makeRequest("GET", viewurl, nil, &results)
}

// DefineIndex define the index on the doctype database
// see query package on how to define an index
func DefineIndex(db Database, index *mango.Index) error {
	_, err := DefineIndexRaw(db, index.Doctype, index.Request)
	return err
}

// DefineIndexRaw defines a index
func DefineIndexRaw(db Database, doctype string, index interface{}) (*IndexCreationResponse, error) {
	url := makeDBName(db, doctype) + "/_index"
	response := &IndexCreationResponse{}
	if err := makeRequest("POST", url, &index, &response); err != nil {
		return nil, err
	}
	return response, nil
}

// DefineIndexes defines a list of indexes
func DefineIndexes(db Database, indexes []*mango.Index) error {
	for _, index := range indexes {
		if err := DefineIndex(db, index); err != nil {
			return err
		}
	}
	return nil
}

// FindDocs returns all documents matching the passed FindRequest
// documents will be unmarshalled in the provided results slice.
func FindDocs(db Database, doctype string, req *FindRequest, results interface{}) error {
	return FindDocsRaw(db, doctype, req, results)
}

// FindDocsRaw find documents
// TODO: pagination
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

// GetAllDocs returns all documents of a specified doctype. It filters
// out the possible _design document.
// TODO: pagination
func GetAllDocs(db Database, doctype string, req *AllDocsRequest, results interface{}) error {
	v, err := query.Values(req)
	if err != nil {
		return err
	}
	v.Add("include_docs", "true")

	var response AllDocsResponse
	url := makeDBName(db, doctype) + "/_all_docs?" + v.Encode()
	err = makeRequest("POST", url, &req, &response)
	if err != nil {
		return err
	}

	var docs []json.RawMessage
	for _, row := range response.Rows {
		if !strings.HasPrefix(row.ID, "_design") {
			docs = append(docs, row.Doc)
		}
	}
	// TODO: better way to unmarshal returned data. For now we re-
	// marshal the doc fields a a json array before unmarshalling it
	// again...
	data, err := json.Marshal(docs)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, results)
}

// Proxy generate a httputil.ReverseProxy which forwards the request to the
// correct route.
func Proxy(db Database, doctype, path string) *httputil.ReverseProxy {
	// discard error, it is checked in config
	couchurl, _ := url.Parse(config.CouchURL())

	director := func(req *http.Request) {
		req.URL.Scheme = couchurl.Scheme
		req.URL.Host = couchurl.Host
		req.Header.Del(echo.HeaderAuthorization) // drop stack auth
		req.URL.RawPath = "/" + makeDBName(db, doctype) + "/" + path
		req.URL.Path, _ = url.QueryUnescape(req.URL.RawPath)
	}

	return &httputil.ReverseProxy{
		Director: director,
	}
}

func validateDocID(id string) (string, error) {
	if len(id) > 0 && id[0] == '_' {
		return "", newBadIDError(id)
	}
	return id, nil
}

// IndexCreationResponse is the response from couchdb when we create an Index
type IndexCreationResponse struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	Reason string `json:"reason,omitempty"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
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

// FindRequest is used to build a find request
type FindRequest struct {
	Selector mango.Filter  `json:"selector"`
	UseIndex string        `json:"use_index,omitempty"`
	Limit    int           `json:"limit,omitempty"`
	Skip     int           `json:"skip,omitempty"`
	Sort     *mango.SortBy `json:"sort,omitempty"`
	Fields   []string      `json:"fields,omitempty"`
}

// AllDocsRequest is used to build a _all_docs request
type AllDocsRequest struct {
	Descending bool     `url:"descending,omitempty"`
	Keys       []string `url:"keys,omitempty"`
	Limit      int      `url:"limit,omitempty"`
	Skip       int      `url:"skip,omitempty"`
	StartKey   string   `url:"start_key,omitempty"`
	EndKey     string   `url:"end_key,omitempty"`
}

// AllDocsResponse is the response we receive from an _all_docs request
type AllDocsResponse struct {
	Offset    int `json:"offset"`
	TotalRows int `json:"total_rows"`
	Rows      []struct {
		ID  string          `json:"id"`
		Doc json.RawMessage `json:"doc"`
	} `json:"rows"`
}

// ViewRequest are all params that can be passed to a view
// It can be encoded either as a POST-json or a GET-url.
type ViewRequest struct {
	Key      interface{} `json:"key,omitempty" url:"key,omitempty"`
	StartKey interface{} `json:"start_key,omitempty" url:"start_key,omitempty"`
	EndKey   interface{} `json:"end_key,omitempty" url:"end_key,omitempty"`

	StartKeyDocID string `json:"startkey_docid,omitempty" url:"startkey_docid,omitempty"`
	EndKeyDocID   string `json:"endkey_docid,omitempty" url:"endkey_docid,omitempty"`

	// Keys cannot be used in url mode
	Keys []interface{} `json:"keys,omitempty" url:"-"`

	Limit       int  `json:"limit,omitempty" url:"limit,omitempty"`
	Skip        int  `json:"skip,omitempty" url:"skip,omitempty"`
	Descending  bool `json:"descending,omitempty" url:"descending,omitempty"`
	IncludeDocs bool `json:"include_docs,omitempty" url:"include_docs,omitempty"`

	InclusiveEnd bool `json:"inclusive_end,omitempty" url:"inclusive_end,omitempty"`

	Reduce     bool `json:"reduce" url:"reduce"`
	GroupLevel int  `json:"group_level,omitempty" url:"group_level,omitempty"`
}

// ViewResponse is the response we receive when executing a view
type ViewResponse struct {
	Total int `json:"total_rows"`
	Rows  []struct {
		ID    string           `json:"id"`
		Key   interface{}      `json:"key"`
		Value interface{}      `json:"value"`
		Doc   *json.RawMessage `json:"doc"`
	} `json:"rows"`
}

// DBStatusResponse is the response from DBStatus
type DBStatusResponse struct {
	DBName    string `json:"db_name"`
	UpdateSeq string `json:"update_seq"`
	Sizes     struct {
		File     int `json:"file"`
		External int `json:"external"`
		Active   int `json:"active"`
	} `json:"sizes"`
	PurgeSeq int `json:"purge_seq"`
	Other    struct {
		DataSize int `json:"data_size"`
	} `json:"other"`
	DocDelCount       int    `json:"doc_del_count"`
	DocCount          int    `json:"doc_count"`
	DiskSize          int    `json:"disk_size"`
	DiskFormatVersion int    `json:"disk_format_version"`
	DataSize          int    `json:"data_size"`
	CompactRunning    bool   `json:"compact_running"`
	InstanceStartTime string `json:"instance_start_time"`
}
