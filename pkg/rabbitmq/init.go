package rabbitmq

import (
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
)

var log = logger.WithNamespace("rabbitmq")

// Service handles all the interactions with RabbitMQ
//
// Several implementations exist:
// - [RabbitMQService] interacts with the RabbitMQ nodes
// - [NoopService] when no config is setup
type Service interface {
	StartManagers() ([]*RabbitMQManager, error)
}

func Init(cfg config.RabbitMQ) (Service, error) {
	if !cfg.Enabled || cfg.Nodes == nil {
		return new(NoopService), nil
	}

	return NewService(cfg)
}
