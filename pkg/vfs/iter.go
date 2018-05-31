package vfs

import (
	"path"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

const iterMaxFetchSize = 256

// iter is a struct allowing to iterate over the children of a
// directory. The iterator is not thread-safe.
type iter struct {
	db     prefixer.Prefixer
	sel    mango.Filter
	opt    *IteratorOptions
	list   []*DirOrFileDoc
	path   string
	offset int
	index  int
	done   bool
}

// NewIterator return a new iterator.
func NewIterator(db prefixer.Prefixer, dir *DirDoc, opt *IteratorOptions) DirIterator {
	if opt == nil {
		opt = &IteratorOptions{ByFetch: iterMaxFetchSize}
	}
	if opt.ByFetch == 0 || opt.ByFetch > iterMaxFetchSize {
		opt.ByFetch = iterMaxFetchSize
	}
	sel := mango.Equal("dir_id", dir.DocID)
	if opt.AfterID == "" {
		sel = mango.And(sel, mango.Exists("_id"))
	} else {
		// TODO: adapt this code when filtering and sorting are added to the
		// iterator
		sel = mango.And(sel, mango.Gt("_id", opt.AfterID))
	}
	return &iter{
		db:   db,
		sel:  sel,
		opt:  opt,
		path: dir.Fullpath,
	}
}

// Next should be called to get the next directory or file children of the
// parent directory. If the error is ErrIteratorDone
func (i *iter) Next() (*DirDoc, *FileDoc, error) {
	if i.done {
		return nil, nil, ErrIteratorDone
	}
	if i.index >= len(i.list) {
		if err := i.fetch(); err != nil {
			return nil, nil, err
		}
	}
	d, f := i.list[i.index].Refine()
	if f != nil {
		f.fullpath = path.Join(i.path, f.DocName)
	}
	i.index++
	return d, f, nil
}

// fetch should be called when the index is out of the list boundary.
func (i *iter) fetch() error {
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
