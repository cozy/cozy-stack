package auth

import (
	"encoding/hex"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/go-redis/redis/v7"
)

// Store is essentially an object to store and retrieve confirmation codes
type Store interface {
	AddCode(db prefixer.Prefixer) (string, error)
	GetCode(db prefixer.Prefixer, code string) (bool, error)
}

// storeTTL is the time an entry stay alive
var storeTTL = 5 * time.Minute

// storeCleanInterval is the time interval between each cleanup.
var storeCleanInterval = 1 * time.Hour

var mu sync.Mutex
var globalStore Store

// GetStore returns the store for temporary move objects.
func GetStore() Store {
	mu.Lock()
	defer mu.Unlock()
	if globalStore != nil {
		return globalStore
	}
	cli := config.GetConfig().SessionStorage.Client()
	if cli == nil {
		globalStore = newMemStore()
	} else {
		globalStore = &redisStore{cli}
	}
	return globalStore
}

type memStore struct {
	mu   sync.Mutex
	vals map[string]time.Time
}

func newMemStore() Store {
	store := &memStore{vals: make(map[string]time.Time)}
	go store.cleaner()
	return store
}

func (s *memStore) cleaner() {
	for range time.Tick(storeCleanInterval) {
		now := time.Now()
		for k, v := range s.vals {
			if now.After(v) {
				delete(s.vals, k)
			}
		}
	}
}

func (s *memStore) AddCode(db prefixer.Prefixer) (string, error) {
	code := makeSecret()
	s.mu.Lock()
	defer s.mu.Unlock()
	key := confirmKey(db, code)
	s.vals[key] = time.Now().Add(storeTTL)
	return code, nil
}

func (s *memStore) GetCode(db prefixer.Prefixer, code string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := confirmKey(db, code)
	exp, ok := s.vals[key]
	if !ok {
		return false, nil
	}
	if time.Now().After(exp) {
		delete(s.vals, key)
		return false, nil
	}
	return true, nil
}

type redisStore struct {
	c redis.UniversalClient
}

func (s *redisStore) AddCode(db prefixer.Prefixer) (string, error) {
	code := makeSecret()
	key := confirmKey(db, code)
	if err := s.c.Set(key, "1", storeTTL).Err(); err != nil {
		return "", err
	}
	return code, nil
}

func (s *redisStore) GetCode(db prefixer.Prefixer, code string) (bool, error) {
	key := confirmKey(db, code)
	n, err := s.c.Exists(key).Result()
	if err == redis.Nil || n == 0 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func confirmKey(db prefixer.Prefixer, suffix string) string {
	return db.DBPrefix() + ":confirm_auth:" + suffix
}

func makeSecret() string {
	return hex.EncodeToString(crypto.GenerateRandomBytes(8))
}
