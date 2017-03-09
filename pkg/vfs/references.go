package vfs

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

// FilesReferencedBy returns a slice of ResourceIdentifier  to all File
// documents which are referenced_by the passed document
// @TODO pagination
func FilesReferencedBy(db couchdb.Database, doctype, id string) ([]jsonapi.ResourceIdentifier, error) {
	var res couchdb.ViewResponse
	err := couchdb.ExecView(db, consts.FilesReferencedByView, &couchdb.ViewRequest{
		Key: []string{doctype, id},
	}, &res)
	if err != nil {
		return nil, err
	}

	var out = make([]jsonapi.ResourceIdentifier, len(res.Rows))
	for i, row := range res.Rows {
		out[i] = jsonapi.ResourceIdentifier{
			ID:   row.ID,
			Type: consts.Files,
		}
	}

	return out, nil
}
