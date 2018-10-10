package cache

import (
	"bytes"
	"compress/gzip"
	"io"
	"time"

	"github.com/go-redis/redis"
)

// Cache is a rudimentary key/value caching store backed by redis. It offers a
// Get/Set interface as well a its gzip compressed alternative
// GetCompressed/SetCompressed
type Cache struct {
	client redis.UniversalClient
}

// New returns a new Cache from a potentially nil redis client.
func New(client redis.UniversalClient) Cache {
	return Cache{client}
}

// Get fetch the cached asset at the given key, and returns true only if the
// asset was found.
func (c Cache) Get(key string) (io.Reader, bool) {
	if c.client != nil {
		cmd := c.client.Get(key)
		if b, err := cmd.Bytes(); err == nil {
			return bytes.NewReader(b), true
		}
	}
	return nil, false
}

// Set stores an asset to the given key.
func (c Cache) Set(key string, data []byte, expiration time.Duration) bool {
	if c.client != nil {
		c.client.Set(key, data, expiration)
	}
	return false
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
func (c Cache) SetCompressed(key string, data []byte, expiration time.Duration) bool {
	if c.client != nil {
		dataCompressed := new(bytes.Buffer)
		gw := gzip.NewWriter(dataCompressed)
		io.Copy(gw, bytes.NewReader(data))
		gw.Close()
		return c.Set(key, dataCompressed.Bytes(), expiration)
	}
	return false
}
