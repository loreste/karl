package internal

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisGoAdapter adapts go-redis/v9 Client to the RedisClient interface
// used by ClusterManager and RedisSessionStore.
type RedisGoAdapter struct {
	client *redis.Client
}

// NewRedisGoAdapter wraps a go-redis Client.
func NewRedisGoAdapter(client *redis.Client) *RedisGoAdapter {
	return &RedisGoAdapter{client: client}
}

func (a *RedisGoAdapter) Get(ctx context.Context, key string) (string, error) {
	return a.client.Get(ctx, key).Result()
}

func (a *RedisGoAdapter) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return a.client.Set(ctx, key, value, expiration).Err()
}

func (a *RedisGoAdapter) Del(ctx context.Context, keys ...string) error {
	return a.client.Del(ctx, keys...).Err()
}

func (a *RedisGoAdapter) Keys(ctx context.Context, pattern string) ([]string, error) {
	return a.client.Keys(ctx, pattern).Result()
}

func (a *RedisGoAdapter) Exists(ctx context.Context, keys ...string) (int64, error) {
	return a.client.Exists(ctx, keys...).Result()
}

func (a *RedisGoAdapter) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return a.client.Expire(ctx, key, expiration).Err()
}

func (a *RedisGoAdapter) Publish(ctx context.Context, channel string, message interface{}) error {
	return a.client.Publish(ctx, channel, message).Err()
}

func (a *RedisGoAdapter) Subscribe(ctx context.Context, channels ...string) (PubSub, error) {
	ps := a.client.Subscribe(ctx, channels...)
	return &redisPubSubAdapter{ps: ps}, nil
}

func (a *RedisGoAdapter) MGet(ctx context.Context, keys ...string) ([]string, error) {
	vals, err := a.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	result := make([]string, len(vals))
	for i, v := range vals {
		if v != nil {
			result[i] = v.(string)
		}
	}
	return result, nil
}

func (a *RedisGoAdapter) MSet(ctx context.Context, pairs ...interface{}) error {
	return a.client.MSet(ctx, pairs...).Err()
}

func (a *RedisGoAdapter) Scan(ctx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error) {
	return a.client.Scan(ctx, cursor, pattern, count).Result()
}

func (a *RedisGoAdapter) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return a.client.SAdd(ctx, key, members...).Err()
}

func (a *RedisGoAdapter) SRem(ctx context.Context, key string, members ...interface{}) error {
	return a.client.SRem(ctx, key, members...).Err()
}

func (a *RedisGoAdapter) SMembers(ctx context.Context, key string) ([]string, error) {
	return a.client.SMembers(ctx, key).Result()
}

func (a *RedisGoAdapter) Pipeline(ctx context.Context) RedisPipeline {
	return &redisPipeAdapter{pipe: a.client.Pipeline()}
}

// redisPubSubAdapter adapts go-redis PubSub to the PubSub interface.
type redisPubSubAdapter struct {
	ps *redis.PubSub
}

func (p *redisPubSubAdapter) Receive(ctx context.Context) (interface{}, error) {
	return p.ps.Receive(ctx)
}

func (p *redisPubSubAdapter) Close() error {
	return p.ps.Close()
}

// redisPipeAdapter adapts go-redis Pipeline to the RedisPipeline interface.
type redisPipeAdapter struct {
	pipe redis.Pipeliner
}

func (p *redisPipeAdapter) Get(key string) *PipelineCmd {
	cmd := p.pipe.Get(context.Background(), key)
	return &PipelineCmd{val: cmd.Val(), err: cmd.Err()}
}

func (p *redisPipeAdapter) Set(key string, value interface{}, expiration time.Duration) *PipelineCmd {
	cmd := p.pipe.Set(context.Background(), key, value, expiration)
	return &PipelineCmd{val: cmd.Val(), err: cmd.Err()}
}

func (p *redisPipeAdapter) Del(keys ...string) *PipelineCmd {
	cmd := p.pipe.Del(context.Background(), keys...)
	return &PipelineCmd{err: cmd.Err()}
}

func (p *redisPipeAdapter) Exec(ctx context.Context) error {
	_, err := p.pipe.Exec(ctx)
	return err
}
