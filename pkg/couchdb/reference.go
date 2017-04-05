package couchdb

// DocReference is a reference to a document
type DocReference struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}
