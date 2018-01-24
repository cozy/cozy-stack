package konnectorsauth

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
	InstanceDomain string
	AccountType    string
	ClientState    string
	Nonce          string
	ExpiresAt      int64
}

type stateStorage interface {
	Add(*stateHolder) (string, error)
	Find(ref string) *stateHolder
}

type memStateStorage map[string]*stateHolder

func (store memStateStorage) Add(state *stateHolder) (string, error) {
	state.ExpiresAt = time.Now().UTC().Add(stateTTL).Unix()
	ref := hex.EncodeToString(crypto.GenerateRandomBytes(16))
	store[ref] = state
	return ref, nil
}

func (store memStateStorage) Find(ref string) *stateHolder {
	state, ok := store[ref]
	if !ok {
		return nil
	}
	if state.ExpiresAt < time.Now().UTC().Unix() {
		delete(store, ref)
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

func (store *redisStateStorage) Add(s *stateHolder) (string, error) {
	ref := hex.EncodeToString(crypto.GenerateRandomBytes(16))
	bb, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return ref, store.cl.Set(ref, bb, stateTTL).Err()
}

func (store *redisStateStorage) Find(ref string) *stateHolder {
	bb, err := store.cl.Get(ref).Bytes()
	if err != nil {
		return nil
	}
	var s stateHolder
	err = json.Unmarshal(bb, &s)
	if err != nil {
		logger.WithNamespace("redis-state").Errorf(
			"bad state in redis %s", string(bb))
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
	cli := config.GetConfig().KonnectorsOauthStateStorage.Client()
	if cli == nil {
		globalStorage = &memStateStorage{}
	} else {
		globalStorage = &redisStateStorage{cl: cli}
	}
	return globalStorage
}
