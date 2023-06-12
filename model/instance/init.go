package instance

import "github.com/cozy/cozy-stack/pkg/config/config"

var service *InstanceService

func Init() *InstanceService {
	service = NewService(
		config.GetConfig().CacheStorage,
		config.Lock(),
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
// Deprecated: Use [InstanceService.GetFromCouch] instead.
func GetFromCouch(domain string) (*Instance, error) {
	return service.GetFromCouch(domain)
}

// Update saves the changes in CouchDB.
//
// Deprecated: Use [InstanceService.Update] instead.
func Update(inst *Instance) error {
	return service.Update(inst)
}
