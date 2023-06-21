package settings

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// CouchdbStorage handle all the settings data in couchdb.
type CouchdbStorage struct {
}

// NewCouchdbStorage instantiates a new [CouchdbStorage].
func NewCouchdbStorage() *CouchdbStorage {
	return &CouchdbStorage{}
}

func (s *CouchdbStorage) setInstanceSettings(db prefixer.Prefixer, doc *couchdb.JSONDoc) error {
	if doc.DocType() != consts.Settings {
		return ErrInvalidType
	}

	if doc.ID() != consts.InstanceSettingsID {
		return fmt.Errorf("%w: have %q, expected %q", ErrInvalidID, doc.ID(), consts.InstanceSettingsID)
	}

	return couchdb.UpdateDoc(db, doc)
}

func (s *CouchdbStorage) getInstanceSettings(db prefixer.Prefixer) (*couchdb.JSONDoc, error) {
	doc := &couchdb.JSONDoc{}

	err := couchdb.GetDoc(db, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return nil, err
	}

	doc.Type = consts.Settings

	return doc, nil
}
