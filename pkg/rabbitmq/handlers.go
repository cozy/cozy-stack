package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	if msg.Iterations <= 0 {
		return fmt.Errorf("password change: missing iterations")
	}
	log.Debugf("password change: message validation passed for TwakeID: %s", msg.TwakeID)

	params := lifecycle.PassParameters{
		Pass:       []byte(msg.Hash),
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

	userDomain := msg.TwakeID + "." + msg.Domain
	log.Debugf("password change: retrieving instance for domain: %s", userDomain)
	inst, err := lifecycle.GetInstance(userDomain)
	if err != nil {
		return fmt.Errorf("password change: get instance: %w", err)
	}

	if err := lifecycle.ForceUpdatePassphraseWithSHash(inst, []byte(msg.Hash), params); err != nil {
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
	if msg.Iterations <= 0 {
		return fmt.Errorf("user.created: missing iterations")
	}
	log.Debugf("user.created: message validation passed for TwakeID: %s", msg.TwakeID)

	params := lifecycle.PassParameters{
		Pass:       []byte(msg.Hash),
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

	userDomain := msg.TwakeID + "." + msg.Domain
	log.Debugf("user.created: looking for instance for domain: %s", userDomain)
	inst, err := lifecycle.GetInstance(userDomain)
	if err != nil {
		return fmt.Errorf("user.created: get instance: %w", err)
	}

	if err := lifecycle.ForceUpdatePassphraseWithSHash(inst, []byte(msg.Hash), params); err != nil {
		return fmt.Errorf("user.created: update passphrase: %w", err)
	}
	log.Infof("user.created: successfully updated passphrase for instance: %s (PasswordDefined: %v)", inst.Domain, inst.PasswordDefined)
	return nil
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
