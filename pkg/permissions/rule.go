package permissions

import (
	"errors"
	"fmt"
	"strings"
)

const ruleSep = " "

const valueSep = ","
const partSep = ":"

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
			return out, errors.New("the type is mandatory for a permissions rule")
		}
		out.Type = parts[0]
	default:
		return out, fmt.Errorf("Too many parts in %s", in)
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

// ValuesValid returns true if any value statisfy the predicate
func (r Rule) ValuesValid(o Validable) bool {
	for _, v := range r.Values {
		if o.Valid(r.Selector, v) {
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
