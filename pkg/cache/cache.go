package cache

import (
	"bytes"
	"compress/gzip"
	"io"
	"time"

	"github.com/go-redis/redis"
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
	m      map[string]cacheEntry
}

// New returns a new Cache from a potentially nil redis client.
func New(client redis.UniversalClient) Cache {
	if client != nil {
		return Cache{client, nil}
	}
	m := make(map[string]cacheEntry)
	return Cache{nil, m}
}

// Get fetch the cached asset at the given key, and returns true only if the
// asset was found.
func (c Cache) Get(key string) (io.Reader, bool) {
	if c.client == nil {
		if entry, ok := c.m[key]; ok {
			if time.Now().Before(entry.expiredAt) {
				return bytes.NewReader(entry.payload), true
			}
			c.Clear(key)
		}
	} else {
		cmd := c.client.Get(key)
		if b, err := cmd.Bytes(); err == nil {
			return bytes.NewReader(b), true
		}
	}
	return nil, false
}

// Clear removes a key from the cache
func (c Cache) Clear(key string) {
	if c.client == nil {
		delete(c.m, key)
	} else {
		c.client.Del(key)
	}
}

// Set stores an asset to the given key.
func (c Cache) Set(key string, data []byte, expiration time.Duration) {
	if c.client == nil {
		c.m[key] = cacheEntry{
			payload:   data,
			expiredAt: time.Now().Add(expiration),
		}
	} else {
		c.client.Set(key, data, expiration)
	}
}

// GetCompressed works like Get but expect a compressed asset that is
// uncompressed.
func (c Cache) GetCompressed(key string) (io.Reader, bool) {
	if r, ok := c.Get(key); ok {
		if gr, err := gzip.NewReader(r); err == nil {
			return gr, true
		}
	}
	return nil, false
}

// SetCompressed works like Set but compress the asset data before storing it.
func (c Cache) SetCompressed(key string, data []byte, expiration time.Duration) {
	dataCompressed := new(bytes.Buffer)
	gw := gzip.NewWriter(dataCompressed)
	io.Copy(gw, bytes.NewReader(data))
	gw.Close()
	c.Set(key, dataCompressed.Bytes(), expiration)
}
