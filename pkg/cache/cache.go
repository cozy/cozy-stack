package cache

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

type cacheEntry struct {
	payload   []byte
	expiredAt time.Time
}

// Cache is a rudimentary key/value caching store backed by redis. It offers a
// Get/Set interface as well a its gzip compressed alternative
// GetCompressed/SetCompressed
type Cache struct {
	client redis.UniversalClient
	m      *sync.Map
	ctx    context.Context
}

// New returns a new Cache from a potentially nil redis client.
func New(client redis.UniversalClient) Cache {
	ctx := context.Background()
	if client != nil {
		return Cache{client, nil, ctx}
	}
	m := sync.Map{}
	return Cache{nil, &m, ctx}
}

// CheckStatus checks that the cache is ready, or returns an error.
func (c Cache) CheckStatus() (time.Duration, error) {
	if c.client == nil {
		return 0, nil
	}
	before := time.Now()
	if err := c.client.Ping(c.ctx).Err(); err != nil {
		return 0, err
	}
	return time.Since(before), nil
}

// Get fetch the cached asset at the given key, and returns true only if the
// asset was found.
func (c Cache) Get(key string) ([]byte, bool) {
	if c.client == nil {
		if value, ok := c.m.Load(key); ok {
			entry := value.(cacheEntry)
			if time.Now().Before(entry.expiredAt) {
				return entry.payload, true
			}
			c.Clear(key)
		}
	} else {
		cmd := c.client.Get(c.ctx, key)
		if b, err := cmd.Bytes(); err == nil {
			return b, true
		}
	}
	return nil, false
}

// MultiGet can be used to fetch several keys at once.
func (c Cache) MultiGet(keys []string) [][]byte {
	results := make([][]byte, len(keys))
	if c.client == nil {
		for i, key := range keys {
			results[i], _ = c.Get(key)
		}
	} else {
		cmd := c.client.MGet(c.ctx, keys...)
		for i, val := range cmd.Val() {
			if buf, ok := val.(string); ok {
				results[i] = []byte(buf)
			}
		}
	}
	return results
}

// Keys returns the list of keys with the given prefix.
// Note: it can be slow and should be used carefully.
func (c Cache) Keys(prefix string) []string {
	if c.client != nil {
		cmd := c.client.Keys(c.ctx, prefix+"*")
		return cmd.Val()
	}
	results := make([]string, 0)
	c.m.Range(func(key, _ interface{}) bool {
		k := key.(string)
		if strings.HasPrefix(k, prefix) {
			results = append(results, k)
		}
		return true
	})
	return results
}

// Clear removes a key from the cache
func (c Cache) Clear(key string) {
	if c.client == nil {
		c.m.Delete(key)
	} else {
		c.client.Del(c.ctx, key)
	}
}

// Set stores an asset to the given key.
func (c Cache) Set(key string, data []byte, expiration time.Duration) {
	if c.client == nil {
		c.m.Store(key, cacheEntry{
			payload:   data,
			expiredAt: time.Now().Add(expiration),
		})
	} else {
		c.client.Set(c.ctx, key, data, expiration)
	}
}

// SetNX stores the data in the cache only if the key doesn't exist yet.
func (c Cache) SetNX(key string, data []byte, expiration time.Duration) {
	if c.client == nil {
		c.m.LoadOrStore(key, cacheEntry{
			payload:   data,
			expiredAt: time.Now().Add(expiration),
		})
	} else {
		c.client.SetNX(c.ctx, key, data, expiration)
	}
}

// GetCompressed works like Get but expect a compressed asset that is
// uncompressed.
func (c Cache) GetCompressed(key string) (io.Reader, bool) {
	if r, ok := c.Get(key); ok {
		if gr, err := gzip.NewReader(bytes.NewReader(r)); err == nil {
			return gr, true
		}
	}
	return nil, false
}

// SetCompressed works like Set but compress the asset data before storing it.
func (c Cache) SetCompressed(key string, data []byte, expiration time.Duration) {
	dataCompressed := new(bytes.Buffer)
	gw := gzip.NewWriter(dataCompressed)
	defer gw.Close()
	if _, err := io.Copy(gw, bytes.NewReader(data)); err != nil {
		return
	}
	c.Set(key, dataCompressed.Bytes(), expiration)
}

// RefreshTTL can be used to update the TTL of an existing entry in the cache.
func (c Cache) RefreshTTL(key string, expiration time.Duration) {
	if c.client == nil {
		if value, ok := c.m.Load(key); ok {
			entry := value.(cacheEntry)
			entry.expiredAt = time.Now().Add(expiration)
			c.m.Store(key, entry)
		}
	} else {
		c.client.Expire(c.ctx, key, expiration)
	}
}
