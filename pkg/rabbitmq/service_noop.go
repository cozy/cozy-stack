package rabbitmq

import (
	"context"
	"errors"
)

// ErrNotConfigured is returned when attempting to publish without a RabbitMQ configuration.
var ErrNotConfigured = errors.New("rabbitmq is not configured")

// NoopService implements [Service].
//
// This implem does nothing. It is used when no config is provided.
type NoopService struct{}

// StartManagers does nothing.
func (s *NoopService) StartManagers() ([]*RabbitMQManager, error) {
	log.Warnf("No RabbitMQ managers to start")
	return nil, nil
}

// Publish returns an error because RabbitMQ is not configured.
func (s *NoopService) Publish(_ context.Context, _, _, _ string, _ []byte) error {
	return ErrNotConfigured
}

// ClosePublishers does nothing.
func (s *NoopService) ClosePublishers(_ context.Context) error {
	return nil
}
