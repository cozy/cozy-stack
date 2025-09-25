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
	log.Debugf("Creating queue runner for queue: %s on exchange: %s", q.cfg.Name, exchangeName)

	ch, err := conn.Channel()
	if err != nil {
		log.Errorf("Failed to open channel for queue %s: %v", q.cfg.Name, err)
		return nil, fmt.Errorf("failed to open channel for queue %s: %w", q.cfg.Name, err)
	}
	log.Debugf("Opened channel for queue: %s", q.cfg.Name)

	r := &queueRunner{
		exchangeName: exchangeName,
		queue:        q,
		ch:           ch,
		chClose:      ch.NotifyClose(make(chan *amqp.Error, 1)),
		consumerTag:  fmt.Sprintf("cozy-%s-%s-%d", exchangeName, q.cfg.Name, time.Now().UnixNano()),
	}
	log.Debugf("Created queue runner with consumer tag: %s", r.consumerTag)

	// Declare DLX/DLQ using resolved fields already set on QueueSpec
	if q.cfg.DeclareDLX && q.dlxName != "" {
		log.Debugf("Declaring DLX: %s for queue: %s", q.dlxName, q.cfg.Name)
		if err := r.ch.ExchangeDeclare(q.dlxName, "fanout", true, false, false, false, nil); err != nil {
			_ = ch.Close()
			log.Errorf("Failed to declare DLX %s for queue %s: %v", q.dlxName, q.cfg.Name, err)
			return nil, fmt.Errorf("failed to declare DLX %s: %w", q.dlxName, err)
		}
		log.Debugf("Successfully declared DLX: %s", q.dlxName)
	}

	if q.cfg.DeclareDLQ && q.dlqName != "" {
		log.Debugf("Declaring DLQ: %s for queue: %s", q.dlqName, q.cfg.Name)
		if _, err := r.ch.QueueDeclare(q.dlqName, true, false, false, false, nil); err != nil {
			_ = ch.Close()
			log.Errorf("Failed to declare DLQ %s for queue %s: %v", q.dlqName, q.cfg.Name, err)
			return nil, fmt.Errorf("failed to declare DLQ %s: %w", q.dlqName, err)
		}
		log.Debugf("Successfully declared DLQ: %s", q.dlqName)
		if q.dlxName != "" {
			log.Debugf("Binding DLQ %s to DLX %s", q.dlqName, q.dlxName)
			if err := r.ch.QueueBind(q.dlqName, "", q.dlxName, false, nil); err != nil {
				_ = ch.Close()
				log.Errorf("Failed to bind DLQ %s to DLX %s: %v", q.dlqName, q.dlxName, err)
				return nil, fmt.Errorf("failed to bind DLQ %s to DLX %s: %w", q.dlqName, q.dlxName, err)
			}
			log.Debugf("Successfully bound DLQ %s to DLX %s", q.dlqName, q.dlxName)
		}
	}

	// Declare queue with args and bind to exchange
	log.Debugf("Declaring queue: %s with DLX: %s, DLQ: %s, delivery limit: %d", q.cfg.Name, q.dlxName, q.dlqName, q.cfg.DeliveryLimit)
	qArgs := amqp.Table{
		"x-queue-type":             QuorumType,
		"x-dead-letter-exchange":   q.dlxName,
		"x-delivery-limit":         q.cfg.DeliveryLimit,
		"x-dead-letter-strategy":   StrategyAtLeastOnce,
		"x-overflow":               StrategyOverflow,
		"x-single-active-consumer": true, // Enable Single Active Consumer for failover
	}

	if q.cfg.Declare {
		if _, err := r.ch.QueueDeclare(q.cfg.Name, true, false, false, false, qArgs); err != nil {
			_ = ch.Close()
			log.Errorf("Failed to declare queue %s: %v", q.cfg.Name, err)
			return nil, fmt.Errorf("failed to declare queue %s: %w", q.cfg.Name, err)
		}
		log.Debugf("Successfully declared queue: %s", q.cfg.Name)
	}

	log.Debugf("Binding queue %s to exchange %s with %d routing keys", q.cfg.Name, exchangeName, len(q.cfg.Bindings))
	for _, binding := range q.cfg.Bindings {
		log.Debugf("Binding queue %s with routing key: %s", q.cfg.Name, binding)
		if err := r.ch.QueueBind(q.cfg.Name, binding, exchangeName, false, nil); err != nil {
			_ = ch.Close()
			log.Errorf("Failed to bind queue %s with routing key %s: %v", q.cfg.Name, binding, err)
			return nil, fmt.Errorf("failed to bind queue %s with routing key %s: %w", q.cfg.Name, binding, err)
		}
		log.Debugf("Successfully bound queue %s with routing key: %s", q.cfg.Name, binding)
	}

	// QoS
	prefetch := q.cfg.Prefetch
	if prefetch == 0 {
		prefetch = DefaultPrefetch
	}
	log.Debugf("Setting QoS for queue %s: prefetch=%d", q.cfg.Name, prefetch)
	if err := r.ch.Qos(prefetch, 0, false); err != nil {
		_ = ch.Close()
		log.Errorf("Failed to set QoS for queue %s: %v", q.cfg.Name, err)
		return nil, fmt.Errorf("failed to set QoS for queue %s: %w", q.cfg.Name, err)
	}
	log.Debugf("Successfully set QoS for queue %s", q.cfg.Name)

	log.Debugf("Queue runner setup completed for: %s", q.cfg.Name)
	return r, nil
}

