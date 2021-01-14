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
	SetAllowDeleteAccounts(db prefixer.Prefixer) error
	ClearAllowDeleteAccounts(db prefixer.Prefixer) error
	AllowDeleteAccounts(db prefixer.Prefixer) bool
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
	key = db.DBPrefix() + ":req:" + key
	ref, ok := s.vals[key]
	if !ok {
		return nil, nil
	}
	if time.Now().After(ref.exp) {
		delete(s.vals, key)
		return nil, nil
	}
	return ref.val, nil
}

func (s *memStore) SaveRequest(db prefixer.Prefixer, req *Request) (string, error) {
	key := makeSecret()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vals[db.DBPrefix()+":req:"+key] = &memRef{
		val: req,
		exp: time.Now().Add(storeTTL),
	}
	return key, nil
}

func (s *memStore) SetAllowDeleteAccounts(db prefixer.Prefixer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vals[db.DBPrefix()+":allow_delete_accounts"] = &memRef{
		val: nil,
		exp: time.Now().Add(storeTTL),
	}
	return nil
}

func (s *memStore) ClearAllowDeleteAccounts(db prefixer.Prefixer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.vals, db.DBPrefix()+":allow_delete_accounts")
	return nil
}

func (s *memStore) AllowDeleteAccounts(db prefixer.Prefixer) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := db.DBPrefix() + ":allow_delete_accounts"
	ref, ok := s.vals[key]
	if !ok {
		return false
	}
	if time.Now().After(ref.exp) {
		delete(s.vals, key)
		return false
	}
	return true
}

type redisStore struct {
	c redis.UniversalClient
}

func (s *redisStore) GetRequest(db prefixer.Prefixer, key string) (*Request, error) {
	b, err := s.c.Get(db.DBPrefix() + ":req:" + key).Bytes()
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
	if err = s.c.Set(db.DBPrefix()+":req:"+key, v, storeTTL).Err(); err != nil {
		return "", err
	}
	return key, nil
}

func (s *redisStore) SetAllowDeleteAccounts(db prefixer.Prefixer) error {
	key := db.DBPrefix() + ":allow_delete_accounts"
	return s.c.Set(key, true, storeTTL).Err()
}

func (s *redisStore) ClearAllowDeleteAccounts(db prefixer.Prefixer) error {
	key := db.DBPrefix() + ":allow_delete_accounts"
	return s.c.Del(key).Err()
}

func (s *redisStore) AllowDeleteAccounts(db prefixer.Prefixer) bool {
	key := db.DBPrefix() + ":allow_delete_accounts"
	r, err := s.c.Exists(db.DBPrefix() + ":req:" + key).Result()
	if err != nil {
		return false
	}
	return r > 0
}

func makeSecret() string {
	return hex.EncodeToString(crypto.GenerateRandomBytes(8))
}
