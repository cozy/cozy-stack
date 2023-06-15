package settings

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

var service *SettingsService

type Service interface {
	GetInstanceSettings(inst prefixer.Prefixer) (*couchdb.JSONDoc, error)
	SetInstanceSettings(inst prefixer.Prefixer, doc *couchdb.JSONDoc) error
}

func Init() *SettingsService {
	service = NewService()

	return service
}

// SettingsDocument returns the document with the settings of this instance
//
// Deprecated: Use [Service.GetInstanceSettings] instead
func SettingsDocument(inst prefixer.Prefixer) (*couchdb.JSONDoc, error) {
	return service.GetInstanceSettings(inst)
}
