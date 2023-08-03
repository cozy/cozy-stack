package mango

// An IndexDef is a list of fields to be indexed and an optional partial filter
// for complex and costly requests.
type IndexDef struct {
	Fields        []string `json:"fields"`
	PartialFilter Filter   `json:"partial_filter_selector,omitempty"`
}

// IndexRequest is a request to be POSTED to create the index
type IndexRequest struct {
	Name  string   `json:"name,omitempty"`
	DDoc  string   `json:"ddoc,omitempty"`
	Index IndexDef `json:"index"`
}

// Index contains an index request on a specified domain.
type Index struct {
	Doctype string
	Request *IndexRequest
}

// MakeIndex constructs a new Index
// it lets couchdb defaults for index & designdoc names.
func MakeIndex(doctype, name string, def IndexDef) *Index {
	return &Index{
		Doctype: doctype,
		Request: &IndexRequest{
			DDoc:  name,
			Index: def,
		},
	}
}
