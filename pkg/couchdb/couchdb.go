package couchdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/sirupsen/logrus"
)

// MaxString is the unicode character "\uFFFF", useful in query as
// a upperbound for string.
const MaxString = mango.MaxString

// SelectorReferencedBy is the string constant for the references in a JSON
// document.
const SelectorReferencedBy = "referenced_by"

// accountDocType is equal to consts.Accounts which we cannot import without
// a circular dependency for now.
const accountDocType = "io.cozy.accounts"

// Doc is the interface that encapsulate a couchdb document, of any
// serializable type. This interface defines method to set and get the
// ID of the document.
type Doc interface {
	ID() string
	Rev() string
	DocType() string
	Clone() Doc

	SetID(id string)
	SetRev(rev string)
}

// Database is the type passed to every function in couchdb package
// for now it is just a string with the database prefix.
type Database prefixer.Prefixer

// newDatabase returns a Database from a prefix, useful for test
func newDatabase(prefix string) prefixer.Prefixer {
	return prefixer.NewPrefixer("", prefix)
}

// RTEvent published a realtime event for a couchDB change
func RTEvent(db Database, verb string, doc, oldDoc Doc) {
	if err := runHooks(db, verb, doc, oldDoc); err != nil {
		logger.WithDomain(db.DomainName()).WithField("nspace", "couchdb").
			Errorf("error in hooks on %s %s %v\n", verb, doc.DocType(), err)
	}
	docClone := doc.Clone()
	go realtime.GetHub().Publish(db, verb, docClone, oldDoc)
}

// GlobalDB is the prefix used for stack-scoped db
var GlobalDB = newDatabase("global")

// GlobalSecretsDB is the the prefix used for db which hold
// a cozy stack secrets.
var GlobalSecretsDB = newDatabase("secrets")

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

// Clone is used to create a copy of the document
func (j JSONDoc) Clone() Doc {
	cloned := JSONDoc{Type: j.Type}
	cloned.M = deepClone(j.M)
	return cloned
}

func deepClone(m map[string]interface{}) map[string]interface{} {
	clone := make(map[string]interface{}, len(m))
	for k, v := range m {
		if vv, ok := v.(map[string]interface{}); ok {
			clone[k] = deepClone(vv)
		} else if vv, ok := v.([]interface{}); ok {
			clone[k] = deepCloneSlice(vv)
		} else {
			clone[k] = v
		}
	}
	return clone
}

