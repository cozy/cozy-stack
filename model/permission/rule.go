package permission

import (
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
)

const ruleSep = " "

const valueSep = ","
const partSep = ":"

// RefSep is used to separate doctype and value for a referenced selector
const RefSep = "/"

// Rule represent a single permissions rule, ie a Verb and a type
type Rule struct {
	// Type is the JSON-API type or couchdb Doctype
	Type string `json:"type"`

	// Title is a human readable (i18n key) header for this rule
	Title string `json:"-"`

	// Description is a human readable (i18n key) purpose of this rule
	Description string `json:"description,omitempty"`

	// Verbs is a subset of http methods.
	Verbs VerbSet `json:"verbs,omitempty"`

	// Selector is the field which must be one of Values.
	Selector string   `json:"selector,omitempty"`
	Values   []string `json:"values,omitempty"`
}

// MarshalScopeString transform a Rule into a string of the shape
// io.cozy.files:GET:io.cozy.files.music-dir
func (r Rule) MarshalScopeString() (string, error) {
	out := r.Type
	hasVerbs := len(r.Verbs) != 0
	hasValues := len(r.Values) != 0
	hasSelector := r.Selector != ""

	if hasVerbs || hasValues || hasSelector {
		out += partSep + r.Verbs.String()
	}

	if hasValues {
		out += partSep + strings.Join(r.Values, valueSep)
	}

	if hasSelector {
		out += partSep + r.Selector
	}

	return out, nil
}

// UnmarshalRuleString parse a scope formated rule
func UnmarshalRuleString(in string) (Rule, error) {
	var out Rule
	parts := strings.Split(in, partSep)
	switch len(parts) {
	case 4:
		out.Selector = parts[3]
		fallthrough
	case 3:
		out.Values = strings.Split(parts[2], valueSep)
		fallthrough
	case 2:
		out.Verbs = VerbSplit(parts[1])
		fallthrough
	case 1:
		if parts[0] == "" {
			return out, ErrBadScope
		}
		out.Type = parts[0]
	default:
		return out, ErrBadScope
	}
	return out, nil
}

// SomeValue returns true if any value statisfy the predicate
func (r Rule) SomeValue(predicate func(v string) bool) bool {
	for _, v := range r.Values {
		if predicate(v) {
			return true
		}
	}
	return false
}

// ValuesMatch returns true if any value statisfy the predicate
func (r Rule) ValuesMatch(o Matcher) bool {
	for _, v := range r.Values {
		if o.Match(r.Selector, v) {
			return true
		}
	}
	return false
}

// ValuesContain returns true if all the values are in r.Values
func (r Rule) ValuesContain(values ...string) bool {
	for _, value := range values {
		valueOK := false
		for _, v := range r.Values {
			if v == value {
				valueOK = true
			}
		}
		if !valueOK {
			return false
		}
	}
	return true
}

// TranslationKey returns a string that can be used as a key for translating a
// description of this rule
func (r Rule) TranslationKey() string {
	switch r.Type {
	case consts.Settings:
		if r.Verbs.ReadOnly() && len(r.Values) == 1 && r.Values[0] == consts.DiskUsageID {
			return "Permissions disk usage"
		}
	case consts.Jobs, consts.Triggers:
		if len(r.Values) == 1 && r.Selector == "worker" {
			return "Permissions worker " + r.Values[0]
		}
		// TODO a specific folder (io.cozy.files)
	}
	return "Permissions " + r.Type
}

// Merge merges the rule2 in rule1
// Rule1 name & description are kept
func (r Rule) Merge(r2 Rule) (*Rule, error) {
	if r.Type != r2.Type {
		return nil, fmt.Errorf("Cannot merge these rules, type is different")
	}

	newRule := &r

	// Verbs
	for verb, content := range r2.Verbs {
		if !newRule.Verbs.Contains(verb) {
			newRule.Verbs[verb] = content
		}
	}

	for _, value := range r2.Values {
		if !newRule.ValuesContain(value) {
			newRule.Values = append(newRule.Values, value)
		}
	}

	return newRule, nil
}
