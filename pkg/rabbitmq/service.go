package rabbitmq

import (
	"context"

	"github.com/cozy/cozy-stack/pkg/config/config"
)

type RabbitMQService struct {
	Managers map[string]*RabbitMQManager
}

func NewService(cfg config.RabbitMQ) (*RabbitMQService, error) {
	managers := map[string]*RabbitMQManager{}
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
	}

	return &RabbitMQService{managers}, nil
}

func buildManager(node config.RabbitMQNode, exchangesCfg []config.RabbitExchange) (*RabbitMQManager, error) {
	connection, err := BuildConnection(node)
	if err != nil {
		return nil, err
	}

	exchanges := BuildExchangeSpecs(exchangesCfg)

	return NewRabbitMQManager(connection, exchanges), nil
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
