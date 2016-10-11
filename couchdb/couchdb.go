package couchdb

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	uuid "github.com/satori/go.uuid"
)

type updateResponse struct {
	ID  string `json:"id"`
	Rev string `json:"rev"`
	Ok  bool   `json:"ok"`
}

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
type JSONDoc map[string]interface{}

// ID returns the qualified identifier field of the document
//   "io.cozy.event/123abc123" == doc.ID()
func (j JSONDoc) ID() string {
	qid, ok := j["_id"].(string)
	if ok {
		return qid
	}
	return ""
}

// Rev returns the revision field of the document
//   "3-1234def1234" == doc.Rev()
func (j JSONDoc) Rev() string {
	rev, ok := j["_rev"].(string)
	if ok {
		return rev
	}
	return ""
}

// DocType returns the document type of the document
//   "io.cozy.event" == doc.Doctype()
func (j JSONDoc) DocType() string {
	qid, ok := j["_id"].(string)
	if !ok {
		return ""
	}
	return qid[0:strings.Index(qid, "/")]
}

// SetID is used to set the qualified identifier of the document
func (j JSONDoc) SetID(qid string) {
	j["_id"] = qid
}

// SetRev is used to set the revision of the document
func (j JSONDoc) SetRev(rev string) {
	j["_rev"] = rev
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

func genDocID(doctype string) string {
	u := uuid.NewV4()
	return doctype + "/" + hex.EncodeToString(u[:])
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

// GetDoc fetch a document by its docType and ID, out is filled with
// the document by json.Unmarshal-ing
func GetDoc(dbprefix, doctype, id string, out Doc) error {
	err := makeRequest("GET", docURL(dbprefix, doctype, id), nil, out)
	if isNoDatabaseError(err) {
		err.(*Error).Reason = "wrong_doctype"
	}
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
	if err != nil {
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
		return fmt.Errorf("UpdateDoc argument should have doctype, id and rev ")
	}

	url := docURL(dbprefix, doctype, id)
	var res updateResponse
	err = makeRequest("PUT", url, doc, &res)
	if err == nil {
		doc.SetRev(res.Rev)
	}
	return err
}

func createDocOrDb(dbprefix, doctype string, doc Doc, response interface{}) (err error) {
	db := makeDBName(dbprefix, doctype)
	err = makeRequest("POST", db, doc, response)
	if err == nil || !isNoDatabaseError(err) {
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
// with the document's new QID and Rev.
// This function creates a database if this is the first document of its type
//
// @TODO: we still pass the doctype around to handle the /data api
func CreateDoc(dbprefix, doctype string, doc Doc) (err error) {
	var res *updateResponse

	if doc.ID() != "" {
		err = fmt.Errorf("Can not create document with a defined ID")
		return
	}

	doc.SetID(genDocID(doctype))
	err = createDocOrDb(dbprefix, doctype, doc, &res)
	if err != nil {
		return
	}

	if !res.Ok {
		err = fmt.Errorf("CouchDB replied with 200 ok=false")
		return
	}

	doc.SetRev(res.Rev)
	return
}
