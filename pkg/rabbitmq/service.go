package rabbitmq

import (
	"context"
	"fmt"

	"github.com/cozy/cozy-stack/pkg/config/config"
)

type RabbitMQService struct {
	Managers   map[string]*RabbitMQManager
	publishers map[string]*Publisher
}

func NewService(cfg config.RabbitMQ) (*RabbitMQService, error) {
	managers := map[string]*RabbitMQManager{}
	publishers := map[string]*Publisher{}

	for context, node := range cfg.Nodes {
		if !node.Enabled {
			log.Infof("No RabbitMQ manager for context %s", context)
			continue
		}

		manager, err := buildManager(node, cfg.Exchanges)
		if err != nil {
			log.Errorf("Error while building RabbitMQ manager: %v", err)
			return nil, err
		}
		managers[context] = manager

		// Build a dedicated publisher connection, separate from the consumer.
		pubConn, err := BuildConnection(node)
		if err != nil {
			log.Errorf("Error while building RabbitMQ publisher: %v", err)
			return nil, err
		}
		publishers[context] = NewPublisher(pubConn)
	}

	return &RabbitMQService{Managers: managers, publishers: publishers}, nil
}

func buildManager(node config.RabbitMQNode, exchangesCfg []config.RabbitExchange) (*RabbitMQManager, error) {
	connection, err := BuildConnection(node)
	if err != nil {
		return nil, err
	}

	exchanges := BuildExchangeSpecs(exchangesCfg)

	return NewRabbitMQManager(connection, exchanges), nil
}

// Publish sends a persistent JSON message to the given exchange and routing key.
// It uses a dedicated publisher connection (not the consumer manager's connection)
// for the given context, falling back to "default" if needed.
func (s *RabbitMQService) Publish(ctx context.Context, contextName, exchange, routingKey string, body []byte) error {
	pub, ok := s.publishers[contextName]
	if !ok {
		pub, ok = s.publishers["default"]
		if !ok {
			return fmt.Errorf("no rabbitmq publisher for context %q", contextName)
		}
		log.Warnf("No publisher for context %q, falling back to default", contextName)
	}

	return pub.Publish(ctx, exchange, routingKey, body)
}

// ClosePublishers closes all publisher connections. It should be called during
// graceful shutdown to avoid leaking connections.
func (s *RabbitMQService) ClosePublishers(ctx context.Context) error {
	for name, pub := range s.publishers {
		if err := pub.Close(); err != nil {
			log.Errorf("Failed to close publisher for context %s: %v", name, err)
		}
	}
	return nil
}

// StartManagers runs the managers in background and returns Shutdowners
func (s *RabbitMQService) StartManagers() (managers []*RabbitMQManager, err error) {
	log.Info("Starting RabbitMQ managers")

	for c, m := range s.Managers {
		log.Infof("Starting RabbitMQ manager for context %s", c)

		started, err := m.Start(context.Background())
		if err == nil {
			managers = append(managers, started)
		} else {
			log.Errorf("Failed to start RabbitMQ manager for context %s: %v", c, err)
			break
		}
	}

	return
}
