package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
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
	TwakeID    string `json:"twakeId"`    // external user identifier
	Iterations int    `json:"iterations"` // PBKDF2 iterations used client-side (when applicable)
	Hash       string `json:"hash"`       // client-side hashed passphrase (base64)
	PublicKey  string `json:"publicKey"`  // [OPTIONAL] Bitwarden public key (base64)
	PrivateKey string `json:"privateKey"` // [OPTIONAL] Bitwarden private key (encrypted, CipherString)
	Key        string `json:"key"`        // [OPTIONAL] encrypted symmetric key (CipherString)
	Timestamp  int64  `json:"timestamp"`  // [OPTIONAL] unix timestamp of the event
	Domain     string `json:"domain"`     // [OPTIONAL] domain of the instance, e.g. "twake.app"
}

// Handle processes a password change message.
func (h *PasswordChangeHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("password change: received password change message: %s", d.RoutingKey)

	var msg PasswordChangeMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		return fmt.Errorf("password change:  ailed to unmarshal password change message: %w", err)
	}

	if msg.Domain == "" {
		msg.Domain = DefaultDomain
	}
	if msg.Hash == "" {
		return fmt.Errorf("password change: missing passphrase hash")
	}
	if msg.Iterations <= 0 {
		return fmt.Errorf("password change: missing iterations")
	}

	params := lifecycle.PassParameters{
		Pass:       []byte(msg.Hash),
		Iterations: msg.Iterations,
	}

	if msg.Key != "" {
		params.Key = msg.Key
	}

	// if one of the keys is missing, do not update any of the keys
	if msg.PublicKey != "" && msg.PrivateKey != "" {
		params.PublicKey = msg.PublicKey
		params.PrivateKey = msg.PrivateKey
	}

	inst, err := lifecycle.GetInstance(msg.Domain)
	if err != nil {
		return fmt.Errorf("password change: get instance: %w", err)
	}

	if err := lifecycle.ForceUpdatePassphraseWithSHash(inst, []byte(msg.Hash), params); err != nil {
		return fmt.Errorf("password change: update passphrase: %w", err)
	}
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
}

// Handle processes a user created message.
func (h *UserCreatedHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("user.created: received message: %s", d.RoutingKey)

	var msg UserCreatedMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		return fmt.Errorf("user.created: failed to unmarshal message: %w", err)
	}

	// Basic validation
	if msg.TwakeID == "" {
		return fmt.Errorf("user.created: missing twakeId")
	}

	if msg.Domain == "" {
		msg.Domain = DefaultDomain
	}
	if msg.Hash == "" {
		return fmt.Errorf("user.created: missing passphrase hash")
	}
	if msg.Iterations <= 0 {
		return fmt.Errorf("user.created: missing iterations")
	}

	params := lifecycle.PassParameters{
		Pass:       []byte(msg.Hash),
		Iterations: msg.Iterations,
	}

	if msg.Key != "" {
		params.Key = msg.Key
	}

	// if one of the keys is missing, do not update any of the keys
	if msg.PublicKey != "" && msg.PrivateKey != "" {
		params.PublicKey = msg.PublicKey
		params.PrivateKey = msg.PrivateKey
	}

	inst, err := lifecycle.GetInstance(msg.Domain)
	if err != nil {
		return fmt.Errorf("user.created: get instance: %w", err)
	}

	if err := lifecycle.ForceUpdatePassphraseWithSHash(inst, []byte(msg.Hash), params); err != nil {
		return fmt.Errorf("user.created: update passphrase: %w", err)
	}
	return nil
}
