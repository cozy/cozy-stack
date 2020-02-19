package permission

import "strings"

// Fetcher is an interface for an object to see if it matches a rule.
type Fetcher interface {
	ID() string
	DocType() string
	Fetch(field string) []string
}

func matchValues(r Rule, o Fetcher) bool {
	// empty r.Values = any value
	if len(r.Values) == 0 {
		return true
	}
	if r.Selector == "" {
		return r.ValuesContain(o.ID())
	}
	return r.ValuesMatch(o)
}

func matchOnFields(r Rule, o Fetcher, fields ...string) bool {
	// in this case, if r.Values is empty the selector is considered too wide and
	// is forbidden
	if len(r.Values) == 0 || r.Selector == "" {
		return false
	}
	var matchSelector bool
	for _, f := range fields {
		if r.Selector == f {
			matchSelector = true
			break
		}
	}
	if !matchSelector {
		return false
	}
	return r.ValuesMatch(o)
}

func matchVerb(r Rule, v Verb) bool {
	return r.Verbs.Contains(v)
}

func matchType(r Rule, doctype string) bool {
	if r.Type == doctype {
		return true
	}
	if !isWildcard(r.Type) {
		return false
	}
	typ := trimWildcard(r.Type)
	return typ == doctype || strings.HasPrefix(doctype, typ+".")
}

func matchWholeType(r Rule) bool {
	return len(r.Values) == 0
}

func matchID(r Rule, id string) bool {
	return r.Selector == "" && r.ValuesContain(id)
}

// AllowWholeType returns true if the set allows to apply verb to every
// document from the given doctypes (ie. r.values == 0)
func (s Set) AllowWholeType(v Verb, doctype string) bool {
	return s.Some(func(r Rule) bool {
		return matchVerb(r, v) &&
			matchType(r, doctype) &&
			matchWholeType(r)
	})
}

// AllowID returns true if the set allows to apply verb to given type & id
func (s Set) AllowID(v Verb, doctype, id string) bool {
	return s.Some(func(r Rule) bool {
		return matchVerb(r, v) &&
			matchType(r, doctype) &&
			(matchWholeType(r) || matchID(r, id))
	})
}

// Allow returns true if the set allows to apply verb to given doc
func (s Set) Allow(v Verb, o Fetcher) bool {
	return s.Some(func(r Rule) bool {
		return matchVerb(r, v) &&
			matchType(r, o.DocType()) &&
			matchValues(r, o)
	})
}

// AllowOnFields returns true if the set allows to apply verb to given doc on
// the specified fields.
func (s Set) AllowOnFields(v Verb, o Fetcher, fields ...string) bool {
	return s.Some(func(r Rule) bool {
		return matchVerb(r, v) &&
			matchType(r, o.DocType()) &&
			matchOnFields(r, o, fields...)
	})
}
