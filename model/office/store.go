package office

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/redis/go-redis/v9"
)

type conflictDetector struct {
	ID     string
	Rev    string
	MD5Sum []byte
}

// Store is an object to store and retrieve document server keys <-> id,rev
type Store interface {
	GetSecretByID(db prefixer.Prefixer, id string) (string, error)
	AddDoc(db prefixer.Prefixer, payload conflictDetector) (string, error)
	GetDoc(db prefixer.Prefixer, secret string) (*conflictDetector, error)
	UpdateDoc(db prefixer.Prefixer, secret string, payload conflictDetector) error
	RemoveDoc(db prefixer.Prefixer, secret string) error
}

// storeTTL is the time an entry stay alive
var storeTTL = 30 * 24 * time.Hour

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

type memRef struct {
	val conflictDetector
	exp time.Time
}

func newMemStore() Store {
	store := &memStore{
		vals: make(map[string]*memRef),
		byID: make(map[string]string),
	}
	go store.cleaner()
	return store
}

type memStore struct {
	mu   sync.Mutex
	vals map[string]*memRef
	byID map[string]string // id -> secret
}

func (s *memStore) cleaner() {
	for range time.Tick(storeCleanInterval) {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.vals {
			if now.After(v.exp) {
				if s.byID[v.val.ID] == k {
					delete(s.byID, v.val.ID)
				}
				delete(s.vals, k)
			}
		}
		s.mu.Unlock()
	}
}

func (s *memStore) GetSecretByID(db prefixer.Prefixer, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byID[id], nil
}

func (s *memStore) AddDoc(db prefixer.Prefixer, payload conflictDetector) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret := makeSecret()
	s.byID[payload.ID] = secret
	key := docKey(db, secret)
	s.vals[key] = &memRef{
		val: payload,
		exp: time.Now().Add(storeTTL),
	}
	return secret, nil
}

func (s *memStore) GetDoc(db prefixer.Prefixer, secret string) (*conflictDetector, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := docKey(db, secret)
	ref, ok := s.vals[key]
	if !ok {
		return nil, nil
	}
	if time.Now().After(ref.exp) {
		delete(s.vals, key)
		return nil, nil
	}
	return &ref.val, nil
}

func (s *memStore) UpdateDoc(db prefixer.Prefixer, secret string, payload conflictDetector) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byID[payload.ID] != secret {
		return nil
	}
	key := docKey(db, secret)
	s.vals[key] = &memRef{
		val: payload,
		exp: time.Now().Add(storeTTL),
	}
	return nil
}

func (s *memStore) RemoveDoc(db prefixer.Prefixer, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := docKey(db, secret)
	if v, ok := s.vals[key]; ok {
		if s.byID[v.val.ID] == secret {
			delete(s.byID, v.val.ID)
		}
	}
	delete(s.vals, key)
	return nil
}

type redisStore struct {
	c   redis.UniversalClient
	ctx context.Context
}

func (s *redisStore) GetSecretByID(db prefixer.Prefixer, id string) (string, error) {
	idKey := docKey(db, id)
	return s.c.Get(s.ctx, idKey).Result()
}

func (s *redisStore) AddDoc(db prefixer.Prefixer, payload conflictDetector) (string, error) {
	idKey := docKey(db, payload.ID)
	v, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	secret := makeSecret()
	key := docKey(db, secret)
	if err = s.c.Set(s.ctx, key, v, storeTTL).Err(); err != nil {
		return "", err
	}
	if err = s.c.Set(s.ctx, idKey, secret, storeTTL).Err(); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *redisStore) GetDoc(db prefixer.Prefixer, secret string) (*conflictDetector, error) {
	key := docKey(db, secret)
	b, err := s.c.Get(s.ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var val conflictDetector
	if err = json.Unmarshal(b, &val); err != nil {
		return nil, err
	}
	return &val, nil
}

func (s *redisStore) UpdateDoc(db prefixer.Prefixer, secret string, payload conflictDetector) error {
	idKey := docKey(db, payload.ID)
	result, err := s.c.Get(s.ctx, idKey).Result()
	if err != nil {
		return err
	}
	if result != secret {
		return nil
	}
	v, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	key := docKey(db, secret)
	if err = s.c.Set(s.ctx, key, v, storeTTL).Err(); err != nil {
		return err
	}
	return nil
}

func (s *redisStore) RemoveDoc(db prefixer.Prefixer, secret string) error {
	payload, _ := s.GetDoc(db, secret)
	if payload != nil {
		idKey := docKey(db, payload.ID)
		if result, err := s.c.Get(s.ctx, idKey).Result(); err == nil && result == secret {
			_ = s.c.Del(s.ctx, idKey)
		}
	}
	key := docKey(db, secret)
	return s.c.Del(s.ctx, key).Err()
}

func docKey(db prefixer.Prefixer, suffix string) string {
	return db.DBPrefix() + ":oodoc:" + suffix
}

func makeSecret() string {
	return hex.EncodeToString(crypto.GenerateRandomBytes(12))
}
