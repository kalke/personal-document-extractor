package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrUnavailable = errors.New("rate limit store unavailable")

type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

type Limiter struct {
	rdb   *redis.Client
	limit int
	now   func() time.Time
}

func New(rdb *redis.Client, limitPerMinute int) *Limiter {
	if limitPerMinute <= 0 {
		limitPerMinute = 60
	}
	return &Limiter{
		rdb:   rdb,
		limit: limitPerMinute,
		now:   time.Now,
	}
}

// Allow increments the fixed 1-minute window counter for principalID.
// Fail-closed: Redis errors return ErrUnavailable.
func (l *Limiter) Allow(ctx context.Context, kind, principalID string) (Result, error) {
	if l == nil || l.rdb == nil {
		return Result{}, ErrUnavailable
	}
	now := l.now().UTC()
	window := now.Truncate(time.Minute).Unix()
	key := fmt.Sprintf("rl:%s:%s:%d", kind, principalID, window)
	n, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if n == 1 {
		_ = l.rdb.Expire(ctx, key, 2*time.Minute).Err()
	}
	remaining := l.limit - int(n)
	if remaining < 0 {
		remaining = 0
	}
	res := Result{
		Allowed:    int(n) <= l.limit,
		Limit:      l.limit,
		Remaining:  remaining,
		RetryAfter: now.Truncate(time.Minute).Add(time.Minute).Sub(now),
	}
	if res.RetryAfter < time.Second {
		res.RetryAfter = time.Second
	}
	return res, nil
}
