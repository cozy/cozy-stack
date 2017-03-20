package vfs

import (
	"errors"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
)

// ErrIteratorDone is returned by the Next() merthod of the iterator when
// the iterator is actually done.
var ErrIteratorDone = errors.New("No more element in the iterator")

// IteratorDefaultFetchSize is the default number of elements fetched from
// couchdb on each iteration.
const IteratorDefaultFetchSize = 100

// IteratorOptions contains the options of the iterator.
type IteratorOptions struct {
	StartKey string
	ByFetch  int
}

// Iterator is a struct allowing to iterate over the children of a directory.
// The iterator is not thread-safe.
type Iterator struct {
	ctx    Context
	sel    mango.Filter
	opt    *IteratorOptions
	list   []*DirOrFileDoc
	offset int
	index  int
	done   bool
}

// NewIterator return a new iterator.
func NewIterator(c Context, sel mango.Filter, opt *IteratorOptions) *Iterator {
	if opt == nil {
		opt = &IteratorOptions{ByFetch: IteratorDefaultFetchSize}
	}
	if opt.ByFetch == 0 {
		opt.ByFetch = IteratorDefaultFetchSize
	}
	return &Iterator{
		ctx: c,
		sel: sel,
		opt: opt,
	}
}

// Next should be called to get the next directory or file children of the
// parent directory. If the error is ErrIteratorDone
func (i *Iterator) Next() (*DirDoc, *FileDoc, error) {
	if i.done {
		return nil, nil, ErrIteratorDone
	}
	if i.index >= len(i.list) {
		if err := i.fetch(); err != nil {
			return nil, nil, err
		}
	}
	d, f := i.list[i.index].Refine()
	i.index++
	return d, f, nil
}

// fetch should be called when the index is out of the list boundary.
func (i *Iterator) fetch() error {
	l := len(i.list)
	if l > 0 && l < i.opt.ByFetch {
		i.done = true
		return ErrIteratorDone
	}

	i.offset += l
	i.index = 0
	i.list = i.list[:0]

	var skip int
	sel := i.sel
	if i.opt.StartKey != "" {
		// TODO: adapt this code when filtering and sorting are added to the
		// iterator
		sel = mango.And(sel, mango.AfterID(i.opt.StartKey))
	} else {
		skip = i.offset
	}

	req := &couchdb.FindRequest{
		Selector: sel,
		Limit:    i.opt.ByFetch,
		Skip:     skip,
	}
	err := couchdb.FindDocs(i.ctx, consts.Files, req, &i.list)
	if err != nil {
		return err
	}
	if len(i.list) == 0 {
		return ErrIteratorDone
	}
	return nil
}
