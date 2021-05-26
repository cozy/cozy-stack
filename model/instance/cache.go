package instance

import (
	"encoding/json"

	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// GetFromCouch finds an instance in CouchDB from its domain.
func GetFromCouch(domain string) (*Instance, error) {
	var res couchdb.ViewResponse
	err := couchdb.ExecView(couchdb.GlobalDB, couchdb.DomainAndAliasesView, &couchdb.ViewRequest{
		Key:         domain,
		IncludeDocs: true,
		Limit:       1,
	}, &res)
	if couchdb.IsNoDatabaseError(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(res.Rows) == 0 {
		return nil, ErrNotFound
	}
	inst := &Instance{}
	err = json.Unmarshal(res.Rows[0].Doc, &inst)
	if err != nil {
		return nil, err
	}
	if err = inst.MakeVFS(); err != nil {
		return nil, err
	}
	return inst, nil
}

// Update saves the changes in CouchDB.
func (inst *Instance) Update() error {
	return couchdb.UpdateDoc(couchdb.GlobalDB, inst)
}

// Delete removes the instance document in CouchDB.
func (inst *Instance) Delete() error {
	return couchdb.DeleteDoc(couchdb.GlobalDB, inst)
}
