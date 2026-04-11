package cache

import (
	"context"
	"fmt"
	"time"
)

type Redis struct{}

func NewRedis() (*Redis, error) {
	return nil, fmt.Errorf("%w: redis cache is deferred to Phase 2", errRedisUnavailable)
}

func (r *Redis) Get(context.Context, string) (string, bool, error) {
	return "", false, errRedisUnavailable
}

func (r *Redis) Set(context.Context, string, string, time.Duration) error {
	return errRedisUnavailable
}

func (r *Redis) Increment(context.Context, string, time.Duration) (int64, error) {
	return 0, errRedisUnavailable
}

func (r *Redis) Ping(context.Context) error {
	return errRedisUnavailable
}

func (r *Redis) Close() error {
	return nil
}

var errRedisUnavailable = fmt.Errorf("redis cache is not implemented")
