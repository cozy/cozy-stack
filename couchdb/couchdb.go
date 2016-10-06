package couchdb

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	uuid "github.com/satori/go.uuid"
)

// Doc : A couchdb Doc is just a json object
type Doc map[string]interface{}

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

func makeDocID(doctype string, id string) string {
	return url.QueryEscape(doctype + "/" + id)
}

func docURL(dbprefix, doctype, id string) string {
	return makeDBName(dbprefix, doctype) + "/" + makeDocID(doctype, id)
}

func makeUUID() string {
	u := uuid.NewV4()
	return hex.Dump(u[:])
}

func makeRequest(method, path string, reqbody interface{}, resbody interface{}) error {
	var reqjson []byte
	var err error

	fmt.Printf("[couchdb request] %v %v \n", method, path)

	if reqbody != nil {
		reqjson, err = json.Marshal(reqbody)
		if err != nil {
			return err
		}
	}
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Couchdb as returned an error HTTP status code
		return newCouchdbError(resp.StatusCode, body)
	}

	fmt.Printf("BODY = %v %v\n", string(body), reflect.TypeOf(resbody))

	return json.Unmarshal(body, &resbody)

}

// GetDoc fetch a document by its docType and ID, out is filled with
// the document by json.Unmarshal-ing
func GetDoc(dbprefix, doctype, id string, out Doc) error {
	return makeRequest("GET", docURL(dbprefix, doctype, id), nil, &out)
}

// CreateDB creates the necessary database for a doctype
func CreateDB(dbprefix, doctype string) error {
	var out Doc
	return makeRequest("PUT", makeDBName(dbprefix, doctype), nil, &out)
}

func attemptCreateDBAndDoc(dbprefix, doctype string, doc Doc) error {
	createErr := CreateDB(dbprefix, doctype)
	if createErr != nil {
		return createErr
	}
	return CreateDoc(dbprefix, doctype, doc)
}

// CreateDoc creates a document
// created is populated with keys from
func CreateDoc(dbprefix, doctype string, doc Doc) error {
	var response map[string]interface{}

	doc["_id"] = doctype + "/" + makeUUID()

	err := makeRequest("POST", makeDBName(dbprefix, doctype), &doc, &response)
	if isNoDatabaseError(err) {
		return attemptCreateDBAndDoc(dbprefix, doctype, doc)
	} else if err != nil {
		return err
	} else if !response["ok"].(bool) {
		return fmt.Errorf("couchdb replied with 200 ok=false")
	}
	// assign extracted values to the given doc
	// doubt : should we instead try to be more immutable and make a new map ?
	doc["_id"] = response["id"]
	doc["_rev"] = response["rev"]
	return nil
}
