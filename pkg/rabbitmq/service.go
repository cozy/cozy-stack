package rabbitmq

import (
	"context"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/utils"
)

type RabbitMQService struct {
	managers map[string]*RabbitMQManager
}

func NewService(cfg config.RabbitMQ) *RabbitMQService {
	managers := map[string]*RabbitMQManager{}
	for context, node := range cfg.Nodes {
		manager, err := buildManager(node, cfg.Exchanges)
		if err != nil {
			log.Errorf("Error while building RabbitMQ manager: %v", err)
			continue
		}
		if manager == nil {
			log.Warnf("No RabbitMQ manager for context %s", context)
			continue
		}

		managers[context] = manager
	}

	return &RabbitMQService{managers}
}

func buildManager(node config.RabbitMQNode, exchangesCfg []config.RabbitExchange) (*RabbitMQManager, error) {
	connection, err := BuildConnection(node)
	if err != nil {
		return nil, err
	}
	if connection == nil {
		return nil, nil
	}

	exchanges := BuildExchangeSpecs(exchangesCfg)

	return NewRabbitMQManager(connection, exchanges), nil
}

// StartManagers runs the managers in background and returns Shutdowners
func (s *RabbitMQService) StartManagers() ([]utils.Shutdowner, error) {
	log.Info("Starting RabbitMQ managers")

	var shutdowners []utils.Shutdowner

	for c, m := range s.managers {
		log.Infof("Starting RabbitMQ manager for context %s", c)

		shutdowner, err := m.Start(context.Background())
		if err != nil {
			log.Errorf("Failed to start RabbitMQ manager for context %s: %v", c, err)
			return shutdowners, err
		}

		shutdowners = append(shutdowners, shutdowner)
	}

	return shutdowners, nil
}
