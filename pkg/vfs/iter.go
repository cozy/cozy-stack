package vfs

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
)

// LocalIterator is a struct allowing to iterate over the children of a
// directory. The iterator is not thread-safe.
type LocalIterator struct {
	db     couchdb.Database
	sel    mango.Filter
	opt    *IteratorOptions
	list   []*DirOrFileDoc
	offset int
	index  int
	done   bool
}

// NewLocalIterator return a new iterator.
func NewLocalIterator(db couchdb.Database, dir *DirDoc, opt *IteratorOptions) DirIterator {
	if opt == nil {
		opt = &IteratorOptions{ByFetch: IteratorDefaultFetchSize}
	}
	if opt.ByFetch == 0 {
		opt.ByFetch = IteratorDefaultFetchSize
	}
	sel := mango.Equal("dir_id", dir.DocID)
	if opt.AfterID != "" {
		// TODO: adapt this code when filtering and sorting are added to the
		// iterator
		sel = mango.And(sel, mango.Gt("_id", opt.AfterID))
	}
	return &LocalIterator{
		db:  db,
		sel: sel,
		opt: opt,
	}
}

// Next should be called to get the next directory or file children of the
// parent directory. If the error is ErrIteratorDone
func (i *LocalIterator) Next() (*DirDoc, *FileDoc, error) {
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
func (i *LocalIterator) fetch() error {
	l := len(i.list)
	if l > 0 && l < i.opt.ByFetch {
		i.done = true
		return ErrIteratorDone
	}

	i.offset += l
	i.index = 0
	i.list = i.list[:0]

	req := &couchdb.FindRequest{
		UseIndex: "dir-children",
		Selector: i.sel,
		Limit:    i.opt.ByFetch,
		Skip:     i.offset,
	}
	err := couchdb.FindDocs(i.db, consts.Files, req, &i.list)
	if err != nil {
		return err
	}
	if len(i.list) == 0 {
		return ErrIteratorDone
	}
	return nil
}
