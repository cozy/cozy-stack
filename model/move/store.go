package move

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/go-redis/redis/v8"
)

// Store is essentially an object to store and retrieve move requests
type Store interface {
	GetRequest(db prefixer.Prefixer, secret string) (*Request, error)
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
		ctx := context.Background()
		globalStore = &redisStore{cli, ctx}
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

func (s *memStore) GetRequest(db prefixer.Prefixer, secret string) (*Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := requestKey(db, secret)
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
	secret := makeSecret()
	s.mu.Lock()
	defer s.mu.Unlock()
	key := requestKey(db, secret)
	s.vals[key] = &memRef{
		val: req,
		exp: time.Now().Add(storeTTL),
	}
	return secret, nil
}

func (s *memStore) SetAllowDeleteAccounts(db prefixer.Prefixer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := deleteAccountsKey(db)
	s.vals[key] = &memRef{
		val: nil,
		exp: time.Now().Add(storeTTL),
	}
	return nil
}

func (s *memStore) ClearAllowDeleteAccounts(db prefixer.Prefixer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := deleteAccountsKey(db)
	delete(s.vals, key)
	return nil
}

func (s *memStore) AllowDeleteAccounts(db prefixer.Prefixer) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := deleteAccountsKey(db)
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
	c   redis.UniversalClient
	ctx context.Context
}

func (s *redisStore) GetRequest(db prefixer.Prefixer, secret string) (*Request, error) {
	key := requestKey(db, secret)
	b, err := s.c.Get(s.ctx, key).Bytes()
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
	secret := makeSecret()
	key := requestKey(db, secret)
	if err = s.c.Set(s.ctx, key, v, storeTTL).Err(); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *redisStore) SetAllowDeleteAccounts(db prefixer.Prefixer) error {
	key := deleteAccountsKey(db)
	return s.c.Set(s.ctx, key, true, storeTTL).Err()
}

func (s *redisStore) ClearAllowDeleteAccounts(db prefixer.Prefixer) error {
	key := deleteAccountsKey(db)
	return s.c.Del(s.ctx, key).Err()
}

func (s *redisStore) AllowDeleteAccounts(db prefixer.Prefixer) bool {
	key := deleteAccountsKey(db)
	r, err := s.c.Exists(s.ctx, key).Result()
	if err != nil {
		return false
	}
	return r > 0
}

func requestKey(db prefixer.Prefixer, suffix string) string {
	return db.DBPrefix() + ":req:" + suffix
}

func deleteAccountsKey(db prefixer.Prefixer) string {
	return db.DBPrefix() + ":allow_delete_accounts"
}

func makeSecret() string {
	return hex.EncodeToString(crypto.GenerateRandomBytes(8))
}
