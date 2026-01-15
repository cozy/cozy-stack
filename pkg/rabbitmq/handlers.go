package rabbitmq

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/crypto"
)

const DefaultDomain = "twake.app"

// PasswordChangeHandler handles password change messages.
type PasswordChangeHandler struct{}

// NewPasswordChangeHandler creates a new password change handler.
func NewPasswordChangeHandler() *PasswordChangeHandler {
	return &PasswordChangeHandler{}
}

// PasswordChangeMessage represents a password change message using the new schema.
type PasswordChangeMessage struct {
	TwakeID       string `json:"twakeId"`       // external user identifier
	Iterations    int    `json:"iterations"`    // PBKDF2 iterations used client-side (when applicable)
	Hash          string `json:"hash"`          // client-side hashed passphrase (base64)
	PublicKey     string `json:"publicKey"`     // [OPTIONAL] Bitwarden public key (base64)
	PrivateKey    string `json:"privateKey"`    // [OPTIONAL] Bitwarden private key (encrypted, CipherString)
	Key           string `json:"key"`           // [OPTIONAL] encrypted symmetric key (CipherString)
	Timestamp     int64  `json:"timestamp"`     // [OPTIONAL] unix timestamp of the event
	Domain        string `json:"domain"`        // [OPTIONAL] domain of the instance, e.g. "twake.app"
	WorkplaceFqdn string `json:"workplaceFqdn"` // The fully qualified workplace domain
}

// Handle processes a password change message.
func (h *PasswordChangeHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("password change: received password change message: %s", d.RoutingKey)
	log.Debugf("password change: message details - MessageId: %s, ContentType: %s, Body size: %d bytes", d.MessageId, d.ContentType, len(d.Body))

	var msg PasswordChangeMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		return fmt.Errorf("password change: failed to unmarshal password change message: %w", err)
	}
	log.Debugf("password change: successfully unmarshaled message for TwakeID: %s", msg.TwakeID)

	// Log incoming message with masked sensitive data
	log.Infof("password change: processing message - TwakeID: %s, Domain: %s, Iterations: %d, Hash: %s, PublicKey: %s, PrivateKey: %s, Key: %s, Timestamp: %d",
		msg.TwakeID,
		msg.Domain,
		msg.Iterations,
		maskSensitiveData(msg.Hash),
		maskSensitiveData(msg.PublicKey),
		maskSensitiveData(msg.PrivateKey),
		maskSensitiveData(msg.Key),
		msg.Timestamp)

	if msg.Domain == "" {
		log.Debugf("password change: no domain provided, using default: %s", DefaultDomain)
		msg.Domain = DefaultDomain
	}
	if msg.Hash == "" {
		log.Errorf("password change: validation failed - missing passphrase hash for TwakeID: %s", msg.TwakeID)
		return fmt.Errorf("password change: missing passphrase hash")
	}
	if msg.WorkplaceFqdn == "" {
		log.Errorf("password change: validation failed - missing workplaceFqdn for TwakeID: %s", msg.TwakeID)
		return fmt.Errorf("password change: missing workplaceFqdn")
	}
	if msg.Iterations <= 0 {
		return fmt.Errorf("password change: missing iterations")
	}
	log.Debugf("password change: message validation passed for TwakeID: %s", msg.TwakeID)

	decoded, err := decodePassword(msg.Hash)
	if err != nil {
		return err
	}
	params := lifecycle.PassParameters{
		Pass:       decoded,
		Iterations: msg.Iterations,
	}

	if msg.Key != "" {
		log.Debugf("password change: setting key parameter for TwakeID: %s", msg.TwakeID)
		params.Key = msg.Key
	}

	// if one of the keys is missing, do not update any of the keys
	if msg.PublicKey != "" && msg.PrivateKey != "" {
		log.Debugf("password change: setting public/private key parameters for TwakeID: %s", msg.TwakeID)
		params.PublicKey = msg.PublicKey
		params.PrivateKey = msg.PrivateKey
	} else {
		log.Debugf("password change: skipping key parameters (incomplete pair) for TwakeID: %s", msg.TwakeID)
	}

	log.Debugf("password change: retrieving instance for domain: %s", msg.WorkplaceFqdn)
	inst, err := lifecycle.GetInstance(msg.WorkplaceFqdn)
	if err != nil {
		return fmt.Errorf("password change: get instance: %w", err)
	}

	if err := lifecycle.ForceUpdatePassphraseWithSHash(inst, params.Pass, params); err != nil {
		return fmt.Errorf("password change: update passphrase: %w", err)
	}
	log.Infof("password change: successfully updated passphrase for instance: %s", inst.Domain)
	return nil
}

