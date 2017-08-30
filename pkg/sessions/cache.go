package sessions

import (
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/cache"
)

type sessionsCache struct {
	base cache.Cache
}

func (ic *sessionsCache) Get(domain, id string) *Session {
	var s Session
	if ok := ic.base.Get(domain+id, &s); ok {
		return &s
	}
	return nil
}
func (ic *sessionsCache) Set(domain, id string, s *Session) {
	ic.base.Set(domain+id, s)
}
func (ic *sessionsCache) Revoke(domain, id string) {
	ic.base.Del(domain + id)
}

var mu sync.Mutex
var globalCache *sessionsCache

func getCache() *sessionsCache {
	mu.Lock()
	defer mu.Unlock()

	if globalCache == nil {
		globalCache = &sessionsCache{
			base: cache.Create(SessionContextKey, 5*time.Minute),
		}
	}

	return globalCache
}
