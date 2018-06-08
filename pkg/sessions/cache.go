package sessions

import (
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/cache"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

const sessionCacheKey = "session"

type sessionsCache struct {
	base cache.Cache
}

func (ic *sessionsCache) Get(db prefixer.Prefixer, id string) *Session {
	var s Session
	if ok := ic.base.Get(db.DBPrefix()+id, &s); ok {
		return &s
	}
	return nil
}
func (ic *sessionsCache) Set(db prefixer.Prefixer, id string, s *Session) {
	ic.base.Set(db.DBPrefix()+id, s)
}
func (ic *sessionsCache) Revoke(db prefixer.Prefixer, id string) {
	ic.base.Del(db.DBPrefix() + id)
}

var mu sync.Mutex
var globalCache *sessionsCache

func getCache() *sessionsCache {
	mu.Lock()
	defer mu.Unlock()

	if globalCache == nil {
		globalCache = &sessionsCache{
			base: cache.Create(sessionCacheKey, 5*time.Minute),
		}
	}

	return globalCache
}