func deepCloneSlice(s []interface{}) []interface{} {
	clone := make([]interface{}, len(s))
	for i, v := range s {
		if vv, ok := v.(map[string]interface{}); ok {
			clone[i] = deepClone(vv)
		} else if vv, ok := v.([]interface{}); ok {
			clone[i] = deepCloneSlice(vv)
		} else {
			clone[i] = v
		}
	}
	return clone
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

// Match implements permissions.Matcher on JSONDoc.
//
// The `referenced_by` selector is a special case: the `values` field of such
// rule has the format "doctype/id" and it cannot directly be compared to the
// same field of a JSONDoc since, in the latter, the format is:
// "referenced_by": [
//     {"type": "doctype1", "id": "id1"},
//     {"type": "doctype2", "id": "id2"},
// ]
func (j JSONDoc) Match(field, value string) bool {
	if field == SelectorReferencedBy {
		rawReferences := j.Get(field)
		references, ok := rawReferences.([]interface{})
		if !ok {
			return false
		}

		values := strings.Split(value, "/")
		if len(values) != 2 {
			return false
		}
		valueType, valueID := values[0], values[1]

		for _, ref := range references {
			reference := ref.(map[string]interface{})
			if valueType == reference["type"].(string) &&
				valueID == reference["id"].(string) {
				return true
			}
		}

		return false
	}

	return fmt.Sprintf("%v", j.Get(field)) == value
}

var couchdbClient = &http.Client{
	Timeout: 10 * time.Second,
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
	dbname := escapeCouchdbName(db.DBPrefix() + "/" + doctype)
	return url.PathEscape(dbname)
}

func dbNameHasPrefix(dbname, dbprefix string) (bool, string) {
	dbprefix = escapeCouchdbName(dbprefix + "/")
	if !strings.HasPrefix(dbname, dbprefix) {
		return false, ""
	}
	return true, strings.Replace(dbname, dbprefix, "", 1)
}

func makeRequest(db Database, doctype, method, path string, reqbody interface{}, resbody interface{}) error {
	var reqjson []byte
	var err error

	if reqbody != nil {
		reqjson, err = json.Marshal(reqbody)
		if err != nil {
			return err
		}
	}

	if doctype != "" {
		path = makeDBName(db, doctype) + "/" + path
	}

	log := logger.WithDomain(db.DomainName()).WithField("nspace", "couchdb")

	// We do not log the account doctype to avoid printing account informations
	// in the log files.
	logDebug := doctype != accountDocType && logger.IsDebug(log)

	if logDebug {
		log.Debugf("request: %s %s %s", method, path, string(bytes.TrimSpace(reqjson)))
	}

	req, err := http.NewRequest(
		method,
		config.CouchURL().String()+path,
		bytes.NewReader(reqjson),
	)
	// Possible err = wrong method, unparsable url
	if err != nil {
		return newRequestError(err)
	}
	req.Header.Add("Accept", "application/json")
	if reqbody != nil {
		req.Header.Add("Content-Type", "application/json")
	}

	auth := config.GetConfig().CouchDB.Auth
	if auth != nil {
		if p, ok := auth.Password(); ok {
			req.SetBasicAuth(auth.Username(), p)
		}
	}
	start := time.Now()
	resp, err := couchdbClient.Do(req)
	elapsed := time.Since(start)
	// Possible err = mostly connection failure
	if err != nil {
		err = newConnectionError(err)
		log.Error(err.Error())
		return err
	}
	defer resp.Body.Close()

	if elapsed.Seconds() >= 10 {
		log.Printf("slow request on %s %s (%s)", method, path, elapsed)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			err = newIOReadError(err)
			log.Error(err.Error())
		} else {
			err = newCouchdbError(resp.StatusCode, body)
			log.Debug(err.Error())
		}
		return err
	}
	if resbody == nil {
		return nil
	}

	if logDebug {
		var data []byte
		data, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		log.Debugf("response: %s", string(bytes.TrimSpace(data)))
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
	return &out, makeRequest(db, doctype, http.MethodGet, "", nil, &out)
}

// AllDoctypes returns a list of all the doctypes that have a database
// on a given instance
func AllDoctypes(db Database) ([]string, error) {
	var dbs []string
	if err := makeRequest(db, "", http.MethodGet, "_all_dbs", nil, &dbs); err != nil {
		return nil, err
	}
	prefix := escapeCouchdbName(db.DBPrefix())
	var doctypes []string
	for _, dbname := range dbs {
		parts := strings.Split(dbname, "/")
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
	if id == "" {
		return fmt.Errorf("Missing ID for GetDoc")
	}
	return makeRequest(db, doctype, http.MethodGet, url.PathEscape(id), nil, out)
}

// GetDocRev fetch a document by its docType and ID on a specific revision, out
// is filled with the document by json.Unmarshal-ing
func GetDocRev(db Database, doctype, id, rev string, out Doc) error {
	var err error
	id, err = validateDocID(id)
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("Missing ID for GetDoc")
	}
	url := url.PathEscape(id) + "?rev=" + url.QueryEscape(rev)
	return makeRequest(db, doctype, http.MethodGet, url, nil, out)
}

// EnsureDBExist creates the database for the doctype if it doesn't exist
func EnsureDBExist(db Database, doctype string) error {
	if _, err := DBStatus(db, doctype); IsNoDatabaseError(err) {
		if err = CreateDB(db, doctype); err != nil {
			_, err = DBStatus(db, doctype)
			return err
		}
	}
	return nil
}

// CreateDB creates the necessary database for a doctype
func CreateDB(db Database, doctype string) error {
	return makeRequest(db, doctype, http.MethodPut, "", nil, nil)
}

// DeleteDB destroy the database for a doctype
func DeleteDB(db Database, doctype string) error {
	return makeRequest(db, doctype, http.MethodDelete, "", nil, nil)
}

// DeleteAllDBs will remove all the couchdb doctype databases for
// a couchdb.DB.
func DeleteAllDBs(db Database) error {
	dbprefix := db.DBPrefix()
	if dbprefix == "" {
		return fmt.Errorf("You need to provide a valid database")
	}

	var dbsList []string
	err := makeRequest(db, "", http.MethodGet, "_all_dbs", nil, &dbsList)
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
	if id == "" {
		return fmt.Errorf("Missing ID for DeleteDoc")
	}
	old := doc.Clone()

	// XXX Specific log for the deletion of an account, to help monitor this
	// metric.
	if doc.DocType() == accountDocType {
		logger.WithDomain(db.DomainName()).
			WithFields(logrus.Fields{
				"log_id":      "account_delete",
				"account_id":  doc.ID(),
				"account_rev": doc.Rev(),
				"nspace":      "couchb",
			}).
			Infof("Deleting account %s", doc.ID())
	}

	var res UpdateResponse
	url := url.PathEscape(id) + "?rev=" + url.QueryEscape(doc.Rev())
	err = makeRequest(db, doc.DocType(), http.MethodDelete, url, nil, &res)
	if err != nil {
		return err
	}
	doc.SetRev(res.Rev)
	RTEvent(db, realtime.EventDelete, doc, old)
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

	url := url.PathEscape(id)
	// The old doc is requested to be emitted thought RTEvent.
	// This is useful to keep track of the modifications for the triggers.
	oldDoc := doc.Clone()
	err = makeRequest(db, doctype, http.MethodGet, url, nil, oldDoc)
	if err != nil {
		return err
	}
	var res UpdateResponse
	err = makeRequest(db, doctype, http.MethodPut, url, doc, &res)
	if err != nil {
		return err
	}
	doc.SetRev(res.Rev)
	RTEvent(db, realtime.EventUpdate, doc, oldDoc)
	return nil
}

// UpdateDocWithOld updates a document, like UpdateDoc. The difference is that
// if we already have oldDoc there is no need to refetch it from database.
func UpdateDocWithOld(db Database, doc, oldDoc Doc) error {
	id, err := validateDocID(doc.ID())
	if err != nil {
		return err
	}
	doctype := doc.DocType()
	if id == "" || doc.Rev() == "" || doctype == "" {
		return fmt.Errorf("UpdateDoc doc argument should have doctype, id and rev")
	}

	url := url.PathEscape(id)
	var res UpdateResponse
	err = makeRequest(db, doctype, http.MethodPut, url, doc, &res)
	if err != nil {
		return err
	}
	doc.SetRev(res.Rev)
	RTEvent(db, realtime.EventUpdate, doc, oldDoc)
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
	var res UpdateResponse
	err = makeRequest(db, doctype, http.MethodPut, url.PathEscape(id), doc, &res)
	if err != nil {
		return err
	}
	doc.SetRev(res.Rev)
	RTEvent(db, realtime.EventCreate, doc, nil)
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

// Upsert create the doc or update it if it already exists.
func Upsert(db Database, doc Doc) error {
	id, err := validateDocID(doc.ID())
	if err != nil {
		return err
	}

	var old JSONDoc
	err = GetDoc(db, doc.DocType(), id, &old)
	if IsNoDatabaseError(err) {
		err = CreateDB(db, doc.DocType())
		if err != nil {
			return err
		}
		return CreateNamedDoc(db, doc)
	}
	if IsNotFoundError(err) {
		return CreateNamedDoc(db, doc)
	}
	if err != nil {
		return err
	}

	doc.SetRev(old.Rev())
	return UpdateDoc(db, doc)
}

func createDocOrDb(db Database, doc Doc, response interface{}) error {
	doctype := doc.DocType()
	err := makeRequest(db, doctype, http.MethodPost, "", doc, response)
	if err == nil || !IsNoDatabaseError(err) {
		return err
	}
	err = CreateDB(db, doctype)
	if err == nil || IsFileExists(err) {
		err = makeRequest(db, doctype, http.MethodPost, "", doc, response)
	}
	return err
}

// CreateDoc is used to persist the given document in the couchdb
// database. The document's SetRev and SetID function will be called
// with the document's new ID and Rev.
// This function creates a database if this is the first document of its type
func CreateDoc(db Database, doc Doc) error {
	var res *UpdateResponse

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
	RTEvent(db, realtime.EventCreate, doc, nil)
	return nil
}

// DefineViews creates a design doc with some views
func DefineViews(db Database, views []*View) error {
	for _, v := range views {
		id := "_design/" + v.Name
		url := url.PathEscape(id)
		doc := &ViewDesignDoc{
			ID:    id,
			Lang:  "javascript",
			Views: map[string]*View{v.Name: v},
		}
		err := makeRequest(db, v.Doctype, http.MethodPut, url, &doc, nil)
		if IsNoDatabaseError(err) {
			err = CreateDB(db, v.Doctype)
			if err != nil {
				return err
			}
			err = makeRequest(db, v.Doctype, http.MethodPut, url, &doc, nil)
		}
		if IsConflictError(err) {
			var old ViewDesignDoc
			err = makeRequest(db, v.Doctype, http.MethodGet, url, nil, &old)
			if err != nil {
				return err
			}
			if !equalViews(&old, doc) {
				doc.Rev = old.Rev
				err = makeRequest(db, v.Doctype, http.MethodPut, url, &doc, nil)
			} else {
				err = nil
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func equalViews(v1 *ViewDesignDoc, v2 *ViewDesignDoc) bool {
	if v1.Lang != v2.Lang {
		return false
	}
	if len(v1.Views) != len(v2.Views) {
		return false
	}
	for name, view1 := range v1.Views {
		view2, ok := v2.Views[name]
		if !ok {
			return false
		}
		if view1.Map != view2.Map ||
			view1.Reduce != view2.Reduce {
			return false
		}
	}
	return true
}

// ExecView executes the specified view function
func ExecView(db Database, view *View, req *ViewRequest, results interface{}) error {
	viewurl := fmt.Sprintf("_design/%s/_view/%s", view.Name, view.Name)
	if req.GroupLevel > 0 {
		req.Group = true
	}
	v, err := req.Values()
	if err != nil {
		return err
	}
	viewurl += "?" + v.Encode()
	if req.Keys != nil {
		return makeRequest(db, view.Doctype, http.MethodPost, viewurl, req, &results)
	}
	return makeRequest(db, view.Doctype, http.MethodGet, viewurl, nil, &results)
}

// DefineIndex define the index on the doctype database
// see query package on how to define an index
func DefineIndex(db Database, index *mango.Index) error {
	_, err := DefineIndexRaw(db, index.Doctype, index.Request)
	return err
}

// DefineIndexRaw defines a index
func DefineIndexRaw(db Database, doctype string, index interface{}) (*IndexCreationResponse, error) {
	url := "_index"
	response := &IndexCreationResponse{}
	err := makeRequest(db, doctype, http.MethodPost, url, &index, &response)
	if IsNoDatabaseError(err) {
		if err = CreateDB(db, doctype); err != nil {
			return nil, err
		}
		err = makeRequest(db, doctype, http.MethodPost, url, &index, &response)
	}
	if err != nil {
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
	url := "_find"
	// prepare a structure to receive the results
	var response findResponse
	err := makeRequest(db, doctype, http.MethodPost, url, &req, &response)
	if err != nil {
		if isIndexError(err) {
			jsonReq, errm := json.Marshal(req)
			if errm != nil {
				return err
			}
			errc := err.(*Error)
			errc.Reason += fmt.Sprintf(" (original req: %s)", string(jsonReq))
			return errc
		}
		return err
	}
	if response.Warning != "" {
		// Developer should not rely on unoptimized index.
		return unoptimalError()
	}
	return json.Unmarshal(response.Docs, results)
}

// NormalDocs returns all the documents from a database, with pagination, but
// it excludes the design docs.
func NormalDocs(db Database, doctype string, skip, limit int) (*NormalDocsResponse, error) {
	var findRes struct {
		Docs []json.RawMessage `json:"docs"`
	}
	req := FindRequest{
		Selector: mango.Gte("_id", nil),
		Skip:     skip,
		Limit:    limit,
	}
	err := makeRequest(db, doctype, http.MethodPost, "_find", &req, &findRes)
	if err != nil {
		return nil, err
	}
	res := NormalDocsResponse{
		Rows: findRes.Docs,
	}
	if len(res.Rows) < limit {
		res.Total = skip + len(res.Rows)
	} else {
		var designRes ViewResponse
		err = makeRequest(db, doctype, http.MethodGet, "_design_docs", nil, &designRes)
		if err != nil {
			return nil, err
		}
		if designRes.Offset+designRes.Total < skip+len(res.Rows) {
			// https://github.com/apache/couchdb/issues/1603
			return nil, errors.New("Unexpected state")
		}
		res.Total = designRes.Total - len(designRes.Rows)
	}
	return &res, nil
}

func validateDocID(id string) (string, error) {
	if len(id) > 0 && id[0] == '_' {
		return "", newBadIDError(id)
	}
	return id, nil
}

// ViewDesignDoc is the structure if a _design doc containing views
type ViewDesignDoc struct {
	ID    string           `json:"_id,omitempty"`
	Rev   string           `json:"_rev,omitempty"`
	Lang  string           `json:"language"`
	Views map[string]*View `json:"views"`
}

// IndexCreationResponse is the response from couchdb when we create an Index
type IndexCreationResponse struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	Reason string `json:"reason,omitempty"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
}

// UpdateResponse is the response from couchdb when updating documents
type UpdateResponse struct {
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
	Selector mango.Filter `json:"selector"`
	UseIndex string       `json:"use_index,omitempty"`
	Limit    int          `json:"limit,omitempty"`
	Skip     int          `json:"skip,omitempty"`
	Sort     mango.SortBy `json:"sort,omitempty"`
	Fields   []string     `json:"fields,omitempty"`
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
	Group      bool `json:"group" url:"group"`
	GroupLevel int  `json:"group_level,omitempty" url:"group_level,omitempty"`
}

// ViewResponseRow is a row in a ViewResponse
type ViewResponseRow struct {
	ID    string          `json:"id"`
	Key   interface{}     `json:"key"`
	Value interface{}     `json:"value"`
	Doc   json.RawMessage `json:"doc"`
}

// ViewResponse is the response we receive when executing a view
type ViewResponse struct {
	Total  int                `json:"total_rows"`
	Offset int                `json:"offset,omitempty"`
	Rows   []*ViewResponseRow `json:"rows"`
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

// NormalDocsResponse is the response the stack send for _normal_docs queries
type NormalDocsResponse struct {
	Total int               `json:"total_rows"`
	Rows  []json.RawMessage `json:"rows"`
}
