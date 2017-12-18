package vfs

import (
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/go-redis/redis"
)

// A DownloadStore is essentially an object to store Archives & Files by keys
type DownloadStore interface {
	AddFile(domain, filePath string) (string, error)
	AddArchive(domain string, archive *Archive) (string, error)
	GetFile(domain, key string) (string, error)
	GetArchive(domain, key string) (*Archive, error)
}

// downloadStoreTTL is the time an Archive stay alive
var downloadStoreTTL = 1 * time.Hour

// downloadStoreCleanInterval is the time interval between each download
// cleanup.
var downloadStoreCleanInterval = 1 * time.Hour

var globalStoreMu sync.Mutex
var globalStore DownloadStore

type memRef struct {
	val interface{}
	exp time.Time
}

// GetStore returns the DownloadStore.
func GetStore() DownloadStore {
	globalStoreMu.Lock()
	defer globalStoreMu.Unlock()
	if globalStore != nil {
		return globalStore
	}
	cli := config.GetConfig().DownloadStorage.Client()
	if cli == nil {
		globalStore = newMemStore()
	} else {
		globalStore = &redisStore{cli}
	}
	return globalStore
}

func newMemStore() DownloadStore {
	store := &memStore{vals: make(map[string]*memRef)}
	go store.cleaner()
	return store
}

type memStore struct {
	mu   sync.Mutex
	vals map[string]*memRef
}

func (s *memStore) cleaner() {
	for range time.Tick(downloadStoreCleanInterval) {
		now := time.Now()
		for k, v := range s.vals {
			if now.After(v.exp) {
				delete(s.vals, k)
			}
		}
	}
}

func (s *memStore) AddFile(domain, filePath string) (string, error) {
	key := makeSecret()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vals[domain+":"+key] = &memRef{
		val: filePath,
		exp: time.Now().Add(downloadStoreTTL),
	}
	return key, nil
}

func (s *memStore) AddArchive(domain string, archive *Archive) (string, error) {
	key := makeSecret()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vals[domain+":"+key] = &memRef{
		val: archive,
		exp: time.Now().Add(downloadStoreTTL),
	}
	return key, nil
}

func (s *memStore) GetFile(domain, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key = domain + ":" + key
	ref, ok := s.vals[key]
	if !ok {
		return "", nil
	}
	if time.Now().After(ref.exp) {
		delete(s.vals, key)
		return "", nil
	}
	f, ok := ref.val.(string)
	if !ok {
		return "", nil
	}
	return f, nil
}

func (s *memStore) GetArchive(domain, key string) (*Archive, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key = domain + ":" + key
	ref, ok := s.vals[key]
	if !ok {
		return nil, nil
	}
	if time.Now().After(ref.exp) {
		delete(s.vals, key)
		return nil, nil
	}
	a, ok := ref.val.(*Archive)
	if !ok {
		return nil, nil
	}
	return a, nil
}

type redisStore struct {
	c redis.UniversalClient
}

func (s *redisStore) AddFile(domain, filePath string) (string, error) {
	key := makeSecret()
	if err := s.c.Set(domain+":"+key, filePath, downloadStoreTTL).Err(); err != nil {
		return "", err
	}
	return key, nil
}

func (s *redisStore) AddArchive(domain string, archive *Archive) (string, error) {
	v, err := json.Marshal(archive)
	if err != nil {
		return "", err
	}
	key := makeSecret()
	if err = s.c.Set(domain+":"+key, v, downloadStoreTTL).Err(); err != nil {
		return "", err
	}
	return key, nil
}

func (s *redisStore) GetFile(domain, key string) (string, error) {
	f, err := s.c.Get(domain + ":" + key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return f, nil
}

func (s *redisStore) GetArchive(domain, key string) (*Archive, error) {
	b, err := s.c.Get(domain + ":" + key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	arch := &Archive{}
	if err = json.Unmarshal(b, &arch); err != nil {
		return nil, err
	}
	return arch, nil
}

func makeSecret() string {
	return hex.EncodeToString(crypto.GenerateRandomBytes(8))
}
