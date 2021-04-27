package app

import (
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
