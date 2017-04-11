package jobs

import (
	"encoding/json"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

type triggerDoc struct {
	t Trigger
}

func (t triggerDoc) ID() string         { return t.t.Infos().ID }
func (t triggerDoc) Rev() string        { return t.t.Infos().Rev }
func (t triggerDoc) DocType() string    { return consts.Triggers }
func (t triggerDoc) Clone() couchdb.Doc { return t }
func (t triggerDoc) SetID(id string)    { t.t.Infos().ID = id }
func (t triggerDoc) SetRev(rev string)  { t.t.Infos().Rev = rev }
func (t triggerDoc) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.t.Infos())
}

// CouchStorage implements the TriggerStorage interface and uses CouchDB as the
// underlying storage for triggers.
type CouchStorage struct {
	db couchdb.Database
}

// NewTriggerCouchStorage returns a new instance of CouchStorage using the
// specified database.
func NewTriggerCouchStorage(db couchdb.Database) *CouchStorage {
	return &CouchStorage{db}
}

// GetAll implements the GetAll method of the TriggerStorage.
func (s *CouchStorage) GetAll() ([]*TriggerInfos, error) {
	var infos []*TriggerInfos
	// TODO(pagination): use a sort of couchdb.WalkDocs function when available.
	req := &couchdb.AllDocsRequest{Limit: 100}
	if err := couchdb.GetAllDocs(s.db, consts.Triggers, req, &infos); err != nil {
		if couchdb.IsNoDatabaseError(err) {
			return infos, nil
		}
		return nil, err
	}
	return infos, nil
}

// Add implements the Add method of the TriggerStorage.
func (s *CouchStorage) Add(trigger Trigger) error {
	return couchdb.CreateDoc(s.db, &triggerDoc{trigger})
}

// Delete implements the Delete method of the TriggerStorage.
func (s *CouchStorage) Delete(trigger Trigger) error {
	return couchdb.DeleteDoc(s.db, &triggerDoc{trigger})
}
