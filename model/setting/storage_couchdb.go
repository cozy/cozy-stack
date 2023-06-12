package setting

import (
	"context"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// CouchdbStorage handle all the interactions with the
type CouchdbStorage struct {
}

// NewCouchdbStorage instantiates a new [CouchdbStorage].
func NewCouchdbStorage() *CouchdbStorage {
	return &CouchdbStorage{}
}

func (s *CouchdbStorage) get(ctx context.Context, db prefixer.Prefixer) (*couchdb.JSONDoc, error) {
	doc := &couchdb.JSONDoc{}

	err := couchdb.GetDoc(db, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return nil, err
	}

	doc.Type = consts.Settings

	return doc, nil
}
