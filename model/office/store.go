package office

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

type conflictDetector struct {
	ID     string
	Rev    string
	MD5Sum []byte
}

// Store is an object to store and retrieve document server keys <-> id,rev
type Store interface {
	AddDoc(db prefixer.Prefixer, payload conflictDetector) (string, error)
	GetDoc(db prefixer.Prefixer, secret string) (*conflictDetector, error)
	UpdateDoc(db prefixer.Prefixer, secret string, payload conflictDetector) error
	RemoveDoc(db prefixer.Prefixer, secret string) error
}

// storeTTL is the time an entry stay alive
var storeTTL = 24 * time.Hour

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
		now := time.Now()
		for k, v := range s.vals {
			if now.After(v.exp) {
				delete(s.byID, v.val.ID)
				delete(s.vals, k)
			}
		}
	}
}

func (s *memStore) AddDoc(db prefixer.Prefixer, payload conflictDetector) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret, ok := s.byID[payload.ID]
	if !ok {
		secret = makeSecret()
		s.byID[payload.ID] = secret
	}
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
		delete(s.byID, v.val.ID)
	}
	delete(s.vals, key)
	return nil
}

type redisStore struct {
	c   redis.UniversalClient
	ctx context.Context
}

func (s *redisStore) AddDoc(db prefixer.Prefixer, payload conflictDetector) (string, error) {
	idKey := docKey(db, payload.ID)
	if secret, err := s.c.Get(s.ctx, idKey).Result(); err == nil {
		return secret, nil
	}
	v, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	secret := makeSecret()
	key := docKey(db, secret)
	if err = s.c.Set(s.ctx, key, v, storeTTL).Err(); err != nil {
		return "", err
	}
	_ = s.c.Set(s.ctx, idKey, secret, storeTTL)
	return secret, nil
}

func (s *redisStore) GetDoc(db prefixer.Prefixer, secret string) (*conflictDetector, error) {
	key := docKey(db, secret)
	b, err := s.c.Get(s.ctx, key).Bytes()
	if err == redis.Nil {
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
		_ = s.c.Del(s.ctx, idKey)
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
