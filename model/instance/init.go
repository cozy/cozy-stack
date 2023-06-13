package instance

import (
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
)

var service *InstanceService

// Service handle all the interactions with the "instance" domain.
//
// This interface has several implementations:
// - [InstanceService] with the business logic
// - [Mock] with a mock implementation
type Service interface {
	Get(domain string) (*Instance, error)
	GetWithoutCache(domain string) (*Instance, error)
	Update(inst *Instance) error
	Delete(inst *Instance) error
	CheckPassphrase(inst *Instance, pass []byte) error
}

func Init() *InstanceService {
	service = NewService(
		config.GetConfig().CacheStorage,
		logger.WithNamespace("instance"),
	)

	return service
}

// Get finds an instance from its domain by using CouchDB or the cache.
//
// Deprecated: Use [InstanceService.Get] instead.
func Get(domain string) (*Instance, error) {
	return service.Get(domain)
}

// GetFromCouch finds an instance in CouchDB from its domain.
//
// Deprecated: Use [InstanceService.GetWithoutCache] instead.
func GetFromCouch(domain string) (*Instance, error) {
	return service.GetWithoutCache(domain)
}

// Update saves the changes in CouchDB.
//
// Deprecated: Use [InstanceService.Update] instead.
func Update(inst *Instance) error {
	return service.Update(inst)
}

// Delete removes the instance document in CouchDB.
//
// Deprecated: Use [InstanceService.Delete] instead.
func Delete(inst *Instance) error {
	return service.Delete(inst)
}

// CheckPassphrase confirm an instance password
//
// Deprecated: Use [InstanceService.CheckPassphrase] instead.
func CheckPassphrase(inst *Instance, pass []byte) error {
	return service.CheckPassphrase(inst, pass)
}
