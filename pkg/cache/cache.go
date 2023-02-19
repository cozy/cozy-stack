package cache

import (
	"context"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
)

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

// New instantiate a Cache Client.
//
// The backend selection is done based on the `client` argument. If a client is
// given, the redis backend is chosen, if nil is provided the inmemory backend would
// be chosen.
func New(client redis.UniversalClient) Cache {
	if client == nil {
		return NewInMemory()
	}

	return NewRedis(client)
}
