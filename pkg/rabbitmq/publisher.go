package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/pkg/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	// ErrManagerNotFound is returned when no RabbitMQ node matches the requested context.
	ErrManagerNotFound = errors.New("rabbitmq context is not configured")
	// ErrPublishReturned is returned when RabbitMQ accepts the publish but cannot route it.
	ErrPublishReturned = errors.New("rabbitmq publish was returned")
	// ErrPublishNacked is returned when RabbitMQ negatively acknowledges a publish.
	ErrPublishNacked = errors.New("rabbitmq publish was nacked")
)

// PublishRequest describes a single outgoing RabbitMQ message.
type PublishRequest = messaging.PublishRequest

// PublishReturnedError reports an unroutable publish returned by RabbitMQ.
type PublishReturnedError struct {
	Exchange   string
	RoutingKey string
	ReplyCode  uint16
	ReplyText  string
}

func (e *PublishReturnedError) Error() string {
	return fmt.Sprintf("rabbitmq publish returned for %s/%s: %d %s", e.Exchange, e.RoutingKey, e.ReplyCode, e.ReplyText)
}

func (e *PublishReturnedError) Unwrap() error {
	return ErrPublishReturned
}

// PublishNackedError reports a negative publisher confirm.
type PublishNackedError struct {
	Exchange   string
	RoutingKey string
}

func (e *PublishNackedError) Error() string {
	return fmt.Sprintf("rabbitmq publish nack for %s/%s", e.Exchange, e.RoutingKey)
}

func (e *PublishNackedError) Unwrap() error {
	return ErrPublishNacked
}

func validatePublishRequest(r PublishRequest) error {
	switch {
	case r.Exchange == "":
		return errors.New("rabbitmq publish: exchange is required")
	case r.RoutingKey == "":
		return errors.New("rabbitmq publish: routing key is required")
	case r.Payload == nil:
		return errors.New("rabbitmq publish: payload is required")
	default:
		return nil
	}
}

func marshalPublishPayload(r PublishRequest) ([]byte, error) {
	body, err := json.Marshal(r.Payload)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq publish json: %w", err)
	}
	return body, nil
}

func toAMQPPublishing(r PublishRequest, body []byte) amqp.Publishing {
	deliveryMode := r.DeliveryMode
	if deliveryMode == 0 {
		deliveryMode = amqp.Persistent
	}

	return amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: deliveryMode,
		Body:         body,
		Timestamp:    time.Now().UTC(),
		MessageId:    r.MessageID,
	}
}

func drainReturnedMessage(
	returns <-chan amqp.Return,
	returned *amqp.Return,
) (<-chan amqp.Return, *amqp.Return) {
	if returned != nil || returns == nil {
		return returns, returned
	}

	select {
	case ret, ok := <-returns:
		if !ok {
			return nil, returned
		}
		return returns, &ret
	default:
		return returns, returned
	}
}

func publishConfirmResult(
	exchange string,
	routingKey string,
	confirm amqp.Confirmation,
	ok bool,
	returned *amqp.Return,
) error {
	if returned != nil {
		return &PublishReturnedError{
			Exchange:   exchange,
			RoutingKey: routingKey,
			ReplyCode:  returned.ReplyCode,
			ReplyText:  returned.ReplyText,
		}
	}
	if !ok {
		return fmt.Errorf("rabbitmq publish: confirm channel closed")
	}
	if !confirm.Ack {
		return &PublishNackedError{Exchange: exchange, RoutingKey: routingKey}
	}
	return nil
}

func waitForPublishResult(
	ctx context.Context,
	exchange string,
	routingKey string,
	confirms <-chan amqp.Confirmation,
	returns <-chan amqp.Return,
) error {
	var returned *amqp.Return

	for {
		select {
		case ret, ok := <-returns:
			if !ok {
				// Disable the select branch once RabbitMQ closes the channel.
				returns = nil
				continue
			}
			returned = &ret
		case confirm, ok := <-confirms:
			// race-protection path, returns and confirms are ready at the same time, final darin before "trust this confirm"
			returns, returned = drainReturnedMessage(returns, returned)
			return publishConfirmResult(exchange, routingKey, confirm, ok, returned)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Publish performs the broker-side publish on this manager's shared connection.
//
// It opens a fresh channel for the publish, enables publisher confirms and then
// waits for either:
//   - a positive confirm, which means the publish succeeded;
//   - a returned message, which means the publish was unroutable;
//   - a negative confirm, which means the broker did not accept the publish;
//   - a context cancellation or connection/channel failure.
func (m *RabbitMQManager) Publish(ctx context.Context, req PublishRequest) error {
	if err := validatePublishRequest(req); err != nil {
		return err
	}
	body, err := marshalPublishPayload(req)
	if err != nil {
		return err
	}

	conn, err := m.connection.Connect(ctx, 3)
	if err != nil {
		return fmt.Errorf("rabbitmq publish: connect: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq publish: open channel: %w", err)
	}
	defer ch.Close()
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("rabbitmq publish: enable confirms: %w", err)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	returns := ch.NotifyReturn(make(chan amqp.Return, 1))

	if err := ch.PublishWithContext(ctx, req.Exchange, req.RoutingKey, !req.UnroutableOK, false, toAMQPPublishing(req, body)); err != nil {
		return fmt.Errorf("rabbitmq publish: send %s/%s: %w", req.Exchange, req.RoutingKey, err)
	}

	return waitForPublishResult(ctx, req.Exchange, req.RoutingKey, confirms, returns)
}
