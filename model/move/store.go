package move

import (
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/go-redis/redis/v7"
)

// Store is essentially an object to store and retrieve move requests
type Store interface {
	GetRequest(db prefixer.Prefixer, key string) (*Request, error)
	SaveRequest(db prefixer.Prefixer, req *Request) (string, error)
}

// storeTTL is the time an entry stay alive
var storeTTL = 5 * time.Minute

// storeCleanInterval is the time interval between each cleanup.
var storeCleanInterval = 1 * time.Hour

var mu sync.Mutex
var globalStore Store

func getStore() Store {
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

type memRef struct {
	val *Request
	exp time.Time
}

func newMemStore() Store {
	store := &memStore{vals: make(map[string]*memRef)}
	go store.cleaner()
	return store
}

type memStore struct {
	mu   sync.Mutex
	vals map[string]*memRef
}

func (s *memStore) cleaner() {
	for range time.Tick(storeCleanInterval) {
		now := time.Now()
		for k, v := range s.vals {
			if now.After(v.exp) {
				delete(s.vals, k)
			}
		}
	}
}

func (s *memStore) GetRequest(db prefixer.Prefixer, key string) (*Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key = db.DBPrefix() + ":" + key
	ref, ok := s.vals[key]
	if !ok {
		return nil, nil
	}
	if time.Now().After(ref.exp) {
		delete(s.vals, key)
	}
	return ref.val, nil
}

func (s *memStore) SaveRequest(db prefixer.Prefixer, req *Request) (string, error) {
	key := makeSecret()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vals[db.DBPrefix()+":"+key] = &memRef{
		val: req,
		exp: time.Now().Add(storeTTL),
	}
	return key, nil
}

type redisStore struct {
	c redis.UniversalClient
}

func (s *redisStore) GetRequest(db prefixer.Prefixer, key string) (*Request, error) {
	b, err := s.c.Get(db.DBPrefix() + ":" + key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var req Request
	if err = json.Unmarshal(b, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (s *redisStore) SaveRequest(db prefixer.Prefixer, req *Request) (string, error) {
	v, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	key := makeSecret()
	if err = s.c.Set(db.DBPrefix()+":"+key, v, storeTTL).Err(); err != nil {
		return "", err
	}
	return key, nil
}

func makeSecret() string {
	return hex.EncodeToString(crypto.GenerateRandomBytes(8))
}