// UserCreatedHandler handles user creation messages.
type UserCreatedHandler struct{}

// NewUserCreatedHandler creates a new user created handler.
func NewUserCreatedHandler() *UserCreatedHandler {
	return &UserCreatedHandler{}
}

// UserCreatedMessage represents a user creation message.
type UserCreatedMessage struct {
	TwakeID       string `json:"twakeId"`
	Domain        string `json:"domain"`
	Mobile        string `json:"mobile"`
	InternalEmail string `json:"internalEmail"`
	Iterations    int    `json:"iterations"`
	Hash          string `json:"hash"`
	PublicKey     string `json:"publicKey"`
	PrivateKey    string `json:"privateKey"`
	Key           string `json:"key"`
	Timestamp     int64  `json:"timestamp"`
	WorkplaceFqdn string `json:"workplaceFqdn"` // The fully qualified workplace domain
}

// Handle processes a user created message.
func (h *UserCreatedHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("user.created: received message: %s", d.RoutingKey)
	log.Debugf("user.created: message details - MessageId: %s, ContentType: %s, Body size: %d bytes", d.MessageId, d.ContentType, len(d.Body))

	var msg UserCreatedMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		return fmt.Errorf("user.created: failed to unmarshal message: %w", err)
	}
	log.Debugf("user.created: successfully unmarshaled message for TwakeID: %s", msg.TwakeID)

	// Log incoming message with masked sensitive data
	log.Infof("user.created: processing message - TwakeID: %s, Domain: %s, Mobile: %s, InternalEmail: %s, Iterations: %d, Hash: %s, PublicKey: %s, PrivateKey: %s, Key: %s, Timestamp: %d",
		msg.TwakeID,
		msg.Domain,
		msg.Mobile,
		msg.InternalEmail,
		msg.Iterations,
		maskSensitiveData(msg.Hash),
		maskSensitiveData(msg.PublicKey),
		maskSensitiveData(msg.PrivateKey),
		maskSensitiveData(msg.Key),
		msg.Timestamp)

	// Basic validation
	if msg.TwakeID == "" {
		return fmt.Errorf("user.created: missing twakeId")
	}

	if msg.Domain == "" {
		log.Debugf("user.created: no domain provided, using default: %s", DefaultDomain)
		msg.Domain = DefaultDomain
	}
	if msg.Hash == "" {
		return fmt.Errorf("user.created: missing passphrase hash")
	}
	if msg.WorkplaceFqdn == "" {
		return fmt.Errorf("user.created: missing workplaceFqdn")
	}
	if msg.Iterations <= 0 {
		return fmt.Errorf("user.created: missing iterations")
	}
	log.Debugf("user.created: message validation passed for TwakeID: %s", msg.TwakeID)

	decoded, err := decodePassword(msg.Hash)
	if err != nil {
		return err
	}

	params := lifecycle.PassParameters{
		Pass:       decoded,
		Iterations: msg.Iterations,
	}

	if msg.Key != "" {
		log.Debugf("user.created: setting key parameter for TwakeID: %s", msg.TwakeID)
		params.Key = msg.Key
	}

	// if one of the keys is missing, do not update any of the keys
	if msg.PublicKey != "" && msg.PrivateKey != "" {
		log.Debugf("user.created: setting public/private key parameters for TwakeID: %s", msg.TwakeID)
		params.PublicKey = msg.PublicKey
		params.PrivateKey = msg.PrivateKey
	} else {
		log.Debugf("user.created: skipping key parameters (incomplete pair) for TwakeID: %s", msg.TwakeID)
	}

	log.Debugf("user.created: looking for instance for domain: %s", msg.WorkplaceFqdn)
	inst, err := lifecycle.GetInstance(msg.WorkplaceFqdn)
	if err != nil {
		return fmt.Errorf("user.created: get instance: %w", err)
	}

	if err := lifecycle.ForceUpdatePassphraseWithSHash(inst, params.Pass, params); err != nil {
		return fmt.Errorf("user.created: update passphrase: %w", err)
	}
	log.Infof("user.created: successfully updated passphrase for instance: %s (PasswordDefined: %v)", inst.Domain, inst.PasswordDefined)
	return nil
}

