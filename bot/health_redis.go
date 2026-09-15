package bot

import (
	"context"
	"errors"
)

// Ping reports whether Redis is reachable. It is used by the process readiness
// endpoint introduced by the selective AutoMuteUs 9.1.0 backport.
func (redisInterface *RedisInterface) Ping(ctx context.Context) error {
	if redisInterface == nil || redisInterface.client == nil {
		return errors.New("redis client unavailable")
	}
	return redisInterface.client.Ping(ctx).Err()
}
