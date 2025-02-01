package internal

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RTPRedisCache manages Redis-based RTP session storage
type RTPRedisCache struct {
	Client  *redis.Client
	Ctx     context.Context
	Enabled bool // ✅ Now public, so we can check if Redis is enabled
	TTL     time.Duration
	mu      sync.Mutex
}

// NewRTPRedisCache initializes Redis for RTP session tracking
func NewRTPRedisCache(config *Config) *RTPRedisCache {
	if !config.Database.RedisEnabled {
		log.Println("⚠️ Redis is disabled in configuration.")
		return nil
	}

	log.Println("🔌 Connecting to Redis at:", config.Database.RedisAddr)

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Database.RedisAddr,
		Password: "", // Assuming no password; update if needed
		DB:       0,  // Default database
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Redis connection failed: %v", err)
		return nil
	}

	log.Println("✅ Redis connected successfully.")
	return &RTPRedisCache{
		Client:  rdb,
		Ctx:     ctx,
		Enabled: true,
		TTL:     time.Duration(config.Database.RedisCleanupInterval) * time.Second,
	}
}

// StoreRTPPacket stores an RTP packet in Redis with an expiration time
func (r *RTPRedisCache) StoreRTPPacket(sessionID string, packetData string) {
	if !r.Enabled {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := "rtp_session:" + sessionID
	err := r.Client.Set(r.Ctx, key, packetData, r.TTL).Err()
	if err != nil {
		log.Printf("❌ Failed to store RTP packet in Redis: %v", err)
	}
}

// GetRTPPacket retrieves an RTP packet from Redis
func (r *RTPRedisCache) GetRTPPacket(sessionID string) (string, error) {
	if !r.Enabled {
		return "", nil
	}

	key := "rtp_session:" + sessionID
	val, err := r.Client.Get(r.Ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	} else if err != nil {
		log.Printf("❌ Failed to retrieve RTP packet from Redis: %v", err)
		return "", err
	}
	return val, nil
}

// DeleteRTPPacket removes an RTP packet from Redis
func (r *RTPRedisCache) DeleteRTPPacket(sessionID string) {
	if !r.Enabled {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := "rtp_session:" + sessionID
	err := r.Client.Del(r.Ctx, key).Err()
	if err != nil {
		log.Printf("❌ Failed to delete RTP packet from Redis: %v", err)
	}
}

// GetAllActiveSessions retrieves all active RTP sessions stored in Redis
func (r *RTPRedisCache) GetAllActiveSessions() ([]string, error) {
	if !r.Enabled {
		return nil, nil
	}

	keys, err := r.Client.Keys(r.Ctx, "rtp_session:*").Result()
	if err != nil {
		log.Printf("❌ Failed to fetch active RTP sessions: %v", err)
		return nil, err
	}

	return keys, nil
}

// AutoCleanupExpiredSessions runs a background job to clean up old sessions
func (r *RTPRedisCache) AutoCleanupExpiredSessions(interval time.Duration) {
	if !r.Enabled {
		return
	}

	log.Println("🗑️ Starting Redis auto-cleanup every", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		keys, err := r.Client.Keys(r.Ctx, "rtp_session:*").Result()
		if err != nil {
			log.Printf("❌ Redis cleanup error: %v", err)
			r.mu.Unlock()
			continue
		}

		for _, key := range keys {
			ttl, err := r.Client.TTL(r.Ctx, key).Result()
			if err == nil && ttl < 0 {
				r.Client.Del(r.Ctx, key)
				log.Printf("🗑️ Deleted expired RTP session: %s", key)
			}
		}
		r.mu.Unlock()
	}
}

// CheckRedisHealth periodically checks Redis availability
func (r *RTPRedisCache) CheckRedisHealth(interval time.Duration) {
	if !r.Enabled {
		return
	}

	log.Println("🩺 Monitoring Redis health every", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		err := r.Client.Ping(r.Ctx).Err()
		if err != nil {
			log.Printf("🚨 Redis health check failed: %v", err)
		} else {
			log.Println("✅ Redis is healthy.")
		}
		r.mu.Unlock()
	}
}

// Close gracefully shuts down Redis connection
func (r *RTPRedisCache) Close() {
	if !r.Enabled {
		return
	}

	log.Println("🔌 Closing Redis connection...")
	r.Client.Close()
	log.Println("✅ Redis connection closed.")
}
