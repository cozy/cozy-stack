package sharing

import (
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

// Rule describes how the sharing behave when a document matching the rule is
// added, updated or deleted.
type Rule struct {
	Title    string   `json:"title"`
	DocType  string   `json:"doctype"`
	Selector string   `json:"selector,omitempty"`
	Values   []string `json:"values"`
	Local    bool     `json:"local,omitempty"`
	Add      string   `json:"add"`
	Update   string   `json:"update"`
	Remove   string   `json:"remove"`
}

// ValidateRules returns an error if the rules are invalid (the doctype is
// missing for example)
func (s *Sharing) ValidateRules() error {
	if len(s.Rules) == 0 {
		return ErrNoRules
	}
	for _, rule := range s.Rules {
		if rule.Title == "" || rule.DocType == "" || len(rule.Values) == 0 {
			return ErrInvalidRule
		}
		if rule.DocType != consts.Files && permissions.CheckWritable(rule.DocType) != nil {
			return ErrInvalidRule
		}
		if rule.Add == "" {
			rule.Add = "none"
		}
		rule.Add = strings.ToLower(rule.Add)
		if rule.Add != "none" && rule.Add != "push" && rule.Add != "sync" {
			return ErrInvalidRule
		}
		if rule.Update == "" {
			rule.Update = "none"
		}
		rule.Update = strings.ToLower(rule.Update)
		if rule.Update != "none" && rule.Update != "push" && rule.Update != "sync" {
			return ErrInvalidRule
		}
		if rule.Remove == "" {
			rule.Remove = "none"
		}
		rule.Remove = strings.ToLower(rule.Remove)
		if rule.Remove != "none" && rule.Remove != "push" && rule.Remove != "sync" && rule.Remove != "revoke" {
			return ErrInvalidRule
		}
	}
	return nil
}

// Accept returns true if the document matches the rule criteria
// TODO use add, update and remove properties of the rule
func (r Rule) Accept(doctype string, doc map[string]interface{}) bool {
	if r.Local || doctype != r.DocType {
		return false
	}
	var obj interface{} = doc
	if r.Selector == "" || r.Selector == "id" {
		obj = doc["_id"]
	} else if doctype == consts.Files && r.Selector == couchdb.SelectorReferencedBy {
		if o, k := doc["referenced_by"].([]map[string]interface{}); k {
			refs := make([]string, len(o))
			for i, ref := range o {
				refs[i] = ref["type"].(string) + "/" + ref["id"].(string)
			}
			obj = refs
		}
	} else {
		keys := strings.Split(r.Selector, ".")
		for _, key := range keys {
			if o, k := obj.(map[string]interface{}); k {
				obj = o[key]
			} else {
				obj = nil
				break
			}
		}
	}
	if val, ok := obj.(string); ok {
		for _, v := range r.Values {
			if v == val {
				return true
			}
		}
	}
	if val, ok := obj.([]string); ok {
		for _, vv := range val {
			for _, v := range r.Values {
				if v == vv {
					return true
				}
			}
		}
	}
	return false
}
