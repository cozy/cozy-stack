package vfs

import (
	"errors"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
)

var ErrIteratorDone = errors.New("No more element in the iterator")

const IteratorDefaultFetchSize = 30

type IteratorOptions struct {
	StartKey string
	ByFetch  int
}

type Iterator struct {
	ctx    Context
	sel    mango.Filter
	opt    *IteratorOptions
	list   []*DirOrFileDoc
	offset int
	index  int
}

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

func (i *Iterator) Next() (*DirDoc, *FileDoc, error) {
	if len(i.list) == 0 || i.index >= len(i.list) {
		if err := i.fetch(); err != nil {
			return nil, nil, err
		}
	}
	d, f := i.list[i.index].Refine()
	i.index++
	return d, f, nil
}

func (i *Iterator) fetch() error {
	l := len(i.list)
	if l > 0 && l < i.opt.ByFetch {
		return ErrIteratorDone
	}

	i.offset += l
	i.index = 0
	i.list = i.list[:0]

	var skip int
	var sel mango.Filter
	if i.opt.StartKey != "" {
		// TODO: adapt this code when filtering and sorting are added to the
		// iterator
		sel = mango.And(i.sel, mango.AfterID(i.opt.StartKey))
	} else {
		sel = i.sel
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
