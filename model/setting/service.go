package setting

import (
	"context"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Storage is implemented by all the settings storage implementations.
type Storage interface {
	get(ctx context.Context, db prefixer.Prefixer) (*couchdb.JSONDoc, error)
}

// SettingService is the main implementation of [Service].
type SettingService struct {
	storage Storage
}

// NewService instantiates a new [Setting].
func NewService(storage Storage) *SettingService {
	return &SettingService{storage}
}

// SettingsDocument returns the settings document for the giver prefixer.
func (s *SettingService) GetSettings(ctx context.Context, db prefixer.Prefixer) (*couchdb.JSONDoc, error) {
	return s.storage.get(ctx, db)
}
