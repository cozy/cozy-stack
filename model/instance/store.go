package instance

import (
	"context"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/redis/go-redis/v9"
)

// Store is an object to store and retrieve session codes.
type Store interface {
	SaveSessionCode(db prefixer.Prefixer, code string) error
	SaveEmailVerfiedCode(db prefixer.Prefixer, code string) error
	CheckAndClearSessionCode(db prefixer.Prefixer, code string) bool
	CheckEmailVerifiedCode(db prefixer.Prefixer, code string) bool
}

// sessionCodeTTL is the time an entry for a session_code stays alive (1 week)
var sessionCodeTTL = 7 * 24 * time.Hour

// emailVerifiedCodeTTL is the time an entry for an email_verified_code stays alive
var emailVerifiedCodeTTL = 15 * time.Minute

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
	cli := config.GetConfig().SessionStorage
	if cli == nil {
		globalStore = newMemStore()
	} else {
		ctx := context.Background()
		globalStore = &redisStore{cli, ctx}
	}
	return globalStore
}

func newMemStore() Store {
	store := &memStore{vals: make(map[string]time.Time)}
	go store.cleaner()
	return store
}

type memStore struct {
	mu   sync.Mutex
	vals map[string]time.Time // session_code -> expiration time
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

func (s *memStore) SaveSessionCode(db prefixer.Prefixer, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sessionCodeKey(db, code)
	s.vals[key] = time.Now().Add(sessionCodeTTL)
	return nil
}

func (s *memStore) SaveEmailVerfiedCode(db prefixer.Prefixer, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := emailVerifiedCodeKey(db, code)
	s.vals[key] = time.Now().Add(emailVerifiedCodeTTL)
	return nil
}

func (s *memStore) CheckAndClearSessionCode(db prefixer.Prefixer, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sessionCodeKey(db, code)
	exp, ok := s.vals[key]
	if !ok {
		return false
	}
	delete(s.vals, key)
	return time.Now().Before(exp)
}

func (s *memStore) CheckEmailVerifiedCode(db prefixer.Prefixer, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := emailVerifiedCodeKey(db, code)
	exp, ok := s.vals[key]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.vals, key)
		return false
	}
	return true
}

type redisStore struct {
	c   redis.UniversalClient
	ctx context.Context
}

func (s *redisStore) SaveSessionCode(db prefixer.Prefixer, code string) error {
	key := sessionCodeKey(db, code)
	return s.c.Set(s.ctx, key, "1", sessionCodeTTL).Err()
}

func (s *redisStore) SaveEmailVerfiedCode(db prefixer.Prefixer, code string) error {
	key := emailVerifiedCodeKey(db, code)
	return s.c.Set(s.ctx, key, "1", emailVerifiedCodeTTL).Err()
}

func (s *redisStore) CheckAndClearSessionCode(db prefixer.Prefixer, code string) bool {
	key := sessionCodeKey(db, code)
	n, err := s.c.Del(s.ctx, key).Result()
	return err == nil && n > 0
}

func (s *redisStore) CheckEmailVerifiedCode(db prefixer.Prefixer, code string) bool {
	key := emailVerifiedCodeKey(db, code)
	n, err := s.c.Exists(s.ctx, key).Result()
	return err == nil && n > 0
}

func sessionCodeKey(db prefixer.Prefixer, suffix string) string {
	return db.DBPrefix() + ":sessioncode:" + suffix
}

func emailVerifiedCodeKey(db prefixer.Prefixer, suffix string) string {
	return db.DBPrefix() + ":emailverifiedcode:" + suffix
}
