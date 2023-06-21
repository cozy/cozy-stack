package settings

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/token"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/emailer"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

const TokenExpiration = 7 * 24 * time.Hour

var (
	ErrInvalidType = fmt.Errorf("invalid type")
	ErrInvalidID   = fmt.Errorf("invalid id")
)

// Storage used to persiste and fetch settings data.
type Storage interface {
	setInstanceSettings(db prefixer.Prefixer, doc *couchdb.JSONDoc) error
	getInstanceSettings(db prefixer.Prefixer) (*couchdb.JSONDoc, error)
}

// SettingsService handle the business logic around "settings".
//
// This service handle 2 structured documents present in [consts.Settings]
// - The "instance settings" ([consts.InstanceSettingsID])
// - The "bitwarden settings" ([consts.BitwardenSettingsID]) (#TODO)
type SettingsService struct {
	emailer  emailer.Emailer
	instance instance.Service
	token    token.Service
	storage  Storage
}

// NewService instantiates a new [SettingsService].
func NewService(
	emailer emailer.Emailer,
	instance instance.Service,
	token token.Service,
	storage Storage,
) *SettingsService {
	return &SettingsService{emailer, instance, token, storage}
}

// PublicName returns the settings' public name or a default one if missing
func (s *SettingsService) PublicName(db prefixer.Prefixer) (string, error) {
	doc, err := s.storage.getInstanceSettings(db)
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

// GetInstanceSettings allows for fetch directly the [consts.InstanceSettingsID] couchdb document.
func (s *SettingsService) GetInstanceSettings(db prefixer.Prefixer) (*couchdb.JSONDoc, error) {
	return s.storage.getInstanceSettings(db)
}

// SetInstanceSettings allows a set directly the [consts.InstanceSettingsID] couchdb document.
func (s *SettingsService) SetInstanceSettings(db prefixer.Prefixer, doc *couchdb.JSONDoc) error {
	return s.storage.setInstanceSettings(db, doc)
}

type UpdateEmailCmd struct {
	Passphrase []byte
	Email      string
}

// StartEmailUpdate will start the email updating process.
//
// This process consists of:
// - Validating the user with a password
// - Sending a validation email to the new address with a validation link
// - Confirm the new address when the validation link is called
func (s *SettingsService) StartEmailUpdate(inst *instance.Instance, cmd *UpdateEmailCmd) error {
	err := s.instance.CheckPassphrase(inst, cmd.Passphrase)
	if err != nil {
		return fmt.Errorf("failed to check passphrase: %w", err)
	}

	settings, err := s.storage.getInstanceSettings(inst)
	if err != nil {
		return fmt.Errorf("failed to fetch the instance: %w", err)
	}

	publicName, err := s.PublicName(inst)
	if err != nil {
		return fmt.Errorf("failed to retrieve the instance settings: %w", err)
	}

	settings.M["pending_email"] = cmd.Email

	token, err := s.token.GenerateAndSave(inst, token.EmailUpdate, cmd.Email, TokenExpiration)
	if err != nil {
		return fmt.Errorf("failed to generate and save the confirmation token: %w", err)
	}

	err = s.storage.setInstanceSettings(inst, settings)
	if err != nil {
		return fmt.Errorf("failed to save the settings changes: %w", err)
	}

	link := inst.PageURL("/settings/email/confirm", url.Values{
		"token": []string{token},
	})

	err = s.emailer.SendEmail(inst, &emailer.SendEmailCmd{
		TemplateName: "update_email",
		TemplateValues: map[string]interface{}{
			"PublicName":      publicName,
			"EmailUpdateLink": link,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send the email: %w", err)
	}

	return nil
}
