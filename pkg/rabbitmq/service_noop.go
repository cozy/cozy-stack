package rabbitmq

import (
	"github.com/cozy/cozy-stack/pkg/utils"
)

// NoopService implements [Service].
//
// This implem does nothing. It is used when no config is provided.
type NoopService struct{}

// StartManagers does nothing.
func (s *NoopService) StartManagers() ([]utils.Shutdowner, error) {
	log.Warnf("No RabbitMQ managers to start")
	return nil, nil
}
