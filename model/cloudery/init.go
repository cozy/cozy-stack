package cloudery

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
)

var service Service

// Service handle all the interactions with the cloudery
//
// Several implementations exists:
// - [ClouderyService] interacts via HTTP
// - [NoopService] when no config is setup
// - [Mock] for the tests
type Service interface {
	SaveInstance(inst *instance.Instance, cmd *SaveCmd) error
	HasBlockingSubscription(inst *instance.Instance) (bool, error)
}

func Init(contexts map[string]config.ClouderyConfig) Service {
	if contexts == nil {
		service = new(NoopService)
		return service
	}

	service = NewService(contexts)
	return service
}

// SaveInstance data into the cloudery matching the instance context.
//
// Deprecated: Use [ClouderyService.SaveInstance] instead.
func SaveInstance(inst *instance.Instance, cmd *SaveCmd) error {
	return service.SaveInstance(inst, cmd)
}
