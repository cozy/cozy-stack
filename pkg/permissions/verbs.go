package permissions

import "strings"

const verbSep = ","
const allVerbs = "ALL"
const allVerbsLength = 5

// Verb is one of GET,POST,PUT,PATCH,DELETE
type Verb string

// All possible Verbs, a subset of http methods
const (
	GET    = Verb("GET")
	POST   = Verb("POST")
	PUT    = Verb("PUT")
	PATCH  = Verb("PATCH")
	DELETE = Verb("DELETE")
)

// VerbSet is a Set of Verbs
type VerbSet []Verb

func (vs VerbSet) String() string {
	out := ""
	if len(vs) == 0 || len(vs) == allVerbsLength {
		return allVerbs
	}
	for _, v := range vs {
		out += verbSep + string(v)
	}
	return out[1:]
}

// VerbSplit parse a string into a VerbSet
func VerbSplit(in string) VerbSet {
	if in == allVerbs {
		return ALL
	}
	verbs := strings.Split(in, verbSep)
	out := make(VerbSet, len(verbs))
	for i, v := range verbs {
		out[i] = Verb(v)
	}
	return out
}

// Verbs is a utility function to create VerbSets
func Verbs(vs ...Verb) VerbSet {
	return VerbSet(vs)
}

// ALL : the default VerbSet allows all Verbs
var ALL = Verbs(GET, POST, PUT, PATCH, DELETE)
