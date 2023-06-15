package settings

import (
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

var (
	ErrInvalidType = fmt.Errorf("invalid type")
	ErrInvalidID   = fmt.Errorf("invalid id")
)

// SettingsService handle the business logic around "settings".
//
// This service handle 2 structured documents present in [consts.Settings]
// - The "instance settings" ([consts.InstanceSettingsID])
// - The "bitwarden settings" ([consts.BitwardenSettingsID]) (#TODO)
type SettingsService struct {
}

// NewService instantiates a new [SettingsService].
func NewService() *SettingsService {
	return &SettingsService{}
}

// PublicName returns the settings' public name or a default one if missing
func (s *SettingsService) PublicName(db prefixer.Prefixer) (string, error) {
	doc, err := s.GetInstanceSettings(db)
	if err != nil {
		return "", err
	}
	publicName, _ := doc.M["public_name"].(string)
	// if the public name is not defined, use the instance's domain
	if publicName == "" {
		split := strings.Split(db.DomainName(), ".")
		publicName = split[0]
	}
	return publicName, nil
}

func (s *SettingsService) GetInstanceSettings(inst prefixer.Prefixer) (*couchdb.JSONDoc, error) {
	doc := &couchdb.JSONDoc{}

	err := couchdb.GetDoc(inst, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return nil, err
	}

	doc.Type = consts.Settings

	return doc, nil
}

func (s *SettingsService) SetInstanceSettings(inst prefixer.Prefixer, doc *couchdb.JSONDoc) error {
	// TODO: Validate input

	if doc.DocType() != consts.Settings {
		return ErrInvalidType
	}

	if doc.ID() != consts.InstanceSettingsID {
		return fmt.Errorf("%w: have %q, expected %q", ErrInvalidID, doc.ID(), consts.InstanceSettingsID)
	}

	return couchdb.UpdateDoc(inst, doc)
}