func (r *queueRunner) run(ctx context.Context) error {
	log.Infof("Starting consumer [exchange=%s queue=%s tag=%s]", r.exchangeName, r.queue.cfg.Name, r.consumerTag)

	log.Debugf("Starting message consumption for queue: %s", r.queue.cfg.Name)
	deliveries, err := r.ch.Consume(r.queue.cfg.Name, r.consumerTag, false, false, false, false, nil)
	if err != nil {
		log.Errorf("Failed to start consumer for queue %s: %v", r.queue.cfg.Name, err)
		return fmt.Errorf("failed to start consumer [exchange=%s queue=%s]: %w", r.exchangeName, r.queue.cfg.Name, err)
	}
	log.Infof("Successfully started consumer for queue: %s (consumer tag: %s) - waiting for activation with Single Active Consumer", r.queue.cfg.Name, r.consumerTag)

	messageCount := 0
	for {
		select {
		case <-ctx.Done():
			log.Infof("Consumer context cancelled for queue: %s, closing channel", r.queue.cfg.Name)
			return r.ch.Close()
		case err := <-r.chClose:
			if err != nil {
				log.Errorf("Channel closed with error for queue %s: %v", r.queue.cfg.Name, err)
				return fmt.Errorf("channel closed [exchange=%s queue=%s]: %w", r.exchangeName, r.queue.cfg.Name, err)
			}
			log.Warnf("Channel closed without error for queue: %s", r.queue.cfg.Name)
			return fmt.Errorf("channel closed [exchange=%s queue=%s]", r.exchangeName, r.queue.cfg.Name)
		case d, ok := <-deliveries:
			if !ok {
				log.Warnf("Delivery channel closed for queue: %s", r.queue.cfg.Name)
				return fmt.Errorf("delivery channel closed [exchange=%s queue=%s]", r.exchangeName, r.queue.cfg.Name)
			}
			messageCount++
			if messageCount == 1 {
				log.Infof("🎯 CONSUMER ACTIVATED: This process is now the active consumer for queue %s (consumer tag: %s)", r.queue.cfg.Name, r.consumerTag)
			}
			log.Debugf("Received message #%d for queue %s (routing key: %s, message ID: %s)", messageCount, r.queue.cfg.Name, d.RoutingKey, d.MessageId)
			var handleErr error
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						handleErr = fmt.Errorf("panic in handler: %v", rec)
						log.Errorf("Panic in message handler for queue %s: %v", r.queue.cfg.Name, rec)
					}
				}()
				log.Debugf("Processing message #%d for queue %s with handler", messageCount, r.queue.cfg.Name)
				handleErr = r.queue.Handler.Handle(ctx, d)
			}()
			if handleErr != nil {
				log.Errorf("Message #%d processing failed for queue %s: %v, nacking message", messageCount, r.queue.cfg.Name, handleErr)
				_ = d.Nack(false, true)
				continue
			}
			log.Debugf("Message #%d processed successfully for queue %s, acknowledging", messageCount, r.queue.cfg.Name)
			if ackErr := d.Ack(false); ackErr != nil {
				log.Errorf("Failed to ack message #%d [exchange=%s queue=%s]: %v", messageCount, r.exchangeName, r.queue.cfg.Name, ackErr)
			} else {
				log.Debugf("Message #%d acknowledged successfully for queue %s", messageCount, r.queue.cfg.Name)
			}
		}
	}
}

func (r *queueRunner) close() {
	if r.ch != nil {
		log.Debugf("Closing channel for queue: %s", r.queue.cfg.Name)
		_ = r.ch.Close()
		log.Debugf("Channel closed for queue: %s", r.queue.cfg.Name)
	}
}
