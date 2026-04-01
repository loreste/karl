package internal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Authentication errors
var (
	ErrAuthenticationRequired = errors.New("authentication required")
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrTokenExpired           = errors.New("token expired")
	ErrTokenInvalid           = errors.New("token invalid")
	ErrInsufficientPermissions = errors.New("insufficient permissions")
	ErrAPIKeyNotFound         = errors.New("API key not found")
	ErrAPIKeyRevoked          = errors.New("API key has been revoked")
	ErrCertificateInvalid     = errors.New("client certificate invalid")
	ErrCertificateNotAllowed  = errors.New("client certificate not in allowlist")
)

// AuthMethod represents the authentication method used
type AuthMethod string

const (
	AuthMethodNone     AuthMethod = "none"
	AuthMethodAPIKey   AuthMethod = "api_key"
	AuthMethodJWT      AuthMethod = "jwt"
	AuthMethodMTLS     AuthMethod = "mtls"
	AuthMethodBasic    AuthMethod = "basic"
	AuthMethodInternal AuthMethod = "internal"
)

// Permission represents an authorization permission
type Permission string

const (
	PermissionRead       Permission = "read"
	PermissionWrite      Permission = "write"
	PermissionAdmin      Permission = "admin"
	PermissionRecording  Permission = "recording"
	PermissionStatistics Permission = "statistics"
	PermissionConfig     Permission = "config"
)

// AuthConfig holds authentication configuration
type AuthConfig struct {
	// Enable authentication
	Enabled bool

	// Allowed authentication methods
	AllowedMethods []AuthMethod

	// API Key settings
	APIKeyHeader     string
	APIKeyQueryParam string

	// JWT settings
	JWTSecret          []byte
	JWTIssuer          string
	JWTAudience        string
	JWTExpirationHours int

	// mTLS settings
	RequireClientCert bool
	AllowedCertCNs    []string
	AllowedCertOUs    []string

	// Basic auth settings
	BasicAuthRealm string

	// Internal network bypass
	TrustedNetworks []string
}

// DefaultAuthConfig returns sensible defaults
func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		Enabled:            false,
		AllowedMethods:     []AuthMethod{AuthMethodAPIKey},
		APIKeyHeader:       "X-API-Key",
		APIKeyQueryParam:   "api_key",
		JWTExpirationHours: 24,
		BasicAuthRealm:     "Karl Media Server",
	}
}

// AuthenticatedUser represents an authenticated user
type AuthenticatedUser struct {
	ID          string
	Name        string
	Method      AuthMethod
	Permissions []Permission
	Metadata    map[string]string
	AuthTime    time.Time
	ExpiresAt   time.Time
}

// HasPermission checks if the user has a specific permission
func (u *AuthenticatedUser) HasPermission(perm Permission) bool {
	for _, p := range u.Permissions {
		if p == PermissionAdmin || p == perm {
			return true
		}
	}
	return false
}

// IsExpired checks if the authentication has expired
func (u *AuthenticatedUser) IsExpired() bool {
	if u.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(u.ExpiresAt)
}

// APIKey represents an API key
type APIKey struct {
	ID           string
	Name         string
	KeyHash      string // SHA-256 hash of the key
	Permissions  []Permission
	CreatedAt    time.Time
	ExpiresAt    time.Time
	LastUsedAt   time.Time
	Revoked      bool
	RevokedAt    time.Time
	Metadata     map[string]string
	RateLimitRPS int // Per-second rate limit for this key
}

// IsValid checks if the API key is valid
func (k *APIKey) IsValid() bool {
	if k.Revoked {
		return false
	}
	if !k.ExpiresAt.IsZero() && time.Now().After(k.ExpiresAt) {
		return false
	}
	return true
}

// APIKeyStore manages API keys
type APIKeyStore struct {
	keys map[string]*APIKey // keyed by ID
	mu   sync.RWMutex
}

