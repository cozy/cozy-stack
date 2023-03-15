package logger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	debugRedisAddChannel = "add:log-debug"
	debugRedisRmvChannel = "rmv:log-debug"
	debugRedisPrefix     = "debug:"
)

var (
	ErrInvalidDomainFormat = errors.New("invalid domain format")
)

// RedisDebugger is a redis based [Debugger] implementation.
//
// It use redis to synchronize all the instances between them.
// This implementation is safe to use in a multi instance setup.
//
// Technically speaking this is a wrapper around a [MemDebugger] using redis pub/sub
// and store for syncing the instances between them and restoring the domain list at
// startup.
type RedisDebugger struct {
	client redis.UniversalClient
	store  *MemDebugger
	sub    *redis.PubSub
}

// NewRedisDebugger instantiates a new [RedisDebugger], bootstraps the service
// with the state saved in redis and starts the subscription to the change
// channel.
func NewRedisDebugger(client redis.UniversalClient) (*RedisDebugger, error) {
	ctx := context.Background()

	dbg := &RedisDebugger{
		client: client,
		store:  NewMemDebugger(),
		sub:    client.Subscribe(ctx, debugRedisAddChannel, debugRedisRmvChannel),
	}

	err := dbg.bootstrap(ctx)
	if err != nil {
		return nil, err
	}

	go dbg.subscribeEvents()

	return dbg, nil
}

// AddDomain adds the specified domain to the debug list.
func (r *RedisDebugger) AddDomain(domain string, ttl time.Duration) error {
	ctx := context.Background()

	if strings.ContainsRune(domain, '/') {
		return ErrInvalidDomainFormat
	}

	// Publish the domain to add to the other instances, the memory storage will be updated
	// when the instance will consume its own event.
	err := r.client.Publish(ctx, debugRedisAddChannel, domain+"/"+ttl.String()).Err()
	if err != nil {
		return err
	}

	key := debugRedisPrefix + domain
	err = r.client.Set(ctx, key, 0, ttl).Err()

	return err
}

// RemoveDomain removes the specified domain from the debug list.
func (r *RedisDebugger) RemoveDomain(domain string) error {
	ctx := context.Background()

	// Publish the domain to remove to the other instances, the memory storage will be updated
	// when the instance will consume its own event.
	err := r.client.Publish(ctx, debugRedisRmvChannel, domain+"/0").Err()
	if err != nil {
		return err
	}

	key := debugRedisPrefix + domain
	err = r.client.Del(ctx, key).Err()
	return err
}

func (r *RedisDebugger) ExpiresAt(domain string) *time.Time {
	return r.store.ExpiresAt(domain)
}

func (r *RedisDebugger) subscribeEvents() {
	for msg := range r.sub.Channel() {
		parts := strings.Split(msg.Payload, "/")
		domain := parts[0]

		switch msg.Channel {
		case debugRedisAddChannel:
			var ttl time.Duration
			if len(parts) >= 2 {
				ttl, _ = time.ParseDuration(parts[1])
			}

			_ = r.store.AddDomain(domain, ttl)

		case debugRedisRmvChannel:
			r.store.RemoveDomain(domain)
		}
	}
}

// Close the redis client and the subscription channel.
func (r *RedisDebugger) Close() error {
	err := r.sub.Close()
	if err != nil {
		return fmt.Errorf("failed to close the subscription: %w", err)
	}

	err = r.client.Close()
	if err != nil {
		return fmt.Errorf("failed to close the client: %w", err)
	}

	return nil
}

func (r *RedisDebugger) bootstrap(ctx context.Context) error {
	keys, err := r.client.Keys(ctx, debugRedisPrefix+"*").Result()
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}

	for _, key := range keys {
		ttl, err := r.client.TTL(ctx, key).Result()
		if err != nil {
			continue
		}

		domain := strings.TrimPrefix(key, debugRedisPrefix)
		r.store.AddDomain(domain, ttl)
	}

	return nil
}
