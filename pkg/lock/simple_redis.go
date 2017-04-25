package lock

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/utils"
)

const luaRefresh = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("pexpire", KEYS[1], ARGV[2]) else return 0 end`
const luaRelease = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`

type fakeRWLock struct {
	ErrorLocker
}

func (l fakeRWLock) Lock() error    { return l.ErrorLocker.Lock() }
func (l fakeRWLock) RLock() error   { return l.ErrorLocker.Lock() }
func (l fakeRWLock) Unlock() error  { return l.ErrorLocker.Unlock() }
func (l fakeRWLock) RUnlock() error { return l.ErrorLocker.Unlock() }

func getReadisReadWriteLock(c subRedisInterface, ns string) ErrorRWLocker {
	return fakeRWLock{getRedisSimpleLock(c, ns)}
}

const (
	basicLockNS = "locks:"

	// LockValueSize is the size of the random value used to ensure a lock
	// is ours. If two stack were to generate the same value, locks will break.
	lockTokenSize = 16

	// LockTimeout is the expiration of a redis lock
	// if any operation is longer than this, it should
	// refresh the lock
	LockTimeout = 15 * time.Second

	// WaitTimeout maximum time to wait before returning control to caller.
	WaitTimeout = 2 * time.Minute

	// WaitRetry time to wait between retries
	WaitRetry = 100 * time.Millisecond
)

var (
	// ErrTooManyRetry is the error returned when despite several tries
	// we never managed to get a lock
	ErrTooManyRetry = errors.New("too many retry")
)

type redisLock struct {
	client subRedisInterface
	mu     sync.Mutex
	key    string
	token  string
}

func (rl *redisLock) Lock() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.token != "" {
		// we already have a lock, attempts to extends it
		ttl := strconv.FormatInt(int64(LockTimeout/time.Millisecond), 10)
		ok, err := rl.client.Eval(luaRefresh, []string{rl.key}, rl.token, ttl).Result()
		if err != nil {
			return err // most probably redis connectivity error
		}

		if ok == int64(1) {
			return nil
		}
		// this lock is unavaiable, fallback to creating it
		rl.token = ""
	}

	token := utils.RandomString(lockTokenSize)

	// Calculate the timestamp we are willing to wait for
	stop := time.Now().Add(LockTimeout)
	for {
		// Try to obtain a lock
		ok, err := rl.client.SetNX(rl.key, token, LockTimeout).Result()
		if err != nil {
			return err // most probably redis connectivity error
		}

		if ok {
			rl.token = token
			return nil
		}

		if time.Now().Add(WaitRetry).After(stop) {
			break
		}
		time.Sleep(WaitRetry)
	}

	return ErrTooManyRetry
}

func (rl *redisLock) Unlock() error {
	rl.mu.Lock()
	defer func() { rl.token = "" }()
	defer rl.mu.Unlock()
	_, err := rl.client.Eval(luaRelease, []string{rl.key}, rl.token).Result()
	return err
}

var redislocks map[string]*redisLock
var redislocksMu sync.Mutex

func makeRedisSimpleLock(c subRedisInterface, ns string) *redisLock {
	return &redisLock{
		client: c,
		key:    basicLockNS + ns,
	}
}

func getRedisSimpleLock(c subRedisInterface, ns string) ErrorLocker {
	redislocksMu.Lock()
	defer redislocksMu.Unlock()
	if redislocks == nil {
		redislocks = make(map[string]*redisLock)
	}
	l, ok := redislocks[ns]
	if !ok {
		redislocks[ns] = makeRedisSimpleLock(c, ns)
		l = redislocks[ns]
	}
	return l
}
