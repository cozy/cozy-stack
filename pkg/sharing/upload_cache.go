package sharing

import (
	"encoding/hex"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/cache"
	"github.com/cozy/cozy-stack/pkg/crypto"
)

const uploadCacheKey = "sharing-upload"

type uploadsCache struct {
	base cache.Cache
}

func (ic *uploadsCache) Get(domain, key string) *FileDocWithRevisions {
	var f FileDocWithRevisions
	if ok := ic.base.Get(domain+key, &f); ok {
		return &f
	}
	return nil
}

func (ic *uploadsCache) Save(domain string, doc *FileDocWithRevisions) string {
	key := hex.EncodeToString(crypto.GenerateRandomBytes(8))
	ic.base.Set(domain+key, doc)
	return key
}

var mu sync.Mutex
var globalCache *uploadsCache

func getCache() *uploadsCache {
	mu.Lock()
	defer mu.Unlock()
	if globalCache == nil {
		globalCache = &uploadsCache{
			base: cache.Create(uploadCacheKey, 5*time.Minute),
		}
	}
	return globalCache
}
