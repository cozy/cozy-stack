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
type couchStorage struct {
}

// NewTriggerCouchStorage returns a new instance of CouchStorage using the
// specified database.
func NewTriggerCouchStorage() TriggerStorage {
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
	return couchdb.CreateDoc(couchdb.GlobalTriggersDB, &triggerDoc{trigger})
}

func (s *couchStorage) Delete(trigger Trigger) error {
	return couchdb.DeleteDoc(couchdb.GlobalTriggersDB, &triggerDoc{trigger})
}
