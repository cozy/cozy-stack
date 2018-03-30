package sharing

// Rule describes how the sharing behave when a document matching the rule is
// added, updated or deleted.
type Rule struct {
	Title    string   `json:"title"`
	DocType  string   `json:"doctype"`
	Selector string   `json:"selector,omitempty"`
	Values   []string `json:"values,omitempty"`
	Local    bool     `json:"local,omitempty"`
	Add      string   `json:"add"`
	Update   string   `json:"update"`
	Remove   string   `json:"remove"`
}

// Accept returns true if the document matches the rule criteria
// TODO detect if it's a deletion
// TODO improve to detect if the rule is applied on the owner or the recipient
func (r Rule) Accept(doctype string, doc map[string]interface{}) bool {
	if r.Local || doctype != r.DocType {
		return false
	}
	var ok bool
	var val string
	if r.Selector == "" || r.Selector == "id" {
		val, ok = doc["_id"].(string)
	} else {
		// TODO pick nested value if the selector contains dots
		val, ok = doc[r.Selector].(string)
	}
	if !ok {
		return false
	}
	for _, v := range r.Values {
		if v == val {
			return true
		}
	}
	return false
}
