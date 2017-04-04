package instance

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/cache"
)

type instanceCache struct {
	base cache.Cache
}

func (ic *instanceCache) Get(d string) *Instance {
	var out Instance
	ic.base.Get(d, &out)
	if out.DocID == "" {
		return nil
	}
	return &out
}
func (ic *instanceCache) Set(d string, i *Instance) {
	ic.base.Set(d, i)
}
func (ic *instanceCache) Revoke(d string) {
	ic.base.Del(d)
}

var globalCache *instanceCache

func getCache() *instanceCache {

	if globalCache == nil {
		globalCache = &instanceCache{
			base: cache.Create("instance", 5*time.Minute),
		}
	}

	return globalCache
}
