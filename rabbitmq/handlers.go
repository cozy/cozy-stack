package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
)

// PasswordChangeHandler handles password change messages.
type PasswordChangeHandler struct{}

// NewPasswordChangeHandler creates a new password change handler.
func NewPasswordChangeHandler() *PasswordChangeHandler {
	return &PasswordChangeHandler{}
}

// PasswordChangeMessage represents a password change message using the new schema.
type PasswordChangeMessage struct {
	TwakeID    string `json:"twakeId"`    // external user identifier
	Iterations int    `json:"iterations"` // PBKDF2 iterations used client-side (when applicable)
	Hash       string `json:"hash"`       // client-side hashed passphrase (base64)
	PublicKey  string `json:"publicKey"`  // Bitwarden public key (base64)
	PrivateKey string `json:"privateKey"` // Bitwarden private key (encrypted, CipherString)
	Key        string `json:"key"`        // encrypted symmetric key (CipherString)
	Timestamp  int64  `json:"timestamp"`  // unix timestamp of the event
	Domain     string `json:"domain"`     // domain of the instance, e.g. "twake.app"
}

// Handle processes a password change message.
func (h *PasswordChangeHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("password change: received password change message: %s", d.RoutingKey)

	var msg PasswordChangeMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		return fmt.Errorf("fpassword change:  ailed to unmarshal password change message: %w", err)
	}

	if msg.Domain == "" {
		return fmt.Errorf("password change: missing domain")
	}
	if msg.Hash == "" {
		return fmt.Errorf("password change: missing passphrase hash")
	}
	if msg.Iterations <= 0 {
		return fmt.Errorf("password change: missing iterations")
	}
	if msg.Key == "" || msg.PublicKey == "" || msg.PrivateKey == "" {
		return fmt.Errorf("password change: missing key materials")
	}

	inst, err := lifecycle.GetInstance(msg.Domain)
	if err != nil {
		return fmt.Errorf("password change: get instance: %w", err)
	}

	params := lifecycle.PassParameters{
		Pass:       []byte(msg.Hash),
		Iterations: msg.Iterations,
		Key:        msg.Key,
		PublicKey:  msg.PublicKey,
		PrivateKey: msg.PrivateKey,
	}
	if err := lifecycle.ForceUpdatePassphraseWithSHash(inst, []byte(msg.Hash), params); err != nil {
		return fmt.Errorf("password change: update passphrase: %w", err)
	}
	return nil
}

// UserSettingsUpdateHandler handles user settings update messages.
type UserSettingsUpdateHandler struct{}

// NewUserSettingsUpdateHandler creates a new user settings update handler.
func NewUserSettingsUpdateHandler() *UserSettingsUpdateHandler {
	return &UserSettingsUpdateHandler{}
}

// UserSettingsUpdateMessage represents a user settings update message.
type UserSettingsUpdateMessage struct {
	Domain   string                 `json:"domain"`
	Settings map[string]interface{} `json:"settings"`
	Version  int                    `json:"version"`
}

// Handle processes a user settings update message.
func (h *UserSettingsUpdateHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("Received user settings update message: %s", d.RoutingKey)
	//TODO process user setting handler
	return nil
}
