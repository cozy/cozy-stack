package permissions

// Validable is an interface for a object than can be validated by a Set
type Validable interface {
	ID() string
	DocType() string
	Valid(field, expected string) bool
}

func validValues(r Rule, o Validable) bool {
	// empty r.Values = any value
	if len(r.Values) == 0 {
		return true
	}
	if r.Selector == "" {
		return r.ValuesContain(o.ID())
	}
	return r.ValuesValid(o)
}

func validOnFields(r Rule, o Validable, fields ...string) bool {
	// in this case, if r.Values is empty the selector is considered too wide and
	// is forbidden
	if len(r.Values) == 0 || r.Selector == "" {
		return false
	}
	var validSelector bool
	for _, f := range fields {
		if r.Selector == f {
			validSelector = true
			break
		}
	}
	if !validSelector {
		return false
	}
	return r.ValuesValid(o)
}

func validVerbAndType(r Rule, v Verb, doctype string) bool {
	return r.Verbs.Contains(v) && r.Type == doctype
}

func validWholeType(r Rule) bool {
	return len(r.Values) == 0
}

func validID(r Rule, id string) bool {
	return r.Selector == "" && r.ValuesContain(id)
}

// AllowWholeType returns true if the set allows to apply verb to every
// document from the given doctypes (ie. r.values == 0)
func (s Set) AllowWholeType(v Verb, doctype string) bool {
	return s.Some(func(r Rule) bool {
		return validVerbAndType(r, v, doctype) && validWholeType(r)
	})
}

// AllowID returns true if the set allows to apply verb to given type & id
func (s Set) AllowID(v Verb, doctype, id string) bool {
	return s.Some(func(r Rule) bool {
		return validVerbAndType(r, v, doctype) && (validWholeType(r) || validID(r, id))
	})
}

// Allow returns true if the set allows to apply verb to given doc
func (s Set) Allow(v Verb, o Validable) bool {
	return s.Some(func(r Rule) bool {
		return validVerbAndType(r, v, o.DocType()) && validValues(r, o)
	})
}

// AllowOnFields returns true if the set allows to apply verb to given doc on
// the specified fields.
func (s Set) AllowOnFields(v Verb, o Validable, fields ...string) bool {
	return s.Some(func(r Rule) bool {
		return validVerbAndType(r, v, o.DocType()) && validOnFields(r, o, fields...)
	})
}
