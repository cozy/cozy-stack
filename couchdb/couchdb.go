package couchdb

import (
  "net/url"
  "io/ioutil"
  "fmt"
  "strings"
  "net/http"
  "encoding/json"
)
// CouchDBURL is the URL where to check if CouchDB is up
func CouchURL() string {
  return "http://localhost:5984/"
}
var CouchdbClient = &http.Client{}

func makeDBName(dbprefix, doctype string) string{
  // @TODO This should be better analysed
  dbname := dbprefix + doctype
  dbname = strings.Replace(dbname, ".", "-", -1)
  dbname = strings.ToLower(dbname)
	return url.QueryEscape(dbname)
}

func makeDBURL(dbprefix, doctype string) string{
  return CouchURL() + makeDBName(dbprefix, doctype)
}

func makeDocID(doctype string, id string) string{
  return url.QueryEscape(doctype + "/" + id)
}

func DocURL(dbprefix, doctype, id string) string{
  return makeDBURL(dbprefix, doctype) + "/" + makeDocID(doctype, id)
}

func GetDoc(dbprefix, doctype, id string, out interface{}) error {
  url := DocURL(dbprefix, doctype, id)
  fmt.Printf("[couchdb request] %v\n", url)
	req, err := http.NewRequest("GET", url, nil)
  req.Header.Add("Accept", "application/json")
	// Possible err = wrong method, wrong url --> 500
	if err != nil {
		return &CouchdbError{http.StatusInternalServerError,
      []byte("{\"error\":\"Wrong configuration for couchdbserver\"}")};
	}
	resp, err := CouchdbClient.Do(req)
	if err != nil {
		return &CouchdbError{http.StatusServiceUnavailable,
      []byte("{\"error\":\"No couch to seat on.\"}")}
	}
  body, err := ioutil.ReadAll(resp.Body)
  if err != nil {
		return &CouchdbError{resp.StatusCode,
      []byte("{\"error\":\"Couchdb hangup.\"}")}
	}

  if resp.StatusCode != 200 {
    return &CouchdbError{resp.StatusCode, body}
  }

  return json.Unmarshal(body, &out)

}
