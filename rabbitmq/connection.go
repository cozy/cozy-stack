package rabbitmq

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQConnection handles RabbitMQ connection management with automatic reconnection.
type RabbitMQConnection struct {
	url       string
	conn      *amqp.Connection
	connClose chan *amqp.Error
	mu        sync.RWMutex // Protects conn and connClose fields
}

func NewRabbitMQConnection(url string) *RabbitMQConnection {
	return &RabbitMQConnection{
		url: url,
	}
}

func (cm *RabbitMQConnection) Connect(ctx context.Context, maxRetries int) (*amqp.Connection, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.conn != nil && cm.isConnectionAlive() {
		return cm.conn, nil // Already connected and alive
	}

	// Close existing connection if any
	if cm.conn != nil {
		cm.conn.Close()
		cm.conn = nil
		cm.connClose = nil
	}

	// Create new connection
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Apply backoff delay (except for first attempt)
		if attempt > 0 {
			backoff := nextBackoff(attempt - 1)
			time.Sleep(backoff)
		}

		// Try to create connection
		conn, err := amqp.Dial(cm.url)
		if err != nil {
			if attempt == maxRetries-1 {
				return nil, fmt.Errorf("failed to connect after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		// Connection created successfully
		cm.conn = conn
		cm.connClose = make(chan *amqp.Error, 1)
		conn.NotifyClose(cm.connClose)
		return cm.conn, nil
	}

	return nil, fmt.Errorf("exceeded maximum connection attempts")
}

// returns a channel that will receive an error when the connection is closed
func (cm *RabbitMQConnection) MonitorConnection() <-chan *amqp.Error {
	return cm.connClose
}

func (cm *RabbitMQConnection) isConnectionAlive() bool {
	if cm.conn == nil {
		return false
	}

	// Check if connection is closed by trying to create a temporary channel
	// This is a lightweight way to test connection health
	ch, err := cm.conn.Channel()
	if err != nil {
		return false // Connection is dead
	}

	ch.Close()
	return true
}

func (cm *RabbitMQConnection) Close() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.conn != nil {
		if err := cm.conn.Close(); err != nil {
			return fmt.Errorf("failed to close connection: %w", err)
		}
		cm.conn = nil
		cm.connClose = nil
	}
	return nil
}

func nextBackoff(attempt int) time.Duration {
	base := time.Duration(1<<uint(attempt)) * time.Second
	max := 30 * time.Second
	if base > max {
		base = max
	}
	return withJitter(base)
}

// prevent "thundering herd" effect
func withJitter(d time.Duration) time.Duration {
	if d == 0 {
		return 0
	}
	jitter := time.Duration(rand.Int63n(int64(d / 4)))
	return d + jitter
}
