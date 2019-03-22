package oidc

import (
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/go-redis/redis"
)

const stateTTL = 15 * time.Minute

type stateHolder struct {
	id        string
	expiresAt int64
	Instance  string
	Nonce     string
}

func newStateHolder(domain string) *stateHolder {
	id := hex.EncodeToString(crypto.GenerateRandomBytes(16))
	nonce := hex.EncodeToString(crypto.GenerateRandomBytes(16))
	return &stateHolder{
		id:       id,
		Instance: domain,
		Nonce:    nonce,
	}
}

type stateStorage interface {
	Add(*stateHolder) error
	Find(id string) *stateHolder
}

type memStateStorage map[string]*stateHolder

func (store memStateStorage) Add(state *stateHolder) error {
	state.expiresAt = time.Now().UTC().Add(stateTTL).Unix()
	store[state.id] = state
	return nil
}

func (store memStateStorage) Find(id string) *stateHolder {
	state, ok := store[id]
	if !ok {
		return nil
	}
	if state.expiresAt < time.Now().UTC().Unix() {
		delete(store, id)
		return nil
	}
	return state
}

type subRedisInterface interface {
	Get(key string) *redis.StringCmd
	Set(key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

type redisStateStorage struct {
	cl subRedisInterface
}

func (store *redisStateStorage) Add(s *stateHolder) error {
	serialized, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return store.cl.Set(s.id, serialized, stateTTL).Err()
}

func (store *redisStateStorage) Find(id string) *stateHolder {
	serialized, err := store.cl.Get(id).Bytes()
	if err != nil {
		return nil
	}
	var s stateHolder
	err = json.Unmarshal(serialized, &s)
	if err != nil {
		logger.WithNamespace("redis-state").Errorf(
			"Bad state in redis %s", string(serialized))
		return nil
	}
	return &s
}

var globalStorage stateStorage
var globalStorageMutex sync.Mutex

func getStorage() stateStorage {
	globalStorageMutex.Lock()
	defer globalStorageMutex.Unlock()
	if globalStorage != nil {
		return globalStorage
	}
	cli := config.GetConfig().OauthStateStorage.Client()
	if cli == nil {
		globalStorage = &memStateStorage{}
	} else {
		globalStorage = &redisStateStorage{cl: cli}
	}
	return globalStorage
}
