package permissions

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

// Set is a Set of rule
type Set []Rule

// MarshalJSON implements json.Marshaller on Set
// see docs/permission for structure
func (ps Set) MarshalJSON() ([]byte, error) {

	m := make(map[string]*json.RawMessage)

	for i, r := range ps {
		b, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		rm := json.RawMessage(b)
		key := r.Title
		if key == "" {
			key = "rule" + strconv.Itoa(i)
		}
		m[key] = &rm
	}

	return json.Marshal(m)
}

// MarshalScopeString transforms a Set into a string for Oauth Scope
// (a space separated concatenation of its rules)
func (ps Set) MarshalScopeString() (string, error) {
	out := ""
	if len(ps) == 0 {
		return "", nil
	}
	for _, r := range ps {
		s, err := r.MarshalScopeString()
		if err != nil {
			return "", err
		}
		out += " " + s
	}
	return out[1:], nil
}

// UnmarshalJSON parses a json formated permission set
func (ps *Set) UnmarshalJSON(j []byte) error {
	*ps = make(Set, 0)
	var m map[string]*json.RawMessage
	err := json.Unmarshal(j, &m)
	if err != nil {
		return err
	}
	for title, rulejson := range m {
		var r Rule
		err := json.Unmarshal(*rulejson, &r)
		if err != nil {
			return err
		}
		r.Title = title
		*ps = append(*ps, r)
	}
	return nil
}

// UnmarshalScopeString parse a Scope string into a permission Set
func UnmarshalScopeString(in string) (Set, error) {
	if in == "" {
		return nil, ErrBadScope
	}

	parts := strings.Split(in, ruleSep)
	out := make(Set, len(parts))

	for i, p := range parts {
		s, err := UnmarshalRuleString(p)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}

	return out, nil
}

// Some returns true if the predicate return true for any of the rule.
func (ps Set) Some(predicate func(Rule) bool) bool {
	for _, r := range ps {
		if predicate(r) {
			return true
		}
	}
	return false
}

// RuleInSubset returns true if any document allowed by the rule
// is allowed by the set.
func (ps *Set) RuleInSubset(r2 Rule) bool {
	for _, r := range *ps {
		if r.Type != r2.Type {
			continue
		}

		if !r.Verbs.ContainsAll(r2.Verbs) {
			continue
		}

		if r.Selector == "" && len(r.Values) == 0 {
			return true
		}

		if r.Selector != r2.Selector {
			continue
		}

		if r.ValuesContain(r2.Values...) {
			return true
		}
	}

	return false
}

// IsSubSetOf returns true if any document allowed by the set
// would have been allowed by parent.
func (ps *Set) IsSubSetOf(parent Set) bool {
	for _, r := range *ps {
		if !parent.RuleInSubset(r) {
			return false
		}
	}

	return true
}

// HasSameRules returns true if the two sets have exactly the same rules.
func (ps Set) HasSameRules(other Set) bool {
	if len(ps) != len(other) {
		return false
	}

	for _, rule := range ps {
		match := false
		for _, otherRule := range other {
			if reflect.DeepEqual(rule.Values, otherRule.Values) &&
				rule.Selector == otherRule.Selector &&
				rule.Verbs.ContainsAll(otherRule.Verbs) &&
				otherRule.Verbs.ContainsAll(rule.Verbs) &&
				reflect.DeepEqual(otherRule.Type, rule.Type) {
				match = true
				break
			}
		}

		if !match {
			return false
		}
	}

	return true
}
