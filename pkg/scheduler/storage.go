package scheduler

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// triggerStorage interface is used to represent a persistent layer on which
// triggers are stored.
type triggerStorage interface {
	GetAll() ([]*TriggerInfos, error)
	Add(trigger Trigger) error
	Delete(trigger Trigger) error
}

// CouchStorage implements the TriggerStorage interface and uses CouchDB as the
// underlying storage for triggers.
type couchStorage struct{}

// newTriggerCouchStorage returns a new instance of CouchStorage using the
// specified database.
func newTriggerCouchStorage() triggerStorage {
	return &couchStorage{}
}

func (s *couchStorage) GetAll() ([]*TriggerInfos, error) {
	var infos []*TriggerInfos
	// TODO(pagination): use a sort of couchdb.WalkDocs function when available.
	req := &couchdb.AllDocsRequest{Limit: 1000}
	err := couchdb.GetAllDocs(couchdb.GlobalTriggersDB, consts.Triggers, req, &infos)
	if err != nil {
		if couchdb.IsNoDatabaseError(err) {
			return infos, nil
		}
		return nil, err
	}
	return infos, nil
}

func (s *couchStorage) Add(trigger Trigger) error {
	return couchdb.CreateDoc(couchdb.GlobalTriggersDB, trigger.Infos())
}

func (s *couchStorage) Delete(trigger Trigger) error {
	return couchdb.DeleteDoc(couchdb.GlobalTriggersDB, trigger.Infos())
}
