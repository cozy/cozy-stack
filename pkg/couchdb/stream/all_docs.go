// The stream package can be used for streaming CouchDB responses in JSON
// format from the CouchDB cluster to a client, with the stack doing stuff like
// filtering some fields. It is way faster that doing a full parsing of the
// JSON response, doing stuff, and then reserialize to JSON for large payloads.
package stream

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ohler55/ojg/oj"
)

type allDocsFilter struct {
	// config
	fields   []string
	skipDDoc bool

	// state
	w          io.Writer
	row        oj.Builder // The current row without the filtered fields
	rowIsDDoc  bool       // The current row is a design doc
	path       string     // The JSON object keys leading to the current position, joined with `.` (inside a row)
	depth      int        // The number of `{` and `[` minus the number of `}` and `]`
	matchedAt  int        // The depth of an exact match on a field, or -1
	rejectedAt int        // The depth were no fields can match (partial or exact), or -1
	total      int        // The number of rows kept
	err        error
}

// NewAllDocsFilter creates an object that can be use to remove some fields
// from a response to the all_docs endpoint of CouchDB.
func NewAllDocsFilter(fields []string) *allDocsFilter {
	for k, v := range fields {
		fields[k] = "doc." + v
	}
	return &allDocsFilter{fields: fields}
}

// SkipDesignDocs must be called to configure the filter to also remove the
// design docs.
func (f *allDocsFilter) SkipDesignDocs() {
	f.skipDDoc = true
}

// Stream will read the JSON response from CouchDB as the r reader, and will
// write the filtered JSON to the w writer to be sent to the client.
func (f *allDocsFilter) Stream(r io.Reader, w io.Writer) error {
	f.w = w
	f.path = ""
	f.depth = 0
	f.matchedAt = -1
	f.rejectedAt = -1
	f.total = 0
	f.err = nil

	if err := oj.TokenizeLoad(r, f); err != nil {
		return err
	}
	return f.err
}

func (f *allDocsFilter) isKeptField() bool {
	// Decision has already been at an higher level
	if f.matchedAt >= 0 {
		return true
	}
	if f.rejectedAt >= 0 {
		return false
	}

	// Special cases
	if len(f.fields) == 0 {
		return true
	}
	if f.depth <= 3 {
		// offset, rows, and total_rows
		// id, key, value, and doc
		// -> we can remove key (same as id) to gain a few kbs in the response
		return f.path != "key"
	}

	// Looks at fields to decide
	for _, field := range f.fields {
		if field == f.path {
			return true
		}
	}
	return false
}

// currentKey returns the last object key we have seen.
func (f *allDocsFilter) currentKey() string {
	idx := strings.LastIndex(f.path, ".")
	if idx == -1 {
		return f.path
	}
	return f.path[idx+1:]
}

// popKey removes the given key from the path after we have finished processing
// its value.
func (f *allDocsFilter) popKey(key string) {
	pos := len(f.path) - len(key) - 1
	if pos > 0 {
		f.path = f.path[:pos]
	} else {
		f.path = ""
	}
}

// value is used for basic values in JSON: nulls, booleans, numbers and strings.
func (f *allDocsFilter) value(value interface{}) {
	var err error
	key := f.currentKey()
	if key == "[]" {
		if f.rejectedAt < 0 {
			err = f.row.Value(value)
		}
	} else {
		if f.isKeptField() {
			err = f.row.Value(value, key)
		}
		f.popKey(key)
	}
	if err != nil && f.err == nil {
		f.err = err
	}
}

func (f *allDocsFilter) Null() {
	f.value(nil)
}

func (f *allDocsFilter) Bool(b bool) {
	f.value(b)
}

func (f *allDocsFilter) Int(i int64) {
	if f.depth > 2 { // total_rows and offset are not kept from the reader
		f.value(i)
	}
}

func (f *allDocsFilter) Float(x float64) {
	f.value(x)
}

func (f *allDocsFilter) Number(n string) {
	if f.err == nil {
		f.err = fmt.Errorf("number %q is not supported", n)
	}
}

func (f *allDocsFilter) String(s string) {
	if f.skipDDoc && f.depth == 3 && f.path == "id" && strings.HasPrefix(s, "_design") {
		// skip design docs
		f.rowIsDDoc = true
		f.path = ""
	} else {
		f.value(s)
	}
}

