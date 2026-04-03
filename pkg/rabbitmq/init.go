package rabbitmq

import (
	"context"

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
	Publish(ctx context.Context, req PublishRequest) error
}

func Init(cfg config.RabbitMQ) (Service, error) {
	if !cfg.Enabled || cfg.Nodes == nil {
		return new(NoopService), nil
	}

	return NewService(cfg)
}

var globalService Service

// SetService registers the global RabbitMQ service, making it available
// to packages that cannot receive it via dependency injection (e.g. model
// layer workers). Call this once during stack startup.
func SetService(s Service) {
	globalService = s
}

// GetService returns the global RabbitMQ service. It returns a NoopService
// when SetService has not been called, so callers can safely attempt to
// publish and fall back on ErrNotConfigured.
func GetService() Service {
	if globalService == nil {
		return new(NoopService)
	}
	return globalService
}
