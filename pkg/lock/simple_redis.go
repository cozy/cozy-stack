package lock

import (
	"errors"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/go-redis/redis/v7"
	"github.com/sirupsen/logrus"
)

const luaRefresh = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("pexpire", KEYS[1], ARGV[2]) else return 0 end`
const luaRelease = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`

type subRedisInterface interface {
	SetNX(key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	Eval(script string, keys []string, args ...interface{}) *redis.Cmd
}

const (
	basicLockNS = "locks:"

	// LockValueSize is the size of the random value used to ensure a lock
	// is ours. If two stack were to generate the same value, locks will break.
	lockTokenSize = 20

	// LockTimeout is the expiration of a redis lock
	// if any operation is longer than this, it should
	// refresh the lock
	LockTimeout = 15 * time.Second

	// WaitTimeout maximum time to wait before returning control to caller.
	WaitTimeout = 1 * time.Minute

	// WaitRetry time to wait between retries
	WaitRetry = 100 * time.Millisecond
)

var (
	// ErrTooManyRetries is the error returned when despite several tries
	// we never managed to get a lock
	ErrTooManyRetries = errors.New("too many retry")
)

type redisLock struct {
	client subRedisInterface
	mu     sync.Mutex
	key    string
	token  string
	// readers is the number of readers when the lock is acquired for reading
	// or 0 when it is unlocked, or -1 when it is locked for writing.
	readers int
	log     *logrus.Entry
	rng     *rand.Rand
}

func (rl *redisLock) extends(writing bool) (bool, error) {
	if rl.token == "" {
		return false, nil
	}
	if (writing && rl.readers > 0) || (!writing && rl.readers < 0) {
		return false, nil
	}

	// we already have a lock, attempts to extends it
	ttl := strconv.FormatInt(int64(LockTimeout/time.Millisecond), 10)
	ok, err := rl.client.Eval(luaRefresh, []string{rl.key}, rl.token, ttl).Result()
	if err != nil {
		return false, err // most probably redis connectivity error
	}

	if ok == int64(1) {
		if !writing {
			rl.readers++
		}
		return true, nil
	}

	return false, nil
}

func (rl *redisLock) obtains(writing bool, token string) (bool, error) {
	// Try to obtain a lock
	ok, err := rl.client.SetNX(rl.key, token, LockTimeout).Result()
	if err != nil {
		return false, err // most probably redis connectivity error
	}
	if !ok {
		return false, nil
	}

	rl.token = token
	if writing {
		rl.readers = -1
	} else {
		rl.readers++
	}
	return true, nil
}

func (rl *redisLock) extendsWriting() (bool, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.extends(true)
}

func (rl *redisLock) obtainsWriting(token string) (bool, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.obtains(true, token)
}

func (rl *redisLock) Lock() error {
	ok, err := rl.extendsWriting()
	if err != nil || ok {
		return err
	}

	// Calculate the timestamp we are willing to wait for
	stop := time.Now().Add(LockTimeout)
	token := utils.RandomStringFast(rl.rng, lockTokenSize)
	for {
		ok, err := rl.obtainsWriting(token)
		if err != nil || ok {
			return err
		}
		if time.Now().Add(WaitRetry).After(stop) {
			break
		}
		time.Sleep(WaitRetry)
	}

	return ErrTooManyRetries
}

func (rl *redisLock) extendsOrObtainsReading(token string) (bool, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if ok, err := rl.extends(false); err != nil || ok {
		return ok, err
	}
	return rl.obtains(false, token)
}

func (rl *redisLock) RLock() error {
	stop := time.Now().Add(LockTimeout)
	token := utils.RandomStringFast(rl.rng, lockTokenSize)
	for {
		ok, err := rl.extendsOrObtainsReading(token)
		if err != nil || ok {
			return err
		}
		if time.Now().Add(WaitRetry).After(stop) {
			break
		}
		time.Sleep(WaitRetry)
	}

	return ErrTooManyRetries
}

func (rl *redisLock) unlock(writing bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if (writing && rl.readers > 0) || (!writing && rl.readers < 0) {
		rl.log.Errorf("Invalid unlocking: %v %d", writing, rl.readers)
		return
	}

	if !writing && rl.readers > 1 {
		rl.readers--
		return
	}

	_, err := rl.client.Eval(luaRelease, []string{rl.key}, rl.token).Result()
	if err != nil {
		rl.log.Warnf("Failed to unlock: %s", err.Error())
	}

	if writing {
		rl.readers = 0
	} else {
		rl.readers--
	}
	if rl.readers == 0 {
		rl.token = ""
	}
}

func (rl *redisLock) Unlock() {
	rl.unlock(true)
}

func (rl *redisLock) RUnlock() {
	rl.unlock(false)
}

var redislocks map[string]*redisLock
var redislocksMu sync.Mutex

func makeRedisSimpleLock(c subRedisInterface, ns string) *redisLock {
	return &redisLock{
		client: c,
		key:    basicLockNS + ns,
		log:    logger.WithDomain(ns).WithField("nspace", "redis-lock"),
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func getRedisReadWriteLock(c subRedisInterface, name string) ErrorRWLocker {
	redislocksMu.Lock()
	defer redislocksMu.Unlock()
	if redislocks == nil {
		redislocks = make(map[string]*redisLock)
	}
	l, ok := redislocks[name]
	if !ok {
		redislocks[name] = makeRedisSimpleLock(c, name)
		l = redislocks[name]
	}
	return l
}