func (f *allDocsFilter) Key(s string) {
	if f.path == "" {
		f.path = s
	} else {
		f.path += "." + s
	}
}

func (f *allDocsFilter) ObjectStart() {
	var err error
	switch f.depth {
	case 0: // global
		// nothing
	case 1: // rows array
		err = errors.New("unexpected case")
	case 2: // a row
		f.rowIsDDoc = false
		f.path = ""
		err = f.row.Object()
	case 3: // doc or value
		if len(f.fields) == 0 || f.path != "value" {
			err = f.row.Object(f.path)
		}
	default: // inside doc
		err = f.objectStartInDoc()
	}
	if err != nil && f.err == nil {
		f.err = err
	}
	f.depth++
}

func (f *allDocsFilter) objectStartInDoc() error {
	// We are inside an object that won't be copied to the response
	if f.rejectedAt >= 0 {
		return nil
	}

	// Objects inside an array are always kept
	key := f.currentKey()
	if key == "[]" {
		return f.row.Object()
	}

	// We need to keep cozyMetadata.uploadedBy if fields include cozyMetadata,
	// and we keep everything if fields is empty.
	if f.matchedAt >= 0 || len(f.fields) == 0 {
		return f.row.Object(key)
	}

	// Exact match
	for _, field := range f.fields {
		if field == f.path {
			f.matchedAt = f.depth
			return f.row.Object(key)
		}
	}

	// We need to keep metadata if fields include metadata.datetime
	for _, field := range f.fields {
		if strings.HasPrefix(field, f.path+".") {
			return f.row.Object(key)
		}
	}

	// We can remove this object from the response
	f.rejectedAt = f.depth
	return nil
}

func (f *allDocsFilter) ObjectEnd() {
	f.depth--

	switch f.depth {
	case 0: // global
		// nothing
	case 1: // rows array
		if f.err == nil {
			f.err = errors.New("unexpected case")
		}
	case 2: // a row
		if f.rowIsDDoc {
			f.row.Reset()
		} else {
			f.flushRow()
		}
	case 3: // doc or value
		if len(f.fields) == 0 || f.path != "value" {
			f.row.Pop()
		}
		f.path = ""
	default: // inside doc
		f.objectEndInDoc()
	}
}

func (f *allDocsFilter) objectEndInDoc() {
	if key := f.currentKey(); key != "[]" {
		f.popKey(key)
	}

	if f.rejectedAt >= 0 {
		if f.rejectedAt == f.depth {
			f.rejectedAt = -1
		}
		return
	}
	if f.matchedAt == f.depth {
		f.matchedAt = -1
	}

	f.row.Pop()
}

func (f *allDocsFilter) flushRow() {
	prefix := ""
	if f.total != 0 {
		prefix = ","
	}
	row := prefix + oj.JSON(f.row.Result()) + "\n"
	f.row.Reset()
	if _, err := f.w.Write([]byte(row)); err != nil && f.err != nil {
		f.err = err
	}
	f.total++
}

func (f *allDocsFilter) ArrayStart() {
	f.depth++

	if f.depth <= 2 {
		// Special case for the rows array
		if _, err := f.w.Write([]byte(`{"rows":[`)); err != nil && f.err == nil {
			f.err = err
		}
		return
	}

	key := f.currentKey()
	f.path += ".[]"

	if f.rejectedAt >= 0 {
		return
	}

	var err error
	if key == "[]" {
		err = f.row.Array()
	} else if f.isKeptField() {
		err = f.row.Array(key)
	} else {
		f.rejectedAt = f.depth - 1
	}
	if err != nil && f.err == nil {
		f.err = err
	}
}

func (f *allDocsFilter) ArrayEnd() {
	f.depth--

	if f.depth <= 2 {
		// Special case for the rows array
		buf := fmt.Sprintf(`],"offset":0,"total_rows":%d}`, f.total)
		if _, err := f.w.Write([]byte(buf)); err != nil && f.err == nil {
			f.err = err
		}
		return
	}

	f.popKey("[]")
	if key := f.currentKey(); key != "[]" {
		f.popKey(key)
	}

	if f.rejectedAt >= 0 {
		if f.rejectedAt == f.depth {
			f.rejectedAt = -1
		}
		return
	}

	f.row.Pop()
}