// UserPhoneUpdatedHandler handles user phone update messages.
type UserPhoneUpdatedHandler struct{}

// NewUserPhoneUpdatedHandler creates a new user phone update handler.
func NewUserPhoneUpdatedHandler() *UserPhoneUpdatedHandler {
	return &UserPhoneUpdatedHandler{}
}

// UserPhoneUpdatedMessage represents a user phone update message.
type UserPhoneUpdatedMessage struct {
	TwakeID       string `json:"twakeId"`
	Mobile        string `json:"mobile"`
	InternalEmail string `json:"internalEmail"`
	WorkplaceFqdn string `json:"workplaceFqdn"`
}

// Handle processes a user phone update message.
func (h *UserPhoneUpdatedHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("user.phone.updated: received message: %s", d.RoutingKey)
	log.Debugf("user.phone.updated: message details - MessageId: %s, ContentType: %s, Body size: %d bytes",
		d.MessageId, d.ContentType, len(d.Body))

	var msg UserPhoneUpdatedMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		return fmt.Errorf("user.phone.updated: failed to unmarshal message: %w", err)
	}

	log.Infof("user.phone.updated: processing message - TwakeID: %s, Mobile: %s, InternalEmail: %s, WorkplaceFqdn: %s",
		msg.TwakeID, msg.Mobile, msg.InternalEmail, msg.WorkplaceFqdn)

	if msg.TwakeID == "" {
		return fmt.Errorf("user.phone.updated: missing twakeId")
	}
	if msg.Mobile == "" {
		return fmt.Errorf("user.phone.updated: missing mobile")
	}
	if msg.WorkplaceFqdn == "" {
		return fmt.Errorf("user.phone.updated: missing workplaceFqdn")
	}

	inst, err := lifecycle.GetInstance(msg.WorkplaceFqdn)
	if err != nil {
		return fmt.Errorf("user.phone.updated: get instance: %w", err)
	}

	// Get current settings document
	settings, err := inst.SettingsDocument()
	if err != nil {
		return fmt.Errorf("user.phone.updated: get settings document: %w", err)
	}

	// Update phone in settings
	settings.M["phone"] = msg.Mobile

	if err := lifecycle.Patch(inst, &lifecycle.Options{
		SettingsObj:  settings,
		FromCloudery: true, // XXX: do not update the instance's phone on the Cloudery as its API does not permit it and the Cloudery only uses it when requesting the instance creation from the stack
	}); err != nil {
		return fmt.Errorf("user.phone.updated: update settings: %w", err)
	}

	log.Infof("user.phone.updated: successfully updated phone for instance: %s", inst.Domain)
	return nil
}

type DomainSubscriptionChangedHandler struct{}

func NewDomainSubscriptionChangedHandler() *DomainSubscriptionChangedHandler {
	return &DomainSubscriptionChangedHandler{}
}

type DomainSubscriptionChangedMessage struct {
	Domain     string               `json:"domain"`
	IsPaying   bool                 `json:"isPaying"`
	CanUpgrade bool                 `json:"canUpgrade"`
	Features   SubscriptionFeatures `json:"features"`
}

type SubscriptionFeatures struct {
	Stack SubscriptionStackFeatures `json:"stack"`
}

type SubscriptionStackFeatures struct {
	FeatureSets []string `json:"featureSets"`
	DiskQuota   string   `json:"diskQuota"`
}

func (h *DomainSubscriptionChangedHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("domain.subscription.changed: received message: %s", d.RoutingKey)
	log.Debugf("domain.subscription.changed: message details - MessageId: %s, ContentType: %s, Body size: %d bytes",
		d.MessageId, d.ContentType, len(d.Body))

	var msg DomainSubscriptionChangedMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		return fmt.Errorf("domain.subscription.changed: failed to unmarshal message: %w", err)
	}

	log.Infof("domain.subscription.changed: processing message - Domain: %s, IsPaying: %t, CanUpgrade: %t, Features: %v",
		msg.Domain, msg.IsPaying, msg.CanUpgrade, msg.Features)

	if msg.Domain == "" {
		return fmt.Errorf("domain.subscription.changed: missing organization domain")
	}

	if msg.Features.Stack.FeatureSets == nil {
		return fmt.Errorf("domain.subscription.changed: missing feature sets for organization %s: %v", msg.Domain, msg.Features)
	}
	if msg.Features.Stack.DiskQuota == "" {
		return fmt.Errorf("domain.subscription.changed: invalid missing disk quota for organization %s: %v", msg.Domain, msg.Features)
	}

	quota, err := strconv.ParseInt(msg.Features.Stack.DiskQuota, 10, 64)
	if err != nil {
		return fmt.Errorf("domain.subscription.changed: invalid disk quota for organization %s: %w", msg.Domain, err)
	}

	list, err := lifecycle.ListOrgInstances(msg.Domain)
	if err != nil {
		return fmt.Errorf("domain.subscription.changed: could not list instances for organization %s: %w", msg.Domain, err)
	}
	if len(list) == 0 {
		log.Infof("domain.subscription.changed: no instances found for organization %s", msg.Domain)
		return nil
	}

	for _, inst := range list {
		if err := lifecycle.Patch(inst, &lifecycle.Options{
			DiskQuota:    quota,
			FeatureSets:  msg.Features.Stack.FeatureSets,
			FromCloudery: true, // XXX: do not update the instance on the Cloudery as the message comes from the Cloudery
		}); err != nil {
			return fmt.Errorf("domain.subscription.changed: update instance %s: %w", inst.Domain, err)
		}
		log.Infof("domain.subscription.changed: successfully updated instance %s", inst.Domain)
	}

	log.Infof("domain.subscription.changed: successfully updated %d instances for organization %s", len(list), msg.Domain)

	return nil
}

// AppInstallHandler handles app installation and uninstallation messages.
type AppInstallHandler struct{}

// NewAppInstallHandler creates a new app install handler.
func NewAppInstallHandler() *AppInstallHandler {
	return &AppInstallHandler{}
}

// AppInstallMessage represents an app installation/uninstallation message.
type AppInstallMessage struct {
	Emitter       string `json:"emitter"`        // the application requesting the operation
	Type          string `json:"type"`           // "app.install" | "app.uninstall"
	WorkplaceFqdn string `json:"workplaceFqdn"`  // the instance FQDN
	InternalEmail string `json:"internalEmail"`   // internal email (optional)
	Reason        string `json:"reason"`        // why this operation is performed
	Slug          string `json:"slug"`          // the application slug
	Source        string `json:"source"`         // registry source URL (e.g. "registry://admin/stable")
	Timestamp     int64  `json:"timestamp"`     // when
}

// Handle processes an app installation/uninstallation message.
func (h *AppInstallHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("app.install: received message: %s", d.RoutingKey)
	log.Debugf("app.install: message details - MessageId: %s, ContentType: %s, Body size: %d bytes",
		d.MessageId, d.ContentType, len(d.Body))

	var msg AppInstallMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		return fmt.Errorf("app.install: failed to unmarshal message: %w", err)
	}

	log.Infof("app.install: processing message - Emitter: %s, Type: %s, Slug: %s, WorkplaceFqdn: %s, Reason: %s, Source: %s, Timestamp: %d",
		msg.Emitter, msg.Type, msg.Slug, msg.WorkplaceFqdn, msg.Reason, msg.Source, msg.Timestamp)

	// Extract action from type: "app.install" -> "install", "app.uninstall" -> "uninstall"
	var action string
	if msg.Type == "app.install" {
		action = "install"
	} else if msg.Type == "app.uninstall" {
		action = "uninstall"
	} else {
		return fmt.Errorf("app.install: invalid type %s, must be 'app.install' or 'app.uninstall'", msg.Type)
	}

	// Validation
	if msg.Emitter == "" {
		return fmt.Errorf("app.install: missing emitter")
	}
	if msg.Slug == "" {
		return fmt.Errorf("app.install: missing slug")
	}
	if msg.WorkplaceFqdn == "" {
		return fmt.Errorf("app.install: missing workplaceFqdn")
	}

	// Log the source application requesting the operation
	log.Infof("app.install: operation requested by emitter: %s", msg.Emitter)

	// Get the instance
	log.Debugf("app.install: retrieving instance for domain: %s", msg.WorkplaceFqdn)
	inst, err := lifecycle.GetInstance(msg.WorkplaceFqdn)
	if err != nil {
		return fmt.Errorf("app.install: get instance: %w", err)
	}

	// Process the action
	switch action {
	case "install":
		return h.handleInstall(inst, msg)
	case "uninstall":
		return h.handleUninstall(inst, msg)
	default:
		return fmt.Errorf("app.install: unknown action: %s", action)
	}
}

