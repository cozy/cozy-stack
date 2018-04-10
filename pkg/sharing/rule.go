package sharing

import (
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

const (
	// ActionRuleNone is used when an add/update/remove should not be
	// replicated to the other cozys
	ActionRuleNone = "none"
	// ActionRulePush is used when an add/update/remove should be replicated
	// only if it happened on the owner's cozy
	ActionRulePush = "push"
	// ActionRuleSync is used when an add/update/remove should be always replicated
	ActionRuleSync = "sync"
	// ActionRuleRevoke is used when a remove should revoke the sharing
	ActionRuleRevoke = "revoke"
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

// FilesByID returns true if the rule is for the files by doctype and the
// selector is an id (not a referenced_by). With such a rule, the identifiers
// must be xored before being sent to another cozy instance.
func (r Rule) FilesByID() bool {
	if r.DocType != consts.Files {
		return false
	}
	return r.Selector == "" || r.Selector == "id" || r.Selector == "_id"
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
		if rule.DocType == consts.Files {
			// TODO forbid to share folders in the trash
			for _, val := range rule.Values {
				if val == consts.RootDirID ||
					val == consts.TrashDirID ||
					val == consts.SharedWithMeDirID {
					return ErrInvalidRule
				}
			}
		} else if permissions.CheckWritable(rule.DocType) != nil {
			return ErrInvalidRule
		}
		if rule.Add == "" {
			rule.Add = ActionRuleNone
		}
		rule.Add = strings.ToLower(rule.Add)
		if rule.Add != ActionRuleNone &&
			rule.Add != ActionRulePush &&
			rule.Add != ActionRuleSync {
			return ErrInvalidRule
		}
		if rule.Update == "" {
			rule.Update = ActionRuleNone
		}
		rule.Update = strings.ToLower(rule.Update)
		if rule.Update != ActionRuleNone &&
			rule.Update != ActionRulePush &&
			rule.Update != ActionRuleSync {
			return ErrInvalidRule
		}
		if rule.Remove == "" {
			rule.Remove = ActionRuleNone
		}
		rule.Remove = strings.ToLower(rule.Remove)
		if rule.Remove != ActionRuleNone &&
			rule.Remove != ActionRulePush &&
			rule.Remove != ActionRuleSync &&
			rule.Remove != ActionRuleRevoke {
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

// TriggerArgs returns the string that can be used as an argument to create a
// trigger for this rule. The result can be an empty string if the rule doesn't
// need a trigger (a local or one-shot rule).
func (r Rule) TriggerArgs(owner bool) string {
	if r.Local {
		return ""
	}
	verbs := make([]string, 0, 3)
	if r.Add == ActionRuleSync || (owner && r.Add == ActionRulePush) {
		verbs = append(verbs, "CREATED")
	}
	if r.Update == ActionRuleSync || (owner && r.Update == ActionRulePush) {
		verbs = append(verbs, "UPDATED")
	}
	if r.Remove == ActionRuleSync || (owner && r.Remove == ActionRulePush) {
		verbs = append(verbs, "UPDATED")
	}
	if len(verbs) == 0 {
		return ""
	}
	args := r.DocType + ":" + strings.Join(verbs, ",")
	if len(r.Values) > 0 {
		args += ":" + strings.Join(r.Values, ",")
		if r.Selector != "" && r.Selector != "id" {
			args += ":" + r.Selector
		}
	}
	return args
}

// TwoWays returns true if at least one rule has an ActionRuleSync defined
func (s *Sharing) TwoWays() bool {
	for _, r := range s.Rules {
		if r.Add == ActionRuleSync || r.Update == ActionRuleSync ||
			r.Remove == ActionRuleSync {
			return true
		}
	}
	return false
}
