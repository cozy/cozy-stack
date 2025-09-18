package rabbitmq

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	DefaultPrefetch     = 32
	QuorumType          = "quorum"
	StrategyAtLeastOnce = "at-least-once"
	StrategyOverflow    = "reject-publish"
)

// runs a single queue with its own channel/consumer
type queueRunner struct {
	exchangeName string
	queue        QueueSpec
	ch           *amqp.Channel
	chClose      chan *amqp.Error
	consumerTag  string
}

func newQueueRunner(conn *amqp.Connection, exchangeName string, q QueueSpec) (*queueRunner, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open channel for queue %s: %w", q.cfg.Name, err)
	}

	r := &queueRunner{
		exchangeName: exchangeName,
		queue:        q,
		ch:           ch,
		chClose:      ch.NotifyClose(make(chan *amqp.Error, 1)),
		consumerTag:  fmt.Sprintf("cozy-%s-%s-%d", exchangeName, q.cfg.Name, time.Now().UnixNano()),
	}

	// Declare DLX/DLQ using resolved fields already set on QueueSpec
	if q.dlxName != "" {
		if err := r.ch.ExchangeDeclare(q.dlxName, "fanout", true, false, false, false, nil); err != nil {
			_ = ch.Close()
			return nil, fmt.Errorf("failed to declare DLX %s: %w", q.dlxName, err)
		}
	}
	if q.dlqName != "" {
		if _, err := r.ch.QueueDeclare(q.dlqName, true, false, false, false, nil); err != nil {
			_ = ch.Close()
			return nil, fmt.Errorf("failed to declare DLQ %s: %w", q.dlqName, err)
		}
		if q.dlxName != "" {
			if err := r.ch.QueueBind(q.dlqName, "", q.dlxName, false, nil); err != nil {
				_ = ch.Close()
				return nil, fmt.Errorf("failed to bind DLQ %s to DLX %s: %w", q.dlqName, q.dlxName, err)
			}
		}
	}

	// Declare queue with args and bind to exchange
	qArgs := amqp.Table{
		"x-queue-type":           QuorumType,
		"x-dead-letter-exchange": q.dlxName,
		"x-delivery-limit":       q.cfg.DeliveryLimit,
		"x-dead-letter-strategy": StrategyAtLeastOnce,
		"x-overflow":             StrategyOverflow,
	}

	if q.cfg.Declare {
		if _, err := r.ch.QueueDeclare(q.cfg.Name, true, false, false, false, qArgs); err != nil {
			_ = ch.Close()
			return nil, fmt.Errorf("failed to declare queue %s: %w", q.cfg.Name, err)
		}
	}
	for _, binding := range q.cfg.Bindings {
		if err := r.ch.QueueBind(q.cfg.Name, binding, exchangeName, false, nil); err != nil {
			_ = ch.Close()
			return nil, fmt.Errorf("failed to bind queue %s with routing key %s: %w", q.cfg.Name, binding, err)
		}
	}

	// QoS
	prefetch := q.cfg.Prefetch
	if prefetch == 0 {
		prefetch = DefaultPrefetch
	}
	if err := r.ch.Qos(prefetch, 0, false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("failed to set QoS for queue %s: %w", q.cfg.Name, err)
	}

	return r, nil
}

func (r *queueRunner) run(ctx context.Context) error {
	log.Infof("Starting consumer [exchange=%s queue=%s tag=%s]", r.exchangeName, r.queue.cfg.Name, r.consumerTag)

	deliveries, err := r.ch.Consume(r.queue.cfg.Name, r.consumerTag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to start consumer [exchange=%s queue=%s]: %w", r.exchangeName, r.queue.cfg.Name, err)
	}

	for {
		select {
		case <-ctx.Done():
			return r.ch.Close()
		case err := <-r.chClose:
			if err != nil {
				return fmt.Errorf("channel closed [exchange=%s queue=%s]: %w", r.exchangeName, r.queue.cfg.Name, err)
			}
			return fmt.Errorf("channel closed [exchange=%s queue=%s]", r.exchangeName, r.queue.cfg.Name)
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed [exchange=%s queue=%s]", r.exchangeName, r.queue.cfg.Name)
			}
			var handleErr error
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						handleErr = fmt.Errorf("panic in handler: %v", rec)
					}
				}()
				handleErr = r.queue.Handler.Handle(ctx, d)
			}()
			if handleErr != nil {
				_ = d.Nack(false, true)
				continue
			}
			if ackErr := d.Ack(false); ackErr != nil {
				log.Errorf("Failed to ack message [exchange=%s queue=%s]: %v", r.exchangeName, r.queue.cfg.Name, ackErr)
			} else {
				// acked
			}
		}
	}
}

func (r *queueRunner) close() {
	if r.ch != nil {
		_ = r.ch.Close()
	}
}
