package lock

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/redis/go-redis/v9"
)

const luaRefresh = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("pexpire", KEYS[1], ARGV[2]) else return 0 end`
const luaRelease = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`

type subRedisInterface interface {
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd
}

const (
	basicLockNS = "locks:"

	// LockValueSize is the size of the random value used to ensure a lock
	// is ours. If two stack were to generate the same value, locks will break.
	lockTokenSize = 20

	// LockTimeout is the expiration of a redis lock
	// if any operation is longer than this, it should
	// refresh the lock
	LockTimeout = 20 * time.Second

	// WaitTimeout maximum time to wait before returning control to caller.
	WaitTimeout = 1 * time.Minute

	// WaitRetry time to wait between retries
	WaitRetry = 100 * time.Millisecond
)

var (
	// ErrTooManyRetries is the error returned when despite several tries
	// we never managed to get a lock
	ErrTooManyRetries = errors.New("abort after too many failures without getting the lock")
)

type redisLock struct {
	client subRedisInterface
	ctx    context.Context
	mu     sync.Mutex
	key    string
	token  string
	// readers is the number of readers when the lock is acquired for reading
	// or 0 when it is unlocked, or -1 when it is locked for writing.
	readers int
}

func (rl *redisLock) extends() (bool, error) {
	if rl.token == "" || rl.readers < 0 {
		return false, nil
	}

	// we already have a lock, attempts to extends it
	ttl := strconv.FormatInt(int64(LockTimeout/time.Millisecond), 10)
	ok, err := rl.client.Eval(rl.ctx, luaRefresh, []string{rl.key}, rl.token, ttl).Result()
	if err != nil {
		return false, err // most probably redis connectivity error
	}

	if ok == int64(1) {
		rl.readers++
		return true, nil
	}

	return false, nil
}

func (rl *redisLock) obtains(writing bool, token string) (bool, error) {
	// Try to obtain a lock
	ok, err := rl.client.SetNX(rl.ctx, rl.key, token, LockTimeout).Result()
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

func (rl *redisLock) obtainsWriting(token string) (bool, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.obtains(true, token)
}

func (rl *redisLock) LockWithTimeout(timeout time.Duration) error {
	// Calculate the timestamp we are willing to wait for
	stop := time.Now().Add(timeout)
	redislocksMu.Lock()
	token := utils.RandomStringFast(redisRng, lockTokenSize)
	redislocksMu.Unlock()
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

func (rl *redisLock) Lock() error {
	return rl.LockWithTimeout(LockTimeout)
}

func (rl *redisLock) extendsOrObtainsReading(token string) (bool, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if ok, err := rl.extends(); err != nil || ok {
		return ok, err
	}
	return rl.obtains(false, token)
}

func (rl *redisLock) RLock() error {
	stop := time.Now().Add(LockTimeout)
	redislocksMu.Lock()
	token := utils.RandomStringFast(redisRng, lockTokenSize)
	redislocksMu.Unlock()
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
		redisLogger.Errorf("Invalid unlocking: %v %d (%s)", writing, rl.readers, rl.key)
		return
	}

	if !writing && rl.readers > 1 {
		rl.readers--
		return
	}

	_, err := rl.client.Eval(rl.ctx, luaRelease, []string{rl.key}, rl.token).Result()
	if err != nil {
		redisLogger.Warnf("Failed to unlock: %s (%s)", err.Error(), rl.key)
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

var redislocksMu sync.Mutex
var redisRng *rand.Rand
var redisLogger *logger.Entry

type RedisLockGetter struct {
	client redis.UniversalClient
	locks  *sync.Map
}

func NewRedisLockGetter(client redis.UniversalClient) *RedisLockGetter {
	redisRng = rand.New(rand.NewSource(time.Now().UnixNano()))
	redisLogger = logger.WithNamespace("redis-lock")

	return &RedisLockGetter{
		client: client,
		locks:  new(sync.Map),
	}
}

func (r *RedisLockGetter) ReadWrite(db prefixer.Prefixer, name string) ErrorRWLocker {
	lock, _ := r.locks.LoadOrStore(db.DBPrefix()+"/"+name, &redisLock{
		client: r.client,
		ctx:    context.Background(),
		key:    basicLockNS + name,
	})

	return lock.(*redisLock)
}

// LongOperation returns a lock suitable for long operations. It will refresh
// the lock in redis to avoid its automatic expiration.
func (r *RedisLockGetter) LongOperation(db prefixer.Prefixer, name string) ErrorLocker {
	return &longOperation{
		lock: r.ReadWrite(db, name),
	}
}