// NewAPIKeyStore creates a new API key store
func NewAPIKeyStore() *APIKeyStore {
	return &APIKeyStore{
		keys: make(map[string]*APIKey),
	}
}

// GenerateAPIKey generates a new API key
func (s *APIKeyStore) GenerateAPIKey(name string, permissions []Permission, expiration time.Duration) (string, *APIKey, error) {
	// Generate random key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Encode as base64
	rawKey := base64.URLEncoding.EncodeToString(keyBytes)

	// Generate ID
	idBytes := make([]byte, 8)
	rand.Read(idBytes)
	id := hex.EncodeToString(idBytes)

	// Hash the key for storage
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	key := &APIKey{
		ID:          id,
		Name:        name,
		KeyHash:     keyHash,
		Permissions: permissions,
		CreatedAt:   time.Now(),
		Metadata:    make(map[string]string),
	}

	if expiration > 0 {
		key.ExpiresAt = time.Now().Add(expiration)
	}

	s.mu.Lock()
	s.keys[id] = key
	s.mu.Unlock()

	// Return the raw key (only returned once)
	return rawKey, key, nil
}

// AddAPIKey adds an existing API key
func (s *APIKeyStore) AddAPIKey(key *APIKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[key.ID] = key
}

// ValidateAPIKey validates an API key and returns the key info
func (s *APIKeyStore) ValidateAPIKey(rawKey string) (*APIKey, error) {
	// Hash the provided key
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find key by hash
	for _, key := range s.keys {
		if subtle.ConstantTimeCompare([]byte(key.KeyHash), []byte(keyHash)) == 1 {
			if key.Revoked {
				return nil, ErrAPIKeyRevoked
			}
			if !key.IsValid() {
				return nil, ErrTokenExpired
			}

			// Update last used time (need write lock for this in production)
			key.LastUsedAt = time.Now()

			return key, nil
		}
	}

	return nil, ErrAPIKeyNotFound
}

// RevokeAPIKey revokes an API key
func (s *APIKeyStore) RevokeAPIKey(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, exists := s.keys[id]
	if !exists {
		return ErrAPIKeyNotFound
	}

	key.Revoked = true
	key.RevokedAt = time.Now()

	return nil
}

// GetAPIKey retrieves an API key by ID
func (s *APIKeyStore) GetAPIKey(id string) (*APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, exists := s.keys[id]
	if !exists {
		return nil, ErrAPIKeyNotFound
	}

	return key, nil
}

// ListAPIKeys returns all API keys (for admin purposes)
func (s *APIKeyStore) ListAPIKeys() []*APIKey {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]*APIKey, 0, len(s.keys))
	for _, key := range s.keys {
		keys = append(keys, key)
	}
	return keys
}

// Authenticator handles authentication
type Authenticator struct {
	config   *AuthConfig
	keyStore *APIKeyStore

	// Basic auth users
	basicAuthUsers map[string]*basicAuthUser
	basicAuthMu    sync.RWMutex

	// mTLS certificate allowlist
	mtlsAllowList map[string]struct{} // CN or fingerprint
	mtlsMu        sync.RWMutex
}

