package limits

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis implementation of [Counter].
//
// This implementation is safe to use in multi-instances installation.
type Redis struct {
	Client redis.UniversalClient
	ctx    context.Context
}

// NewRedis returns a counter that can be mutualized between several
// cozy-stack processes by using redis.
func NewRedis(client redis.UniversalClient) Counter {
	return &Redis{client, context.Background()}
}

// incrWithTTL is a lua script for redis to increment a counter and sets a TTL
// if it doesn't have one.
const incrWithTTL = `
local n = redis.call("INCR", KEYS[1])
if redis.call("TTL", KEYS[1]) == -1 then
  redis.call("EXPIRE", KEYS[1], KEYS[2])
end
return n
`

func (r *Redis) Increment(key string, timeLimit time.Duration) (int64, error) {
	ttl := strconv.FormatInt(int64(timeLimit/time.Second), 10)
	count, err := r.Client.Eval(r.ctx, incrWithTTL, []string{key, ttl}).Result()
	if err != nil {
		return 0, err
	}
	return count.(int64), nil
}

func (r *Redis) Reset(key string) error {
	_, err := r.Client.Del(r.ctx, key).Result()
	return err
}
