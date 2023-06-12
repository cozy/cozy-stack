package setting

import (
	"context"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

var service *Setting

// Service handles all the interactions with the "setting" notion.
type Service interface {
	GetSettings(ctx context.Context, db prefixer.Prefixer) (*couchdb.JSONDoc, error)
}

func Init() {
	storage := NewCouchdbStorage()
	service = NewSetting(storage)
}

// SettingsDocument returns the document with the settings of this instance
//
// Deprecated: Please uses [setting.Service] or [setting.Storage] instead.
func SettingsDocument(db prefixer.Prefixer) (*couchdb.JSONDoc, error) {
	return service.GetSettings(context.Background(), db)
}
