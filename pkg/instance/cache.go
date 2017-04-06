package instance

import (
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/cache"
)

type instanceCache struct {
	base cache.Cache
}

func (ic *instanceCache) Get(d string) *Instance {
	var out Instance
	if ok := ic.base.Get(d, &out); ok {
		return &out
	}
	return nil
}
func (ic *instanceCache) Set(d string, i *Instance) {
	ic.base.Set(d, i)
}
func (ic *instanceCache) Revoke(d string) {
	ic.base.Del(d)
}

var mu sync.Mutex
var globalCache *instanceCache

func getCache() *instanceCache {
	mu.Lock()
	defer mu.Unlock()

	if globalCache == nil {
		globalCache = &instanceCache{
			base: cache.Create("instance", 5*time.Minute),
		}
	}

	return globalCache
}
