package rabbitmq

import (
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
)

var log = logger.WithNamespace("rabbitmq")

// Service handles all the interactions with RabbitMQ
//
// Several implementations exist:
// - [RabbitMQService] interacts with the RabbitMQ nodes
// - [NoopService] when no config is setup
// - [Mock] for the tests
type Service interface {
	StartManagers() ([]utils.Shutdowner, error)
}

func Init(cfg config.RabbitMQ) Service {
	if !cfg.Enabled || cfg.Nodes == nil {
		return new(NoopService)
	}

	return NewService(cfg)
}