type basicAuthUser struct {
	PasswordHash string
	Permissions  []Permission
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(config *AuthConfig) *Authenticator {
	if config == nil {
		config = DefaultAuthConfig()
	}

	auth := &Authenticator{
		config:         config,
		keyStore:       NewAPIKeyStore(),
		basicAuthUsers: make(map[string]*basicAuthUser),
		mtlsAllowList:  make(map[string]struct{}),
	}

	// Initialize mTLS allowlist
	for _, cn := range config.AllowedCertCNs {
		auth.mtlsAllowList[cn] = struct{}{}
	}

	return auth
}

// GetKeyStore returns the API key store
func (a *Authenticator) GetKeyStore() *APIKeyStore {
	return a.keyStore
}

// AuthenticateRequest authenticates an HTTP request
func (a *Authenticator) AuthenticateRequest(r *http.Request) (*AuthenticatedUser, error) {
	if !a.config.Enabled {
		return &AuthenticatedUser{
			ID:          "anonymous",
			Name:        "Anonymous",
			Method:      AuthMethodNone,
			Permissions: []Permission{PermissionRead, PermissionWrite, PermissionAdmin},
			AuthTime:    time.Now(),
		}, nil
	}

	var lastErr error

	// Try each allowed method in order
	for _, method := range a.config.AllowedMethods {
		switch method {
		case AuthMethodAPIKey:
			user, err := a.authenticateAPIKey(r)
			if err == nil {
				return user, nil
			}
			if !errors.Is(err, ErrAPIKeyNotFound) {
				lastErr = err
			}

		case AuthMethodBasic:
			user, err := a.authenticateBasic(r)
			if err == nil {
				return user, nil
			}
			if !errors.Is(err, ErrAuthenticationRequired) {
				lastErr = err
			}

		case AuthMethodMTLS:
			user, err := a.authenticateMTLS(r)
			if err == nil {
				return user, nil
			}
			if !errors.Is(err, ErrCertificateInvalid) {
				lastErr = err
			}
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, ErrAuthenticationRequired
}

// authenticateAPIKey authenticates using API key
func (a *Authenticator) authenticateAPIKey(r *http.Request) (*AuthenticatedUser, error) {
	// Check header first
	apiKey := r.Header.Get(a.config.APIKeyHeader)

	// Then check query parameter
	if apiKey == "" && a.config.APIKeyQueryParam != "" {
		apiKey = r.URL.Query().Get(a.config.APIKeyQueryParam)
	}

	// Check Authorization header with Bearer scheme
	if apiKey == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			apiKey = strings.TrimPrefix(auth, "Bearer ")
		}
	}

	if apiKey == "" {
		return nil, ErrAPIKeyNotFound
	}

	key, err := a.keyStore.ValidateAPIKey(apiKey)
	if err != nil {
		return nil, err
	}

	return &AuthenticatedUser{
		ID:          key.ID,
		Name:        key.Name,
		Method:      AuthMethodAPIKey,
		Permissions: key.Permissions,
		AuthTime:    time.Now(),
		ExpiresAt:   key.ExpiresAt,
		Metadata:    key.Metadata,
	}, nil
}

// authenticateBasic authenticates using basic auth
func (a *Authenticator) authenticateBasic(r *http.Request) (*AuthenticatedUser, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Basic ") {
		return nil, ErrAuthenticationRequired
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidCredentials
	}

	username, password := parts[0], parts[1]

	a.basicAuthMu.RLock()
	user, exists := a.basicAuthUsers[username]
	a.basicAuthMu.RUnlock()

	if !exists {
		return nil, ErrInvalidCredentials
	}

	// Verify password hash
	hash := sha256.Sum256([]byte(password))
	passwordHash := hex.EncodeToString(hash[:])

	if subtle.ConstantTimeCompare([]byte(user.PasswordHash), []byte(passwordHash)) != 1 {
		return nil, ErrInvalidCredentials
	}

	return &AuthenticatedUser{
		ID:          username,
		Name:        username,
		Method:      AuthMethodBasic,
		Permissions: user.Permissions,
		AuthTime:    time.Now(),
	}, nil
}

// authenticateMTLS authenticates using client certificate
func (a *Authenticator) authenticateMTLS(r *http.Request) (*AuthenticatedUser, error) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return nil, ErrCertificateInvalid
	}

	cert := r.TLS.PeerCertificates[0]

	// Check if CN is in allowlist
	a.mtlsMu.RLock()
	_, allowed := a.mtlsAllowList[cert.Subject.CommonName]
	a.mtlsMu.RUnlock()

	if !allowed {
		// Check if any OU is in allowlist
		for _, ou := range cert.Subject.OrganizationalUnit {
			a.mtlsMu.RLock()
			_, allowed = a.mtlsAllowList[ou]
			a.mtlsMu.RUnlock()
			if allowed {
				break
			}
		}
	}

	if !allowed && len(a.mtlsAllowList) > 0 {
		return nil, ErrCertificateNotAllowed
	}

	// Extract permissions from certificate extensions or use default
	permissions := []Permission{PermissionRead, PermissionWrite}

	return &AuthenticatedUser{
		ID:          cert.Subject.CommonName,
		Name:        cert.Subject.CommonName,
		Method:      AuthMethodMTLS,
		Permissions: permissions,
		AuthTime:    time.Now(),
		ExpiresAt:   cert.NotAfter,
		Metadata: map[string]string{
			"issuer":      cert.Issuer.CommonName,
			"serial":      cert.SerialNumber.String(),
			"fingerprint": certFingerprint(cert),
		},
	}, nil
}

