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
	GetID() string
	SetID(id string)
}

// JSONDoc is a map representing a simple json object that implements
// the Doc interface.
type JSONDoc map[string]interface{}

func (j JSONDoc) GetID() string {
	return j["_id"].(string)
}

func (j JSONDoc) SetID(id string) {
	j["_id"] = id
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
	return makeDBName(dbprefix, doctype) + "/" + url.QueryEscape(doctype+"/"+id)
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
func ResetDB(dbprefix, doctype string) error {
	err := DeleteDB(dbprefix, doctype)
	if err != nil {
		return err
	}
	return CreateDB(dbprefix, doctype)
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
// database. It returns the revision of the added document and sets the
// document id.
func CreateDoc(dbprefix, doctype string, doc Doc) (rev string, err error) {
	var res *updateResponse

	if doc.GetID() != "" {
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

	rev = res.Rev
	return
}

func GetDoctypeAndID(doc Doc) (string, string) {
	parts := strings.Split(doc.GetID(), "/")
	return parts[0], parts[1]
}
