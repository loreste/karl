package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Authenticator handles API key authentication
type Authenticator struct {
	db       *sql.DB
	cache    map[string]*CachedKey
	cacheTTL time.Duration
	mu       sync.RWMutex
}

// CachedKey holds cached API key information
type CachedKey struct {
	Permissions []string
	RateLimit   int
	CachedAt    time.Time
}

// APIKey represents an API key
type APIKey struct {
	ID          string
	KeyHash     string
	Name        string
	Permissions []string
	RateLimit   int
	CreatedAt   time.Time
	LastUsed    time.Time
	Enabled     bool
}

// Common errors
var (
	ErrInvalidKey = errors.New("invalid API key")
	ErrKeyExpired = errors.New("API key expired")
	ErrKeyDisabled = errors.New("API key disabled")
)

// NewAuthenticator creates a new authenticator
func NewAuthenticator(dsn string) *Authenticator {
	auth := &Authenticator{
		cache:    make(map[string]*CachedKey),
		cacheTTL: 5 * time.Minute,
	}

	// Try to connect to database
	if dsn != "" {
		db, err := sql.Open("mysql", dsn)
		if err == nil {
			auth.db = db
		}
	}

	return auth
}

// ValidateKey validates an API key and returns its permissions
func (a *Authenticator) ValidateKey(key string) ([]string, error) {
	// Check cache first
	a.mu.RLock()
	cached, ok := a.cache[key]
	a.mu.RUnlock()

	if ok && time.Since(cached.CachedAt) < a.cacheTTL {
		return cached.Permissions, nil
	}

	// Hash the key
	keyHash := hashKey(key)

	// If no database, use in-memory validation
	if a.db == nil {
		// For development/testing, allow a default key
		if key == "karl-dev-key" {
			return []string{"*"}, nil
		}
		return nil, ErrInvalidKey
	}

	// Query database
	var (
		id          string
		permissions string
		rateLimit   int
		enabled     bool
	)

	err := a.db.QueryRow(`
		SELECT id, permissions, rate_limit, enabled
		FROM api_keys
		WHERE key_hash = ?
	`, keyHash).Scan(&id, &permissions, &rateLimit, &enabled)

	if err == sql.ErrNoRows {
		return nil, ErrInvalidKey
	}
	if err != nil {
		return nil, err
	}

	if !enabled {
		return nil, ErrKeyDisabled
	}

	// Parse permissions JSON
	var perms []string
	if err := json.Unmarshal([]byte(permissions), &perms); err != nil {
		perms = []string{}
	}

	// Update last used
	go a.updateLastUsed(id)

	// Cache result
	a.mu.Lock()
	a.cache[key] = &CachedKey{
		Permissions: perms,
		RateLimit:   rateLimit,
		CachedAt:    time.Now(),
	}
	a.mu.Unlock()

	return perms, nil
}

// updateLastUsed updates the last_used timestamp
func (a *Authenticator) updateLastUsed(id string) {
	if a.db == nil {
		return
	}

	_, _ = a.db.Exec(`UPDATE api_keys SET last_used = NOW() WHERE id = ?`, id)
}

// CreateKey creates a new API key
func (a *Authenticator) CreateKey(name string, permissions []string, rateLimit int) (string, error) {
	// Generate random key
	key, err := generateKey()
	if err != nil {
		return "", err
	}

	keyHash := hashKey(key)

	// Store in database
	if a.db != nil {
		permsJSON, _ := json.Marshal(permissions)

		id := generateID()
		_, err = a.db.Exec(`
			INSERT INTO api_keys (id, key_hash, name, permissions, rate_limit, enabled, created_at)
			VALUES (?, ?, ?, ?, ?, TRUE, NOW())
		`, id, keyHash, name, string(permsJSON), rateLimit)

		if err != nil {
			return "", err
		}
	}

	return key, nil
}

// RevokeKey revokes an API key
func (a *Authenticator) RevokeKey(keyHash string) error {
	if a.db == nil {
		return errors.New("database not available")
	}

	_, err := a.db.Exec(`UPDATE api_keys SET enabled = FALSE WHERE key_hash = ?`, keyHash)
	if err != nil {
		return err
	}

	// Invalidate cache
	a.mu.Lock()
	for k, v := range a.cache {
		if hashKey(k) == keyHash {
			delete(a.cache, k)
		}
		_ = v // Avoid unused warning
	}
	a.mu.Unlock()

	return nil
}

// ListKeys lists all API keys
func (a *Authenticator) ListKeys() ([]*APIKey, error) {
	if a.db == nil {
		return nil, errors.New("database not available")
	}

	rows, err := a.db.Query(`
		SELECT id, key_hash, name, permissions, rate_limit, created_at, last_used, enabled
		FROM api_keys
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		var (
			key         APIKey
			permissions string
			lastUsed    sql.NullTime
		)

		err := rows.Scan(
			&key.ID, &key.KeyHash, &key.Name, &permissions,
			&key.RateLimit, &key.CreatedAt, &lastUsed, &key.Enabled,
		)
		if err != nil {
			continue
		}

		if lastUsed.Valid {
			key.LastUsed = lastUsed.Time
		}

		_ = json.Unmarshal([]byte(permissions), &key.Permissions)
		keys = append(keys, &key)
	}

	return keys, nil
}

// ClearCache clears the key cache
func (a *Authenticator) ClearCache() {
	a.mu.Lock()
	a.cache = make(map[string]*CachedKey)
	a.mu.Unlock()
}

// Close closes the database connection
func (a *Authenticator) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

// hashKey hashes an API key
func hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// generateKey generates a random API key
func generateKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "karl_" + hex.EncodeToString(bytes), nil
}

// generateID generates a random ID
func generateID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to time-based ID if crypto/rand fails
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// Permission constants
const (
	PermSessionRead    = "session:read"
	PermSessionWrite   = "session:write"
	PermSessionDelete  = "session:delete"
	PermStatsRead      = "stats:read"
	PermRecordingRead  = "recording:read"
	PermRecordingWrite = "recording:write"
	PermAdmin          = "admin"
	PermAll            = "*"
)

// DefaultPermissions returns default permissions for a role
func DefaultPermissions(role string) []string {
	switch role {
	case "admin":
		return []string{PermAll}
	case "operator":
		return []string{
			PermSessionRead, PermSessionWrite, PermSessionDelete,
			PermStatsRead,
			PermRecordingRead, PermRecordingWrite,
		}
	case "viewer":
		return []string{
			PermSessionRead,
			PermStatsRead,
			PermRecordingRead,
		}
	case "recorder":
		return []string{
			PermRecordingRead, PermRecordingWrite,
		}
	default:
		return []string{}
	}
}