// AddBasicAuthUser adds a basic auth user
func (a *Authenticator) AddBasicAuthUser(username, password string, permissions []Permission) {
	hash := sha256.Sum256([]byte(password))
	passwordHash := hex.EncodeToString(hash[:])

	a.basicAuthMu.Lock()
	defer a.basicAuthMu.Unlock()

	a.basicAuthUsers[username] = &basicAuthUser{
		PasswordHash: passwordHash,
		Permissions:  permissions,
	}
}

// RemoveBasicAuthUser removes a basic auth user
func (a *Authenticator) RemoveBasicAuthUser(username string) {
	a.basicAuthMu.Lock()
	defer a.basicAuthMu.Unlock()
	delete(a.basicAuthUsers, username)
}

// AddMTLSAllowedCN adds a CN to the mTLS allowlist
func (a *Authenticator) AddMTLSAllowedCN(cn string) {
	a.mtlsMu.Lock()
	defer a.mtlsMu.Unlock()
	a.mtlsAllowList[cn] = struct{}{}
}

// RemoveMTLSAllowedCN removes a CN from the mTLS allowlist
func (a *Authenticator) RemoveMTLSAllowedCN(cn string) {
	a.mtlsMu.Lock()
	defer a.mtlsMu.Unlock()
	delete(a.mtlsAllowList, cn)
}

// certFingerprint returns the SHA-256 fingerprint of a certificate
func certFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// AuthMiddleware creates an authentication middleware
func AuthMiddleware(auth *Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := auth.AuthenticateRequest(r)
			if err != nil {
				if auth.config.BasicAuthRealm != "" {
					w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, auth.config.BasicAuthRealm))
				}
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			// Add user to context
			ctx := context.WithValue(r.Context(), authUserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission creates a middleware that requires a specific permission
func RequirePermission(perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetAuthenticatedUser(r.Context())
			if user == nil {
				http.Error(w, ErrAuthenticationRequired.Error(), http.StatusUnauthorized)
				return
			}

			if !user.HasPermission(perm) {
				http.Error(w, ErrInsufficientPermissions.Error(), http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Context key for authenticated user
type contextKey string

const authUserKey contextKey = "authenticated_user"

// GetAuthenticatedUser retrieves the authenticated user from context
func GetAuthenticatedUser(ctx context.Context) *AuthenticatedUser {
	user, _ := ctx.Value(authUserKey).(*AuthenticatedUser)
	return user
}

// SetAuthenticatedUser sets the authenticated user in context
func SetAuthenticatedUser(ctx context.Context, user *AuthenticatedUser) context.Context {
	return context.WithValue(ctx, authUserKey, user)
}

// AuthStats holds authentication statistics
type AuthStats struct {
	TotalAuthentications   int64
	SuccessfulAuth         int64
	FailedAuth             int64
	APIKeyAuth             int64
	BasicAuth              int64
	MTLSAuth               int64
	ExpiredTokenAttempts   int64
	RevokedKeyAttempts     int64
}

// AuthAuditLog records authentication events
type AuthAuditLog struct {
	Timestamp  time.Time
	Method     AuthMethod
	UserID     string
	Success    bool
	ClientIP   string
	UserAgent  string
	ErrorMsg   string
	RequestURI string
}
