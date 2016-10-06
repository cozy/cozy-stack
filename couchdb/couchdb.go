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

// Doc : A couchdb Doc is just a json object
type Doc map[string]interface{}

// GetDoctypeAndID returns the doctype and unqualified id of a document
func (d Doc) GetDoctypeAndID() (string, string) {
	parts := strings.Split(d["_id"].(string), "/")
	return parts[0], parts[1]
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

func makeDocID(doctype string, id string) string {
	return url.QueryEscape(doctype + "/" + id)
}

func docURL(dbprefix, doctype, id string) string {
	return makeDBName(dbprefix, doctype) + "/" + makeDocID(doctype, id)
}

func makeUUID() string {
	u := uuid.NewV4()
	return hex.EncodeToString(u[:])
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
func GetDoc(dbprefix, doctype, id string, out *Doc) error {
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

func attemptCreateDBAndDoc(dbprefix, doctype string, doc Doc) error {
	createErr := CreateDB(dbprefix, doctype)
	if createErr != nil {
		return createErr
	}
	return CreateDoc(dbprefix, doctype, doc)
}

// CreateDoc creates a document in couchdb. It modifies doc in place to add
// _id and _rev.
func CreateDoc(dbprefix, doctype string, doc Doc) error {
	var response updateResponse

	doc["_id"] = doctype + "/" + makeUUID()

	err := makeRequest("POST", makeDBName(dbprefix, doctype), &doc, &response)
	if isNoDatabaseError(err) {
		return attemptCreateDBAndDoc(dbprefix, doctype, doc)
	}
	if err != nil {
		return err
	}
	if !response.Ok {
		return fmt.Errorf("couchdb replied with 200 ok=false")
	}
	// assign extracted values to the given doc
	// doubt : should we instead try to be more immutable and make a new map ?
	doc["_id"] = response.ID
	doc["_rev"] = response.Rev
	return nil
}