// handleInstall installs an application on the instance.
func (h *AppInstallHandler) handleInstall(inst *instance.Instance, msg AppInstallMessage) error {
	log.Infof("app.install: installing app %s on instance %s (reason: %s)", msg.Slug, msg.WorkplaceFqdn, msg.Reason)

	// Use provided source if available, otherwise construct default
	source := msg.Source
	if source == "" || !strings.HasPrefix(source, "registry://") {
		source = "registry://" + msg.Slug + "/stable"
	}

	// Try to install as webapp first, then as konnector if it fails
	appTypes := []consts.AppType{consts.WebappType, consts.KonnectorType}
	var lastErr error

	for _, appType := range appTypes {
		installer, err := app.NewInstaller(inst, app.Copier(appType, inst), &app.InstallerOptions{
			Operation:  app.Install,
			Type:       appType,
			SourceURL:  source,
			Slug:       msg.Slug,
			Registries: inst.Registries(),
		})
		if err != nil {
			log.Debugf("app.install: failed to create installer for %s as %s: %v", msg.Slug, appType.String(), err)
			lastErr = err
			continue
		}

		_, err = installer.RunSync()
		if err != nil {
			// If the app already exists, that's OK
			if errors.Is(err, app.ErrAlreadyExists) {
				log.Infof("app.install: app %s already exists on instance %s", msg.Slug, msg.WorkplaceFqdn)
				return nil
			}
			log.Debugf("app.install: failed to install %s as %s: %v", msg.Slug, appType.String(), err)
			lastErr = err
			continue
		}

		log.Infof("app.install: successfully installed app %s as %s on instance %s", msg.Slug, appType.String(), msg.WorkplaceFqdn)
		return nil
	}

	return fmt.Errorf("app.install: failed to install app %s on instance %s: %w", msg.Slug, msg.WorkplaceFqdn, lastErr)
}

// handleUninstall uninstalls an application from the instance.
func (h *AppInstallHandler) handleUninstall(inst *instance.Instance, msg AppInstallMessage) error {
	log.Infof("app.install: uninstalling app %s from instance %s (reason: %s)", msg.Slug, msg.WorkplaceFqdn, msg.Reason)

	// First, try to find the app to determine its type
	appTypes := []consts.AppType{consts.WebappType, consts.KonnectorType}
	var foundType consts.AppType
	var found bool

	for _, appType := range appTypes {
		_, err := app.GetBySlug(inst, msg.Slug, appType)
		if err == nil {
			foundType = appType
			found = true
			break
		}
	}

	if !found {
		log.Infof("app.install: app %s not found on instance %s, nothing to uninstall", msg.Slug, msg.WorkplaceFqdn)
		return nil // Not an error if the app doesn't exist
	}

	// Uninstall the app
	installer, err := app.NewInstaller(inst, app.Copier(foundType, inst), &app.InstallerOptions{
		Operation:  app.Delete,
		Type:       foundType,
		Slug:       msg.Slug,
		Registries: inst.Registries(),
	})
	if err != nil {
		return fmt.Errorf("app.install: failed to create uninstaller: %w", err)
	}

	_, err = installer.RunSync()
	if err != nil {
		return fmt.Errorf("app.install: failed to uninstall app %s: %w", msg.Slug, err)
	}

	log.Infof("app.install: successfully uninstalled app %s from instance %s", msg.Slug, msg.WorkplaceFqdn)
	return nil
}
func decodePassword(hash string) ([]byte, error) {
	// Decode base64 hash if applicable
	decoded, err := base64.StdEncoding.DecodeString(hash)
	if err != nil {
		return nil, fmt.Errorf("user.created: invalid passphrase hash format: %w", err)
	}

	// Validate the hash format using scrypt's exported validator (uses UnmarshalText internally)
	if err := crypto.ValidateScryptHashFormat(decoded); err != nil {
		return nil, fmt.Errorf("user.created: invalid passphrase hash format: %w", err)
	}
	return decoded, nil
}

func maskSensitiveData(data string) string {
	if data == "" {
		return ""
	}
	// Show first 3 and last 3 characters, mask the middle with asterisks
	if len(data) <= 8 {
		return strings.Repeat("*", len(data))
	}
	return data[:3] + strings.Repeat("*", len(data)-6) + data[len(data)-3:]
}
