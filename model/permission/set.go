package permission

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// Set is a Set of rule
type Set []Rule

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

// MarshalJSON implements json.Marshaller on Set. Note that the JSON
// representation is a key-value object, but the golang Set is an ordered
// slice. In theory, JSON objects have no order on their keys, but here, we try
// to keep the same order on decoding/encoding.
// See docs/permissions.md for more details on the structure.
func (ps Set) MarshalJSON() ([]byte, error) {
	if len(ps) == 0 {
		return []byte("{}"), nil
	}
	buf := make([]byte, 0, 4096)

	for i, r := range ps {
		title := r.Title
		if title == "" {
			title = "rule" + strconv.Itoa(i)
		}
		key, err := json.Marshal(title)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		buf = append(buf, ',')
		buf = append(buf, key...)
		buf = append(buf, ':')
		buf = append(buf, val...)
	}

	buf[0] = '{'
	buf = append(buf, '}')
	return buf, nil
}

// UnmarshalJSON parses a json formated permission set
func (ps *Set) UnmarshalJSON(j []byte) error {
	var raws map[string]json.RawMessage
	err := json.Unmarshal(j, &raws)
	if err != nil {
		return err
	}
	titles, err := extractJSONKeys(j)
	if err != nil {
		return err
	}

	*ps = make(Set, 0)
	for _, title := range titles {
		raw := raws[title]
		var r Rule
		err := json.Unmarshal(raw, &r)
		if err != nil {
			return err
		}
		r.Title = title
		*ps = append(*ps, r)
	}
	return nil
}

func extractJSONKeys(j []byte) ([]string, error) {
	var keys []string
	dec := json.NewDecoder(bytes.NewReader(j))
	depth := 0
	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if t == json.Delim('{') {
			depth++
		} else if t == json.Delim('}') {
			depth--
		} else if depth == 1 {
			if k, ok := t.(string); ok {
				keys = append(keys, k)
			}
		}
	}
	return keys, nil
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

// Diff returns a the differences between two sets.
// Useful to see what rules had been added between a original manifest
// permissions and now.
//
// TODO: We are ignoring removed values/verbs between rule 1 and rule 2.
// - At the moment, it onlys show the added values, verbs and rules
func Diff(set1, set2 Set) (Set, error) {
	// If sets are the same, do not compute
	if set1.HasSameRules(set2) {
		return set1, nil
	}

	newSet := Set{}

	// Appending not existing rules
	for _, r2 := range set2 {
		found := false

		for _, r := range set1 {
			if r.Title == r2.Title {
				// Rule exist
				found = true
				break
			}
		}

		if !found {
			newSet = append(newSet, r2)
		}
	}

	// Compare each key
	for _, rule1 := range set1 {
		for _, rule2 := range set2 {
			if rule1.Title == rule2.Title { // Same rule, we are going to compute differences
				newRule := Rule{
					Title:  rule1.Title,
					Type:   rule1.Type,
					Verbs:  map[Verb]struct{}{},
					Values: []string{},
				}

				// Handle verbs. Here we are going to find verbs in set2 that
				// are not present in set1, meaning they were added later by an
				// external human action
				for verb2, content2 := range rule2.Verbs {
					if !rule1.Verbs.Contains(verb2) {
						newRule.Verbs[verb2] = content2
					}
				}

				// Handle values
				for _, value2 := range rule2.Values {
					if ok := rule1.ValuesContain(value2); !ok {
						newRule.Values = append(newRule.Values, value2)
					}
				}

				newSet = append(newSet, newRule)
			}
		}
	}
	return newSet, nil
}
