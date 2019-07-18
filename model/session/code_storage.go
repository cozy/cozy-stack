package session

import (
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/go-redis/redis"
)

type codeStorage interface {
	Add(*Code) error
	FindAndDelete(value string, app string) *Code
}

type memCodeStorage struct {
	codes []*Code
}

func (store *memCodeStorage) Add(code *Code) error {
	code.ExpiresAt = time.Now().UTC().Add(CodeTTL).Unix()
	store.codes = append(store.codes, code)
	return nil
}

func (store *memCodeStorage) FindAndDelete(value string, app string) *Code {
	var found *Code
	if len(store.codes) == 0 {
		return nil
	}

	// See https://github.com/golang/go/wiki/SliceTricks#filtering-without-allocating
	validCodes := store.codes[:0]
	now := time.Now().UTC().Unix()
	for _, c := range store.codes {
		if now < c.ExpiresAt {
			if c.Value == value && c.AppHost == app {
				found = c
			} else {
				validCodes = append(validCodes, c)
			}
		}
	}
	store.codes = validCodes
	return found
}

var codelog = logger.WithNamespace("sessions-code")

type subRedisInterface interface {
	Eval(script string, keys []string, args ...interface{}) *redis.Cmd
	Set(key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

type redisCodeStorage struct {
	cl subRedisInterface
}

func (store *redisCodeStorage) Add(c *Code) error {
	err := store.cl.Set(c.Value+":"+c.AppHost, c.SessionID, CodeTTL).Err()
	if err != nil {
		codelog.Errorf("Cannot save a sessions code: %s", err)
	}
	return err
}

const luaGetAndDelete = `local v = redis.call("GET", KEYS[1]); redis.call("DEL", KEYS[1]); return v`

func (store *redisCodeStorage) FindAndDelete(value, app string) *Code {
	sessionID, err := store.cl.Eval(luaGetAndDelete, []string{value + ":" + app}).Result()
	if err != nil {
		codelog.Errorf("Error while fetching a sessions code: %s", err)
		return nil
	}
	if sessionID == redis.Nil {
		return nil
	}

	return &Code{
		SessionID: sessionID.(string),
		Value:     value,
		AppHost:   app,
	}
}

var globalStorage codeStorage
var globalStorageMutex sync.Mutex

func getStorage() codeStorage {
	globalStorageMutex.Lock()
	defer globalStorageMutex.Unlock()
	if globalStorage != nil {
		return globalStorage
	}
	cli := config.GetConfig().SessionStorage.Client()
	if cli == nil {
		globalStorage = &memCodeStorage{}
	} else {
		globalStorage = &redisCodeStorage{cl: cli}
	}
	return globalStorage
}
