package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestRabbitMQManager(t *testing.T) {
	t.Run("ConsumeAll_NoLoss", func(t *testing.T) {
		t.Parallel()

		// Start broker
		rabbitmq := StartRabbitMQ(t, true, false)
		totalMessages := 10

		// Build manager with one exchange and one queue using PasswordChangeHandler wrapper
		handler := newCountingTestHandler()
		mgr := initRabbitMQManager(rabbitmq.AMQPURL, "ex.password", "password-change-queue", "password.changed", handler)
		_, err := mgr.Start(testCtx(t))
		require.NoError(t, err)

		// Wait for manager to declare topology
		require.NoError(t, mgr.WaitReady(testCtx(t)))

		sender := newTestMessageSender(t, mgr, mgr.exchanges[0].cfg.Name, mgr.exchanges[0].Queues[0].cfg.Bindings[0])
		// Publish all messages
		for i := 0; i < totalMessages; i++ {
			msg := PasswordChangeMessage{Domain: fmt.Sprintf("example-%d", i), NewPassword: "secret", Version: 1}
			sender.publish(msg)
		}

		// Await full consumption
		deadline := time.Now().Add(10 * time.Second)
		for {
			if handler.total >= totalMessages {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %d messages, got %d", totalMessages, handler.total)
			}
			time.Sleep(100 * time.Millisecond)
		}
		require.NoError(t, mgr.Shutdown(testCtx(t)))
	})

	t.Run("ConsumeAll_WithReconnection", func(t *testing.T) {
		t.Parallel()

		// Start broker
		rabbitmq := StartRabbitMQ(t, true, false)
		totalMessages := 20

		// Build manager with one exchange and one queue using PasswordChangeHandler wrapper
		handler := newCountingTestHandler()
		mgr := initRabbitMQManager(rabbitmq.AMQPURL, "ex.password", "password-change-queue", "password.changed", handler)
		_, err := mgr.Start(testCtx(t))
		require.NoError(t, err)

		require.NoError(t, mgr.WaitReady(testCtx(t)))

		sender := newTestMessageSender(t, mgr, mgr.exchanges[0].cfg.Name, mgr.exchanges[0].Queues[0].cfg.Bindings[0])
		// Publish half of the messages
		for i := 0; i < totalMessages/2; i++ {
			msg := PasswordChangeMessage{Domain: fmt.Sprintf("example-%d", i), NewPassword: "secret", Version: 1}
			sender.publish(msg)
		}

		//restart the container
		rabbitmq.Stop(context.Background(), 30*time.Second)
		rabbitmq.Restart(context.Background(), 30*time.Second)

		//publish second half
		sender = newTestMessageSender(t, mgr, mgr.exchanges[0].cfg.Name, mgr.exchanges[0].Queues[0].cfg.Bindings[0])
		// Publish half of the messages
		for i := 0; i < totalMessages/2; i++ {
			msg := PasswordChangeMessage{Domain: fmt.Sprintf("example-%d", i), NewPassword: "secret", Version: 1}
			sender.publish(msg)
		}

		// Await full consumption
		deadline := time.Now().Add(100 * time.Second)
		for {
			if handler.total >= totalMessages {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %d messages, got %d", totalMessages, handler.total)
			}
			time.Sleep(100 * time.Millisecond)
		}
		require.NoError(t, mgr.Shutdown(testCtx(t)))
	})
}

func initRabbitMQManager(amqpURL string, exchangeName string, queueName string, routingKey string, handler *countingTestHandler) *RabbitMQManager {
	mgr := &RabbitMQManager{
		connection: NewRabbitMQConnection(amqpURL),
		exchanges: []ExchangeSpec{
			{
				cfg: &config.RabbitExchange{
					Name:    exchangeName,
					Kind:    "topic",
					Durable: true,
				},
				Queues: []QueueSpec{
					{
						cfg: &config.RabbitQueue{
							Name:          queueName,
							Bindings:      []string{routingKey},
							Prefetch:      8,
							DeliveryLimit: 5,
						},
						Handler: handler,
						dlxName: "",
						dlqName: "",
					},
				},
			},
		},
	}
	return mgr
}

type testMessageSender struct {
	t            *testing.T
	ch           *amqp.Channel
	exchangeName string
	routingKey   string
}

func newTestMessageSender(t *testing.T, mgr *RabbitMQManager, exchangeName string, routingKey string) *testMessageSender {
	pubConn, err := mgr.connection.Connect(context.Background(), 5)
	require.NoError(t, err)
	pubCh, err := pubConn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pubCh.Close() })
	return &testMessageSender{
		t:            t,
		ch:           pubCh,
		exchangeName: exchangeName,
		routingKey:   routingKey,
	}

}

func (m *testMessageSender) publish(msg PasswordChangeMessage) {
	m.t.Helper()
	body, err := json.Marshal(msg)
	require.NoError(m.t, err)
	require.NoError(m.t, m.ch.PublishWithContext(
		context.Background(),
		m.exchangeName,
		m.routingKey,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
		},
	))
}

// countingTestHandler wraps PasswordChangeHandler to count processed messages.
type countingTestHandler struct {
	total int
}

func newCountingTestHandler() *countingTestHandler {
	return &countingTestHandler{
		total: 0,
	}
}

func (h *countingTestHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	log.Infof("Received test message: %s", d.RoutingKey)
	h.total++
	return nil
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	t.Cleanup(cancel)
	return ctx
}
