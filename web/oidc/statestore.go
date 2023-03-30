package oidc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/redis/go-redis/v9"
)

const (
	stateTTL = 15 * time.Minute
	codeTTL  = 3 * time.Hour
)

type stateHolder struct {
	id        string
	expiresAt int64
	Provider  ProviderOIDC
	Instance  string
	Redirect  string
	Nonce     string
	Confirm   string
}

type ProviderOIDC int

const (
	GenericProvider ProviderOIDC = iota
	FranceConnectProvider
)

func newStateHolder(domain, redirect, confirm string, provider ProviderOIDC) *stateHolder {
	id := hex.EncodeToString(crypto.GenerateRandomBytes(16))
	nonce := hex.EncodeToString(crypto.GenerateRandomBytes(16))
	return &stateHolder{
		id:       id,
		Provider: provider,
		Instance: domain,
		Redirect: redirect,
		Confirm:  confirm,
		Nonce:    nonce,
	}
}

type stateStorage interface {
	Add(*stateHolder) error
	Find(id string) *stateHolder
	CreateCode(sub string) string
	GetSub(code string) string
}

type memStateStorage struct {
	states map[string]*stateHolder
	codes  map[string]string // delegated code -> sub
}

func (store memStateStorage) Add(state *stateHolder) error {
	state.expiresAt = time.Now().UTC().Add(stateTTL).Unix()
	store.states[state.id] = state
	return nil
}

func (store memStateStorage) Find(id string) *stateHolder {
	state, ok := store.states[id]
	if !ok {
		return nil
	}
	if state.expiresAt < time.Now().UTC().Unix() {
		delete(store.states, id)
		return nil
	}
	return state
}

func (store memStateStorage) CreateCode(sub string) string {
	code := makeCode()
	store.codes[code] = sub
	return code
}

func (store memStateStorage) GetSub(code string) string {
	return store.codes[code]
}

type subRedisInterface interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

type redisStateStorage struct {
	cl  subRedisInterface
	ctx context.Context
}

func (store *redisStateStorage) Add(s *stateHolder) error {
	serialized, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return store.cl.Set(store.ctx, s.id, serialized, stateTTL).Err()
}

func (store *redisStateStorage) Find(id string) *stateHolder {
	serialized, err := store.cl.Get(store.ctx, id).Bytes()
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

func (store *redisStateStorage) CreateCode(sub string) string {
	code := makeCode()
	store.cl.Set(store.ctx, code, sub, codeTTL)
	return code
}

func (store *redisStateStorage) GetSub(code string) string {
	return store.cl.Get(store.ctx, code).String()
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
		globalStorage = &memStateStorage{
			states: make(map[string]*stateHolder),
			codes:  make(map[string]string),
		}
	} else {
		ctx := context.Background()
		globalStorage = &redisStateStorage{cl: cli, ctx: ctx}
	}
	return globalStorage
}

func makeCode() string {
	return hex.EncodeToString(crypto.GenerateRandomBytes(12))
}
