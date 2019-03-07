package permissions

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
