package sharing

import (
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/go-redis/redis"
)

// A UploadStore is essentially an object to store files metadata by key
type UploadStore interface {
	Get(db prefixer.Prefixer, key string) (*FileDocWithRevisions, error)
	Save(db prefixer.Prefixer, doc *FileDocWithRevisions) (string, error)
}

// uploadStoreTTL is the time an entry stay alive
var uploadStoreTTL = 5 * time.Minute

// uploadStoreCleanInterval is the time interval between each upload
// cleanup.
var uploadStoreCleanInterval = 1 * time.Hour

var mu sync.Mutex
var globalStore UploadStore

func getStore() UploadStore {
	mu.Lock()
	defer mu.Unlock()
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

type memRef struct {
	val *FileDocWithRevisions
	exp time.Time
}

func newMemStore() UploadStore {
	store := &memStore{vals: make(map[string]*memRef)}
	go store.cleaner()
	return store
}

type memStore struct {
	mu   sync.Mutex
	vals map[string]*memRef
}

func (s *memStore) cleaner() {
	for range time.Tick(uploadStoreCleanInterval) {
		now := time.Now()
		for k, v := range s.vals {
			if now.After(v.exp) {
				delete(s.vals, k)
			}
		}
	}
}

func (s *memStore) Get(db prefixer.Prefixer, key string) (*FileDocWithRevisions, error) {
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

func (s *memStore) Save(db prefixer.Prefixer, doc *FileDocWithRevisions) (string, error) {
	key := makeSecret()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vals[db.DBPrefix()+":"+key] = &memRef{
		val: doc,
		exp: time.Now().Add(uploadStoreTTL),
	}
	return key, nil
}

type redisStore struct {
	c redis.UniversalClient
}

func (s *redisStore) Get(db prefixer.Prefixer, key string) (*FileDocWithRevisions, error) {
	b, err := s.c.Get(db.DBPrefix() + ":" + key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc FileDocWithRevisions
	if err = json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *redisStore) Save(db prefixer.Prefixer, doc *FileDocWithRevisions) (string, error) {
	v, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	key := makeSecret()
	if err = s.c.Set(db.DBPrefix()+":"+key, v, uploadStoreTTL).Err(); err != nil {
		return "", err
	}
	return key, nil
}

func makeSecret() string {
	return hex.EncodeToString(crypto.GenerateRandomBytes(8))
}
