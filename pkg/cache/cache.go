package cache

import (
	"context"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
)

var service Cache

// Cache is a rudimentary key/value caching store backed by redis. It offers a
// Get/Set interface as well a its gzip compressed alternative
// GetCompressed/SetCompressed
type Cache interface {
	CheckStatus(ctx context.Context) (time.Duration, error)
	Get(key string) ([]byte, bool)
	MultiGet(keys []string) [][]byte
	Keys(prefix string) []string
	Clear(key string)
	Set(key string, data []byte, expiration time.Duration)
	SetNX(key string, data []byte, expiration time.Duration)
	GetCompressed(key string) (io.Reader, bool)
	SetCompressed(key string, data []byte, expiration time.Duration)
	RefreshTTL(key string, expiration time.Duration)
}

type cacheEntry struct {
	payload   []byte
	expiredAt time.Time
}

// Init instantiates a Cache Client and setup the global functions.
//
// The backend selection is done based on the `client` argument. If a client is
// given, the redis backend is chosen, if nil is provided the inmemory backend would
// be chosen.
func Init(client redis.UniversalClient) Cache {
	if client == nil {
		return NewInMemory()
	}

	return NewRedis(client)
}

// CheckStatus checks that the cache is ready, or returns an error.
//
// Deprecated: Use [Cache.CheckStatus] instead.
func CheckStatus(ctx context.Context) (time.Duration, error) {
	return service.CheckStatus(ctx)
}

// Get fetch the cached asset at the given key, and returns true only if the
// asset was found.
//
// Deprecated: Use [Cache.Get] instead.
func Get(key string) ([]byte, bool) {
	return service.Get(key)
}

// MultiGet can be used to fetch several keys at once.
//
// Deprecated: Use [Cache.MultiGet] instead.
func MultiGet(keys []string) [][]byte {
	return service.MultiGet(keys)
}

// Keys returns the list of keys with the given prefix.
//
// Note: it can be slow and should be used carefully.
//
// Deprecated: Use [Cache.Prefix] instead.
func Keys(prefix string) []string {
	return service.Keys(prefix)
}

// Clear removes a key from the cache
//
// Deprecated: Use [Cache.Clear] instead.
func Clear(key string) {
	service.Clear(key)
}

// Set stores an asset to the given key.
//
// Deprecated: Use [Cache.Set] instead.
func Set(key string, data []byte, expiration time.Duration) {
	service.Set(key, data, expiration)
}

// SetNX stores the data in the cache only if the key doesn't exist yet.
//
// Deprecated: Use [Cache.SetNX] instead.
func SetNX(key string, data []byte, expiration time.Duration) {
	service.SetNX(key, data, expiration)
}

// GetCompressed works like Get but expect a compressed asset that is
// uncompressed.
//
// Deprecated: Use [Cache.GetCompressed] instead.
func GetCompressed(key string) (io.Reader, bool) {
	return service.GetCompressed(key)
}

// SetCompressed works like Set but compress the asset data before storing it.
//
// Deprecated: Use [Cache.SetCompressed] instead.
func SetCompressed(key string, data []byte, expiration time.Duration) {
	service.SetCompressed(key, data, expiration)
}

// RefreshTTL can be used to update the TTL of an existing entry in the cache.
//
// Deprecated: Use [Cache.RefreshTTL] instead.
func RefreshTTL(key string, expiration time.Duration) {
	service.RefreshTTL(key, expiration)
}
