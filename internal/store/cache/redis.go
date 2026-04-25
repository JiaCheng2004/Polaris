package cache

import (
	"context"
	"fmt"
	"time"

	redisv9 "github.com/redis/go-redis/v9"
)

type Redis struct {
	client *redisv9.Client
}

func NewRedis(url string) (*Redis, error) {
	if url == "" {
		return nil, fmt.Errorf("redis url is required")
	}

	options, err := redisv9.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redisv9.NewClient(options)
	return &Redis{client: client}, nil
}

func (r *Redis) Get(ctx context.Context, key string) (string, bool, error) {
	value, err := r.client.Get(ctx, key).Result()
	if err == nil {
		return value, true, nil
	}
	if err == redisv9.Nil {
		return "", false, nil
	}
	return "", false, fmt.Errorf("redis get %q: %w", key, err)
}

func (r *Redis) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %q: %w", key, err)
	}
	return nil
}

func (r *Redis) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	if ttl > 0 {
		pipe.PExpire(ctx, key, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("redis increment %q: %w", key, err)
	}
	return incr.Val(), nil
}

func (r *Redis) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

func (r *Redis) Close() error {
	return r.client.Close()
}
