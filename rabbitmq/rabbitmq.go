package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/sync/errgroup"
)

var log = logger.WithNamespace("rabbitmq")

// Handler processes messages for a queue. Return nil to ack, or an error to requeue.
type Handler interface {
	Handle(ctx context.Context, d amqp.Delivery) error
}

type QueueSpec struct {
	cfg     *config.RabbitQueue
	Handler Handler // business logic
	dlxName string  // resolved DLX (queue override or exchange default)
	dlqName string  // resolved DLQ (queue override or exchange default)
}

type ExchangeSpec struct {
	cfg    *config.RabbitExchange
	Queues []QueueSpec
}

type RabbitMQManager struct {
	connection *RabbitMQConnection
	exchanges  []ExchangeSpec
	cancel     context.CancelFunc
	done       chan struct{}
	readyCh    chan struct{}
	readyOnce  sync.Once
}

func NewRabbitMQManager(url string, exchanges []ExchangeSpec) *RabbitMQManager {
	return &RabbitMQManager{
		connection: NewRabbitMQConnection(url),
		exchanges:  exchanges,
		readyCh:    make(chan struct{}),
	}
}

// NewExchangeSpec creates a new ExchangeSpec with the given configuration
func NewExchangeSpec(cfg *config.RabbitExchange) ExchangeSpec {
	return ExchangeSpec{
		cfg:    cfg,
		Queues: []QueueSpec{},
	}
}

// NewQueueSpec creates a new QueueSpec with the given configuration and handler
func NewQueueSpec(cfg *config.RabbitQueue, handler Handler, dlxName, dlqName string) QueueSpec {
	return QueueSpec{
		cfg:     cfg,
		Handler: handler,
		dlxName: dlxName,
		dlqName: dlqName,
	}
}

// Start runs the consumer/manager in background and returns a Shutdowner
func Start(opts config.RabbitMQ) (utils.Shutdowner, error) {
	exchanges := buildExchangeSpecs(opts)
	mgr := NewRabbitMQManager(opts.URL, exchanges)
	// Build TLS config if provided
	tlsCfg, err := buildRabbitTLS(opts.TLS)
	if err != nil {
		return nil, err
	}
	mgr.connection.TLSConfig = tlsCfg
	return mgr.Start(context.Background())
}

// Start runs the manager in background and returns a Shutdowner for graceful stop.
func (m *RabbitMQManager) Start(ctx context.Context) (utils.Shutdowner, error) {
	if m.readyCh == nil {
		m.readyCh = make(chan struct{})
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = m.run(ctx)
	}()
	m.cancel = cancel
	m.done = done
	return m, nil
}

