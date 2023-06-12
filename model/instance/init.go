package instance

import (
	"context"

	"github.com/cozy/cozy-stack/pkg/config/config"
)

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
// It also check if the valid indexes/views are used. If a change have been
// detected it will update all the indexes/views before returning the instance.
//
// Deprecated: Uses [instance.Instance] instead.
func Get(domain string) (*Instance, error) {
	return service.Get(context.Background(), domain)
}
