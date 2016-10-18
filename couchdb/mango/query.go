package mango

import (
	"encoding/json"
	"unicode"
)

// This package provides utility structures to build mango queries

////////////////////////////////////////////////////////////////
// Filter
///////////////////////////////////////////////////////////////

// ValueOperator is an operator between a field and a value
type ValueOperator string

// Eq ($eq) checks that field == value
const eq ValueOperator = "$eq"

// Gt ($gt) checks that field > value
const gt ValueOperator = "$gt"

// Gte ($gte) checks that field >= value
const gte ValueOperator = "$gte"

// Lt ($lt) checks that field < value
const lt ValueOperator = "$lt"

// Lte ($lte) checks that field <= value
const lte ValueOperator = "$lte"

// LogicOperator is an operator between two filters
type LogicOperator string

// And ($and) checks that filter && filter2
const and LogicOperator = "$and"

// Not ($not) checks that !filter
const not LogicOperator = "$not"

// Or ($or) checks that filter1 || filter2 || ...
const or LogicOperator = "$or"

// Nor ($nor) checks that !(filter1 || filter2 || ...)
const nor LogicOperator = "$nor"

// A Filter is a filter on documents, to be passed
// as the selector of a couchdb.FindRequest
// In the future, we might add go-side validation
// but we will need to duplicate the couchdb UCA algorithm
type Filter interface {
	json.Marshaler
	ToMango() map[string]interface{}
}

// valueFilter is a filter on a single field
type valueFilter struct {
	field string
	op    ValueOperator
	value interface{}
}

// ToMango implements the Filter interface on valueFilter
// it returns a map, either `{field: value}` or `{field: {$op: value}}`
func (vf *valueFilter) ToMango() map[string]interface{} {
	var value interface{}
	if vf.op == eq {
		value = vf.value
	} else {
		value = makeMap(string(vf.op), vf.value)
	}
	return makeMap(vf.field, value)
}

func (vf valueFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(vf.ToMango())
}

// logicFilter is a combination of filters with logic operator
type logicFilter struct {
	op      LogicOperator
	filters []Filter
}

// ToMango implements the Filter interface on logicFilter
// We could add some logic to make $and queries more readable
// For instance
// {"$and": [{"field": {"$lt":6}}, {"field": {"$gt":3}}]
// ---> {"field": {"$lt":6, "$gt":3}
// but it doesnt improve performances.
func (lf logicFilter) ToMango() map[string]interface{} {

	// special case, $not has an arity of one
	if lf.op == not {
		return makeMap(string(lf.op), lf.filters[0].ToMango())
	}

	// all other LogicOperator works on arrays
	filters := make([]map[string]interface{}, len(lf.filters))
	for i, v := range lf.filters {
		filters[i] = v.ToMango()
	}
	return makeMap(string(lf.op), filters)
}

func (lf logicFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(lf.ToMango())
}

// ensure ValueFilter & LogicFilter match FilterInterface
var _ Filter = (*valueFilter)(nil)
var _ Filter = (*logicFilter)(nil)

// Some Filter creation function

// And returns a filter combining several filters
func And(filters ...Filter) Filter { return logicFilter{and, filters} }

// Or returns a filter combining several filters
func Or(filters ...Filter) Filter { return logicFilter{or, filters} }

// Nor returns a filter combining several filters
func Nor(filters ...Filter) Filter { return logicFilter{nor, filters} }

// Not returns a filter inversing another filter
func Not(filter Filter) Filter { return logicFilter{not, []Filter{filter}} }

// Equal returns a filter that check if a field == value
func Equal(field string, value interface{}) Filter { return &valueFilter{field, eq, value} }

// Gt returns a filter that check if a field > value
func Gt(field string, value interface{}) Filter { return &valueFilter{field, gt, value} }

// Gte returns a filter that check if a field >= value
func Gte(field string, value interface{}) Filter { return &valueFilter{field, gte, value} }

// Lt returns a filter that check if a field < value
func Lt(field string, value interface{}) Filter { return &valueFilter{field, lt, value} }

// Lte returns a filter that check if a field <= value
func Lte(field string, value interface{}) Filter { return &valueFilter{field, lte, value} }

// Between returns a filter that check if v1 <= field < v2
func Between(field string, v1 interface{}, v2 interface{}) Filter {
	return &logicFilter{op: and, filters: []Filter{
		&valueFilter{field, gte, v1},
		&valueFilter{field, lt, v2},
	}}
}

const uFFFF = string(unicode.MaxRune)

// StartWith returns a filter that check if field's string value start with prefix
func StartWith(field string, prefix string) Filter {
	return Between(field, prefix, prefix+uFFFF)
}

////////////////////////////////////////////////////////////////
// Sort
///////////////////////////////////////////////////////////////

// SortDirection can be either ASC or DESC
type SortDirection string

// Asc is the ascending sorting order
const Asc SortDirection = "asc"

// Desc is the descending sorting order
const Desc SortDirection = "desc"

// SortBy is a sorting rule to be used as the sort of a couchdb.FindRequest
// a (field, direction) combination.
type SortBy struct {
	Field     string
	Direction SortDirection
}

// MarshalJSON implements json.Marshaller on SortBy
// it will returns a json array [field, direction]
func (s SortBy) MarshalJSON() ([]byte, error) {
	asSlice := []string{s.Field, string(s.Direction)}
	return json.Marshal(asSlice)
}

// utility function to create a map with a single key
func makeMap(key string, value interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	out[key] = value
	return out
}
