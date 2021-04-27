package app

import (
	"encoding/json"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// ActivateMaintenance activates maintenance for the given konnector.
func ActivateMaintenance(slug string, opts map[string]interface{}) error {
	doc, err := loadMaintenance(slug)
	if err != nil {
		return err
	}
	doc.M = opts
	if doc.M == nil {
		doc.M = map[string]interface{}{}
	}
	doc.M["flag_infra_maintenance"] = true
	doc.SetID(slug)
	return couchdb.Upsert(couchdb.GlobalDB, &doc)
}

// DeactivateMaintenance disables maintenance for the given konnector.
func DeactivateMaintenance(slug string) error {
	doc, err := loadMaintenance(slug)
	if err != nil {
		return err
	}
	if doc.M == nil {
		doc.M = map[string]interface{}{}
	}
	doc.SetID(slug)
	return couchdb.DeleteDoc(couchdb.GlobalDB, &doc)
}

func loadMaintenance(slug string) (couchdb.JSONDoc, error) {
	var doc couchdb.JSONDoc
	err := couchdb.GetDoc(couchdb.GlobalDB, consts.KonnectorsMaintenance, slug, &doc)
	if err != nil && !couchdb.IsNotFoundError(err) {
		return doc, err
	}
	doc.Type = consts.KonnectorsMaintenance
	return doc, nil
}

// ListMaintenance returns the list of konnectors in maintenance for the stack
// (not from apps registry).
func ListMaintenance() ([]interface{}, error) {
	list := []interface{}{}
	err := couchdb.ForeachDocs(couchdb.GlobalDB, consts.KonnectorsMaintenance, func(id string, raw json.RawMessage) error {
		var opts map[string]interface{}
		if err := json.Unmarshal(raw, &opts); err != nil {
			return err
		}
		delete(opts, "_id")
		delete(opts, "_rev")
		doc := map[string]interface{}{
			"slug":                  id,
			"type":                  "konnector",
			"maintenance_activated": true,
			"maintenance_options":   opts,
		}
		list = append(list, doc)
		return nil
	})
	if err != nil && !couchdb.IsNotFoundError(err) {
		return nil, err
	}
	return list, nil
}
