package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// PasswordChangeHandler handles password change messages.
type PasswordChangeHandler struct{}

// NewPasswordChangeHandler creates a new password change handler.
func NewPasswordChangeHandler() *PasswordChangeHandler {
	return &PasswordChangeHandler{}
}

// PasswordChangeMessage represents a password change message.
type PasswordChangeMessage struct {
	Domain      string `json:"domain"`
	NewPassword string `json:"new_password"`
	Version     int    `json:"version"`
}

// Handle processes a password change message.
func (h *PasswordChangeHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("Received password change message: %s", d.RoutingKey)

	var msg PasswordChangeMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		return fmt.Errorf("failed to unmarshal password change message: %w", err)
	}

	//instance, err := lifecycle.GetInstance(msg.Domain)
	//TODO process password change event
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
