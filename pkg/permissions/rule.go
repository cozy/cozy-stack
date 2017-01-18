package permissions

import (
	"errors"
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
	if len(r.Verbs) != 0 {
		out += partSep + r.Verbs.String()
	}
	if len(r.Selector) != 0 {
		out += partSep + r.Selector
	}
	if len(r.Values) != 0 {
		out += partSep + strings.Join(r.Values, valueSep)
	}
	return out, nil
}

// UnmarshalRuleString parse a scope formated rule
func UnmarshalRuleString(in string) (Rule, error) {
	var out Rule
	parts := strings.Split(in, partSep)

	switch len(parts) {
	case 0:
		return out, errors.New("empty rule string")

	case 1:
		out.Type = parts[0]

	case 2:
		out.Type = parts[0]
		out.Verbs = VerbSplit(parts[1])

	case 3:
		out.Type = parts[0]
		out.Verbs = VerbSplit(parts[1])
		out.Values = strings.Split(parts[2], valueSep)

	case 4:
		out.Type = parts[0]
		out.Verbs = VerbSplit(parts[1])
		out.Selector = parts[2]
		out.Values = strings.Split(parts[3], valueSep)

	}

	return out, nil
}
