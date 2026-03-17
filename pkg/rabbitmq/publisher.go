package rabbitmq

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher owns a dedicated RabbitMQ connection for publishing messages,
// completely independent from the consumer manager's connection.
type Publisher struct {
	connection *RabbitMQConnection
}

// NewPublisher creates a publisher with its own connection.
func NewPublisher(connection *RabbitMQConnection) *Publisher {
	return &Publisher{connection: connection}
}

// Publish sends a persistent JSON message to the given exchange and routing key.
// It opens a transient channel per call, which is appropriate for low-frequency
// publishing (e.g. user-initiated actions).
func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	conn, err := p.connection.Connect(ctx, 3)
	if err != nil {
		return fmt.Errorf("rabbitmq publish: connect: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq publish: open channel: %w", err)
	}
	defer ch.Close()

	return ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		Body:         body,
	})
}

// Close closes the publisher's connection.
func (p *Publisher) Close() error {
	return p.connection.Close()
}
