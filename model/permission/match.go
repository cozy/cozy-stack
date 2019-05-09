package permission

// Matcher is an interface for a object than can be matched by a Set
type Matcher interface {
	ID() string
	DocType() string
	Match(field, expected string) bool
}

func matchValues(r Rule, o Matcher) bool {
	// empty r.Values = any value
	if len(r.Values) == 0 {
		return true
	}
	if r.Selector == "" {
		return r.ValuesContain(o.ID())
	}
	return r.ValuesMatch(o)
}

func matchOnFields(r Rule, o Matcher, fields ...string) bool {
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

func matchVerbAndType(r Rule, v Verb, doctype string) bool {
	return r.Verbs.Contains(v) && r.Type == doctype
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
		return matchVerbAndType(r, v, doctype) && matchWholeType(r)
	})
}

// AllowID returns true if the set allows to apply verb to given type & id
func (s Set) AllowID(v Verb, doctype, id string) bool {
	return s.Some(func(r Rule) bool {
		return matchVerbAndType(r, v, doctype) && (matchWholeType(r) || matchID(r, id))
	})
}

// Allow returns true if the set allows to apply verb to given doc
func (s Set) Allow(v Verb, o Matcher) bool {
	return s.Some(func(r Rule) bool {
		return matchVerbAndType(r, v, o.DocType()) && matchValues(r, o)
	})
}

// AllowOnFields returns true if the set allows to apply verb to given doc on
// the specified fields.
func (s Set) AllowOnFields(v Verb, o Matcher, fields ...string) bool {
	return s.Some(func(r Rule) bool {
		return matchVerbAndType(r, v, o.DocType()) && matchOnFields(r, o, fields...)
	})
}
