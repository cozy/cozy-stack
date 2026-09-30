// Package messaging defines the publishing contract shared by services and workers.
package messaging

import "context"

// Publisher publishes a message.
type Publisher interface {
	Publish(ctx context.Context, req PublishRequest) error
}

// PublishRequest describes a single outgoing message.
type PublishRequest struct {
	// ContextName selects the configured RabbitMQ node.
	ContextName string
	// Exchange is the AMQP exchange name to publish to.
	Exchange string
	// RoutingKey is the AMQP routing key used to route the message. It is
	// empty for a fanout exchange.
	RoutingKey string
	// Payload is the application message body.
	Payload any
	// MessageID is an optional stable identifier for deduplication, tracing or
	// cross-system debugging when a message is observed in broker logs.
	MessageID string
	// DeliveryMode overrides the AMQP persistence mode. Zero means "persistent",
	// which is the safe default for stack messages.
	DeliveryMode uint8
	// UnroutableOK disables the AMQP mandatory flag. By default we keep it false
	// so unroutable messages are returned as errors instead of being dropped.
	UnroutableOK bool
}
