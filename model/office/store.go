package office

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

// Store is an object to store and retrieve document server keys <-> id,rev
type Store interface {
	AddDoc(db prefixer.Prefixer, id, rev string) (string, error)
	GetDoc(db prefixer.Prefixer, secret string) (string, string, error)
	UpdateDoc(db prefixer.Prefixer, secret, id, rev string) error
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
		globalStore = &redisStore{cli}
	}
	return globalStore
}

type memRef struct {
	val [2]string
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
				delete(s.byID, v.val[0])
				delete(s.vals, k)
			}
		}
	}
}

func (s *memStore) AddDoc(db prefixer.Prefixer, id, rev string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret, ok := s.byID[id]
	if !ok {
		secret = makeSecret()
		s.byID[id] = secret
	}
	key := docKey(db, secret)
	s.vals[key] = &memRef{
		val: [2]string{id, rev},
		exp: time.Now().Add(storeTTL),
	}
	return secret, nil
}

func (s *memStore) GetDoc(db prefixer.Prefixer, secret string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := docKey(db, secret)
	ref, ok := s.vals[key]
	if !ok {
		return "", "", nil
	}
	if time.Now().After(ref.exp) {
		delete(s.vals, key)
		return "", "", nil
	}
	return ref.val[0], ref.val[1], nil
}

func (s *memStore) UpdateDoc(db prefixer.Prefixer, secret, id, rev string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := docKey(db, secret)
	s.vals[key] = &memRef{
		val: [2]string{id, rev},
		exp: time.Now().Add(storeTTL),
	}
	return nil
}

func (s *memStore) RemoveDoc(db prefixer.Prefixer, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := docKey(db, secret)
	if v, ok := s.vals[key]; ok {
		delete(s.byID, v.val[0])
	}
	delete(s.vals, key)
	return nil
}

type redisStore struct {
	c redis.UniversalClient
}

func (s *redisStore) AddDoc(db prefixer.Prefixer, id, rev string) (string, error) {
	idKey := docKey(db, id)
	if secret, err := s.c.Get(idKey).Result(); err == nil {
		return secret, nil
	}
	v, err := json.Marshal([]string{id, rev})
	if err != nil {
		return "", err
	}
	secret := makeSecret()
	key := docKey(db, secret)
	if err = s.c.Set(key, v, storeTTL).Err(); err != nil {
		return "", err
	}
	_ = s.c.Set(idKey, secret, storeTTL)
	return secret, nil
}

func (s *redisStore) GetDoc(db prefixer.Prefixer, secret string) (string, string, error) {
	key := docKey(db, secret)
	b, err := s.c.Get(key).Bytes()
	if err == redis.Nil {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	var val []string
	if err = json.Unmarshal(b, &val); err != nil || len(val) != 2 {
		return "", "", err
	}
	return val[0], val[1], nil
}

func (s *redisStore) UpdateDoc(db prefixer.Prefixer, secret, id, rev string) error {
	v, err := json.Marshal([]string{id, rev})
	if err != nil {
		return err
	}
	key := docKey(db, secret)
	if err = s.c.Set(key, v, storeTTL).Err(); err != nil {
		return err
	}
	return nil
}

func (s *redisStore) RemoveDoc(db prefixer.Prefixer, secret string) error {
	id, _, err := s.GetDoc(db, secret)
	if err != nil {
		idKey := docKey(db, id)
		_ = s.c.Del(idKey)
	}
	key := docKey(db, secret)
	return s.c.Del(key).Err()
}

func docKey(db prefixer.Prefixer, suffix string) string {
	return db.DBPrefix() + ":oodoc:" + suffix
}

func makeSecret() string {
	return hex.EncodeToString(crypto.GenerateRandomBytes(12))
}