// WaitReady blocks until the manager has declared exchanges/queues and started consumers.
func (m *RabbitMQManager) WaitReady(ctx context.Context) error {
	select {
	case <-m.readyCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *RabbitMQManager) Shutdown(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.done != nil {
		select {
		case <-m.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return m.connection.Close()
}

// run starts the manager loop with automatic reconnection.
func (m *RabbitMQManager) run(ctx context.Context) error {
	log.Infof("Starting RabbitMQ manager with %d exchanges", len(m.exchanges))

	// Main manager loop with automatic reconnection
	for {
		select {
		case <-ctx.Done():
			log.Info("Context canceled, shutting down manager")
			return m.connection.Close()
		default:
		}

		// Connect to RabbitMQ with retry
		conn, err := m.connection.Connect(ctx, 5)
		if err != nil {
			log.Errorf("Failed to connect to RabbitMQ after retries: %v", err)
			continue
		}

		// Declare exchanges once per cycle
		for _, spec := range m.exchanges {
			if err := declareRabbitMQExchange(conn, spec); err != nil {
				log.Errorf("Failed to declare exchange %s: %v", spec.cfg.Name, err)
				_ = m.connection.Close()
				continue
			}
		}

		g, gctx := errgroup.WithContext(ctx)
		// Start all queue runners across all exchanges
		var queueRunners []*queueRunner
		for _, spec := range m.exchanges {
			for _, q := range spec.Queues {
				r, err := newQueueRunner(conn, spec.cfg.Name, q)
				if err != nil {
					log.Errorf("Failed to start queue runner for %s on %s: %v", q.cfg.Name, spec.cfg.Name, err)
					continue
				}
				queueRunners = append(queueRunners, r)
			}
		}
		for _, r := range queueRunners {
			r := r
			g.Go(func() error { return r.run(gctx) })
		}
		// Signal readiness once topology is set and consumers are started
		m.readyOnce.Do(func() { close(m.readyCh) })
		// Monitor connection in the same errgroup; exit on context cancel too
		g.Go(func() error {
			select {
			case <-gctx.Done():
				return gctx.Err()
			case err := <-m.connection.MonitorConnection():
				if err != nil {
					return err
				}
				return context.Canceled
			}
		})

		if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
			log.Errorf("Manager cycle error: %v", err)
		}

		// Close all queue runners and connection before next reconnect attempt
		for _, r := range queueRunners {
			r.close()
		}
		_ = m.connection.Close()
	}
}

func buildExchangeSpecs(opts config.RabbitMQ) []ExchangeSpec {
	var exchanges []ExchangeSpec

	for _, configExchange := range opts.Exchanges {
		var queues []QueueSpec

		if configExchange.Name == "" || configExchange.Kind == "" {
			log.Warnf("Skipping invalid exchange config (missing name/kind)")
			continue
		}

		for _, configQueue := range configExchange.Queues {
			if configQueue.Name == "" {
				log.Warnf("Skipping invalid queue config on exchange %s: missing name", configExchange.Name)
				continue
			}
			var handler Handler

			// Map queue names to appropriate handlers
			switch configQueue.Name {
			case "user.password.updated":
				handler = NewPasswordChangeHandler()
			case "user-settings-updates":
				handler = NewUserSettingsUpdateHandler()
			}

			if handler == nil {
				log.Warnf("Skipping queue without handler on exchange %s: %s", configExchange.Name, configQueue.Name)
				continue
			}

			// Resolve DLX/DLQ with queue-level override first, else exchange default
			resolvedDLX := configQueue.DLXName
			if resolvedDLX == "" {
				resolvedDLX = configExchange.DLXName
			}
			resolvedDLQ := configQueue.DLQName
			if resolvedDLQ == "" {
				resolvedDLQ = configExchange.DLQName
			}

			queues = append(queues, QueueSpec{
				cfg:     &configQueue,
				Handler: handler,
				dlxName: resolvedDLX,
				dlqName: resolvedDLQ,
			})
		}

		exchanges = append(exchanges, ExchangeSpec{
			cfg:    &configExchange,
			Queues: queues,
		})
	}

	return exchanges
}

func declareRabbitMQExchange(conn *amqp.Connection, spec ExchangeSpec) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel for exchange %s: %w", spec.cfg.Name, err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(spec.cfg.Name, spec.cfg.Kind, spec.cfg.Durable, false, false, false, nil); err != nil {
		return fmt.Errorf("failed to declare exchange %s: %w", spec.cfg.Name, err)
	}
	return nil
}

func buildRabbitTLS(tlsOpt config.RabbitMQTLS) (*tls.Config, error) {
	if tlsOpt.RootCAFile == "" && !tlsOpt.InsecureSkipValidation && tlsOpt.ServerName == "" {
		return nil, nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if tlsOpt.ServerName != "" {
		cfg.ServerName = tlsOpt.ServerName
	}
	if tlsOpt.InsecureSkipValidation {
		cfg.InsecureSkipVerify = true
	}
	if tlsOpt.RootCAFile != "" {
		caCertPEM, err := os.ReadFile(tlsOpt.RootCAFile)
		if err != nil {
			return nil, fmt.Errorf("rabbitmq tls: read root_ca: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(caCertPEM) {
			return nil, fmt.Errorf("rabbitmq tls: failed to append root_ca")
		}
		cfg.RootCAs = roots
	}
	return cfg, nil
}
