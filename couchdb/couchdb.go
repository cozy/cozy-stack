package couchdb

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

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

func makeDBURL(dbprefix, doctype string) string {
	return CouchURL() + makeDBName(dbprefix, doctype)
}

func makeDocID(doctype string, id string) string {
	return url.QueryEscape(doctype + "/" + id)
}

func docURL(dbprefix, doctype, id string) string {
	return makeDBURL(dbprefix, doctype) + "/" + makeDocID(doctype, id)
}

// GetDoc fetch a document by its docType and ID, out is filled with
// the document by json.Unmarshal-ing
func GetDoc(dbprefix, doctype, id string, out interface{}) error {
	url := docURL(dbprefix, doctype, id)
	fmt.Printf("[couchdb request] %v\n", url)
	req, err := http.NewRequest("GET", url, nil)
	// Possible err = wrong method, wrong url --> 500
	if err != nil {
		return &Error{http.StatusInternalServerError,
			[]byte("{\"error\":\"Wrong configuration for couchdbserver\"}")}
	}
	req.Header.Add("Accept", "application/json")
	resp, err := couchdbClient.Do(req)
	if err != nil {
		return &Error{http.StatusServiceUnavailable,
			[]byte("{\"error\":\"No couch to seat on.\"}")}
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return &Error{resp.StatusCode,
			[]byte("{\"error\":\"Couchdb hangup.\"}")}
	}

	if resp.StatusCode != 200 {
		return &Error{resp.StatusCode, body}
	}

	return json.Unmarshal(body, &out)

}
