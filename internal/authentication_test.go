package internal

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultAuthConfig(t *testing.T) {
	config := DefaultAuthConfig()

	if config.Enabled {
		t.Error("Auth should be disabled by default")
	}
	if config.APIKeyHeader != "X-API-Key" {
		t.Errorf("Expected APIKeyHeader 'X-API-Key', got %s", config.APIKeyHeader)
	}
	if config.JWTExpirationHours != 24 {
		t.Errorf("Expected JWTExpirationHours 24, got %d", config.JWTExpirationHours)
	}
}

func TestNewAuthenticator(t *testing.T) {
	auth := NewAuthenticator(nil)

	if auth == nil {
		t.Fatal("NewAuthenticator returned nil")
	}
	if auth.keyStore == nil {
		t.Error("Key store not initialized")
	}
}

func TestAuthenticatedUser_HasPermission(t *testing.T) {
	user := &AuthenticatedUser{
		Permissions: []Permission{PermissionRead, PermissionWrite},
	}

	if !user.HasPermission(PermissionRead) {
		t.Error("Should have read permission")
	}
	if !user.HasPermission(PermissionWrite) {
		t.Error("Should have write permission")
	}
	if user.HasPermission(PermissionAdmin) {
		t.Error("Should not have admin permission")
	}
}

func TestAuthenticatedUser_HasPermission_Admin(t *testing.T) {
	user := &AuthenticatedUser{
		Permissions: []Permission{PermissionAdmin},
	}

	// Admin should have all permissions
	if !user.HasPermission(PermissionRead) {
		t.Error("Admin should have read permission")
	}
	if !user.HasPermission(PermissionWrite) {
		t.Error("Admin should have write permission")
	}
	if !user.HasPermission(PermissionRecording) {
		t.Error("Admin should have recording permission")
	}
}

func TestAuthenticatedUser_IsExpired(t *testing.T) {
	// Not expired
	user := &AuthenticatedUser{
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if user.IsExpired() {
		t.Error("Should not be expired")
	}

	// Expired
	user.ExpiresAt = time.Now().Add(-time.Hour)
	if !user.IsExpired() {
		t.Error("Should be expired")
	}

	// No expiration
	user.ExpiresAt = time.Time{}
	if user.IsExpired() {
		t.Error("Should not be expired when no expiration set")
	}
}

func TestAPIKey_IsValid(t *testing.T) {
	// Valid key
	key := &APIKey{
		ExpiresAt: time.Now().Add(time.Hour),
		Revoked:   false,
	}
	if !key.IsValid() {
		t.Error("Key should be valid")
	}

	// Revoked key
	key.Revoked = true
	if key.IsValid() {
		t.Error("Revoked key should not be valid")
	}

	// Expired key
	key.Revoked = false
	key.ExpiresAt = time.Now().Add(-time.Hour)
	if key.IsValid() {
		t.Error("Expired key should not be valid")
	}
}

func TestAPIKeyStore_GenerateAPIKey(t *testing.T) {
	store := NewAPIKeyStore()

	rawKey, key, err := store.GenerateAPIKey("test-key", []Permission{PermissionRead}, time.Hour)
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	if rawKey == "" {
		t.Error("Raw key should not be empty")
	}
	if key.Name != "test-key" {
		t.Error("Key name not set correctly")
	}
	if len(key.Permissions) != 1 || key.Permissions[0] != PermissionRead {
		t.Error("Key permissions not set correctly")
	}
	if key.ExpiresAt.IsZero() {
		t.Error("Key expiration should be set")
	}
}

func TestAPIKeyStore_ValidateAPIKey(t *testing.T) {
	store := NewAPIKeyStore()

	rawKey, _, err := store.GenerateAPIKey("test-key", []Permission{PermissionRead}, time.Hour)
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	// Valid key
	key, err := store.ValidateAPIKey(rawKey)
	if err != nil {
		t.Fatalf("ValidateAPIKey failed: %v", err)
	}
	if key.Name != "test-key" {
		t.Error("Key name mismatch")
	}

	// Invalid key
	_, err = store.ValidateAPIKey("invalid-key")
	if err != ErrAPIKeyNotFound {
		t.Errorf("Expected ErrAPIKeyNotFound, got %v", err)
	}
}

func TestAPIKeyStore_RevokeAPIKey(t *testing.T) {
	store := NewAPIKeyStore()

	rawKey, key, _ := store.GenerateAPIKey("test-key", []Permission{PermissionRead}, time.Hour)

	// Revoke
	err := store.RevokeAPIKey(key.ID)
	if err != nil {
		t.Fatalf("RevokeAPIKey failed: %v", err)
	}

	// Validate revoked key
	_, err = store.ValidateAPIKey(rawKey)
	if err != ErrAPIKeyRevoked {
		t.Errorf("Expected ErrAPIKeyRevoked, got %v", err)
	}
}

func TestAPIKeyStore_ListAPIKeys(t *testing.T) {
	store := NewAPIKeyStore()

	store.GenerateAPIKey("key-1", []Permission{PermissionRead}, time.Hour)
	store.GenerateAPIKey("key-2", []Permission{PermissionWrite}, time.Hour)

	keys := store.ListAPIKeys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}
}

func TestAuthenticator_AuthenticateRequest_Disabled(t *testing.T) {
	config := &AuthConfig{
		Enabled: false,
	}
	auth := NewAuthenticator(config)

	req := httptest.NewRequest("GET", "/api/test", nil)
	user, err := auth.AuthenticateRequest(req)

	if err != nil {
		t.Fatalf("AuthenticateRequest failed: %v", err)
	}
	if user.Method != AuthMethodNone {
		t.Errorf("Expected method none, got %s", user.Method)
	}
	if !user.HasPermission(PermissionAdmin) {
		t.Error("Anonymous user should have admin permission when auth disabled")
	}
}

func TestAuthenticator_AuthenticateRequest_APIKey_Header(t *testing.T) {
	config := &AuthConfig{
		Enabled:        true,
		AllowedMethods: []AuthMethod{AuthMethodAPIKey},
		APIKeyHeader:   "X-API-Key",
	}
	auth := NewAuthenticator(config)

	// Generate a key
	rawKey, _, _ := auth.GetKeyStore().GenerateAPIKey("test", []Permission{PermissionRead}, time.Hour)

	// Test with valid key
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", rawKey)

	user, err := auth.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("AuthenticateRequest failed: %v", err)
	}
	if user.Method != AuthMethodAPIKey {
		t.Error("Expected API key method")
	}

	// Test without key
	req = httptest.NewRequest("GET", "/api/test", nil)
	_, err = auth.AuthenticateRequest(req)
	if err == nil {
		t.Error("Should fail without API key")
	}
}

func TestAuthenticator_AuthenticateRequest_APIKey_QueryParam(t *testing.T) {
	config := &AuthConfig{
		Enabled:          true,
		AllowedMethods:   []AuthMethod{AuthMethodAPIKey},
		APIKeyHeader:     "X-API-Key",
		APIKeyQueryParam: "api_key",
	}
	auth := NewAuthenticator(config)

	rawKey, _, _ := auth.GetKeyStore().GenerateAPIKey("test", []Permission{PermissionRead}, time.Hour)

	// Test with query param
	req := httptest.NewRequest("GET", "/api/test?api_key="+rawKey, nil)

	user, err := auth.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("AuthenticateRequest with query param failed: %v", err)
	}
	if user.Method != AuthMethodAPIKey {
		t.Error("Expected API key method")
	}
}

func TestAuthenticator_AuthenticateRequest_APIKey_Bearer(t *testing.T) {
	config := &AuthConfig{
		Enabled:        true,
		AllowedMethods: []AuthMethod{AuthMethodAPIKey},
		APIKeyHeader:   "X-API-Key",
	}
	auth := NewAuthenticator(config)

	rawKey, _, _ := auth.GetKeyStore().GenerateAPIKey("test", []Permission{PermissionRead}, time.Hour)

	// Test with Bearer token
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)

	user, err := auth.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("AuthenticateRequest with Bearer failed: %v", err)
	}
	if user.Method != AuthMethodAPIKey {
		t.Error("Expected API key method")
	}
}

func TestAuthenticator_BasicAuth(t *testing.T) {
	config := &AuthConfig{
		Enabled:        true,
		AllowedMethods: []AuthMethod{AuthMethodBasic},
		BasicAuthRealm: "Test",
	}
	auth := NewAuthenticator(config)

	// Add a user
	auth.AddBasicAuthUser("admin", "secret123", []Permission{PermissionAdmin})

	// Test valid credentials
	req := httptest.NewRequest("GET", "/api/test", nil)
	credentials := base64.StdEncoding.EncodeToString([]byte("admin:secret123"))
	req.Header.Set("Authorization", "Basic "+credentials)

	user, err := auth.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("Basic auth failed: %v", err)
	}
	if user.Method != AuthMethodBasic {
		t.Error("Expected basic auth method")
	}
	if user.ID != "admin" {
		t.Errorf("Expected user ID 'admin', got %s", user.ID)
	}

	// Test invalid password
	credentials = base64.StdEncoding.EncodeToString([]byte("admin:wrong"))
	req.Header.Set("Authorization", "Basic "+credentials)

	_, err = auth.AuthenticateRequest(req)
	if err != ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials, got %v", err)
	}

	// Test unknown user
	credentials = base64.StdEncoding.EncodeToString([]byte("unknown:password"))
	req.Header.Set("Authorization", "Basic "+credentials)

	_, err = auth.AuthenticateRequest(req)
	if err != ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials for unknown user, got %v", err)
	}
}

func TestAuthenticator_RemoveBasicAuthUser(t *testing.T) {
	auth := NewAuthenticator(nil)
	auth.AddBasicAuthUser("testuser", "password", []Permission{PermissionRead})
	auth.RemoveBasicAuthUser("testuser")

	config := &AuthConfig{
		Enabled:        true,
		AllowedMethods: []AuthMethod{AuthMethodBasic},
	}
	auth.config = config

	req := httptest.NewRequest("GET", "/api/test", nil)
	credentials := base64.StdEncoding.EncodeToString([]byte("testuser:password"))
	req.Header.Set("Authorization", "Basic "+credentials)

	_, err := auth.AuthenticateRequest(req)
	if err != ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials after removing user, got %v", err)
	}
}

func TestAuthenticator_MTLSAllowlist(t *testing.T) {
	auth := NewAuthenticator(nil)

	// Add CN
	auth.AddMTLSAllowedCN("client.example.com")

	auth.mtlsMu.RLock()
	_, exists := auth.mtlsAllowList["client.example.com"]
	auth.mtlsMu.RUnlock()

	if !exists {
		t.Error("CN should be in allowlist")
	}

	// Remove CN
	auth.RemoveMTLSAllowedCN("client.example.com")

	auth.mtlsMu.RLock()
	_, exists = auth.mtlsAllowList["client.example.com"]
	auth.mtlsMu.RUnlock()

	if exists {
		t.Error("CN should not be in allowlist after removal")
	}
}

func TestAuthMiddleware(t *testing.T) {
	config := &AuthConfig{
		Enabled:        true,
		AllowedMethods: []AuthMethod{AuthMethodAPIKey},
		APIKeyHeader:   "X-API-Key",
		BasicAuthRealm: "Test",
	}
	auth := NewAuthenticator(config)

	rawKey, _, _ := auth.GetKeyStore().GenerateAPIKey("test", []Permission{PermissionRead}, time.Hour)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetAuthenticatedUser(r.Context())
		if user == nil {
			t.Error("User should be in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := AuthMiddleware(auth)
	wrapped := middleware(handler)

	// Test with valid key
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", rawKey)
	rr := httptest.NewRecorder()

	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Test without key
	req = httptest.NewRequest("GET", "/api/test", nil)
	rr = httptest.NewRecorder()

	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rr.Code)
	}
}

func TestRequirePermission(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := RequirePermission(PermissionAdmin)
	wrapped := middleware(handler)

	// Test with admin permission
	req := httptest.NewRequest("GET", "/api/admin", nil)
	user := &AuthenticatedUser{
		Permissions: []Permission{PermissionAdmin},
	}
	ctx := SetAuthenticatedUser(req.Context(), user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 for admin, got %d", rr.Code)
	}

	// Test without admin permission
	user = &AuthenticatedUser{
		Permissions: []Permission{PermissionRead},
	}
	ctx = SetAuthenticatedUser(req.Context(), user)
	req = httptest.NewRequest("GET", "/api/admin", nil)
	req = req.WithContext(ctx)

	rr = httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 for non-admin, got %d", rr.Code)
	}

	// Test without authentication
	req = httptest.NewRequest("GET", "/api/admin", nil)
	rr = httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 without auth, got %d", rr.Code)
	}
}

func TestGetSetAuthenticatedUser(t *testing.T) {
	ctx := context.Background()

	// Initially nil
	user := GetAuthenticatedUser(ctx)
	if user != nil {
		t.Error("Should return nil for empty context")
	}

	// Set user
	testUser := &AuthenticatedUser{
		ID:   "test",
		Name: "Test User",
	}
	ctx = SetAuthenticatedUser(ctx, testUser)

	// Get user
	user = GetAuthenticatedUser(ctx)
	if user == nil {
		t.Fatal("User should not be nil")
	}
	if user.ID != "test" {
		t.Errorf("Expected ID 'test', got %s", user.ID)
	}
}

func TestAPIKeyStore_GetAPIKey(t *testing.T) {
	store := NewAPIKeyStore()

	_, key, _ := store.GenerateAPIKey("test", []Permission{PermissionRead}, time.Hour)

	// Get existing key
	retrieved, err := store.GetAPIKey(key.ID)
	if err != nil {
		t.Fatalf("GetAPIKey failed: %v", err)
	}
	if retrieved.Name != "test" {
		t.Error("Key name mismatch")
	}

	// Get non-existent key
	_, err = store.GetAPIKey("non-existent")
	if err != ErrAPIKeyNotFound {
		t.Errorf("Expected ErrAPIKeyNotFound, got %v", err)
	}
}

func TestAPIKeyStore_AddAPIKey(t *testing.T) {
	store := NewAPIKeyStore()

	// Create key manually
	hash := sha256.Sum256([]byte("my-secret-key"))
	keyHash := hex.EncodeToString(hash[:])

	key := &APIKey{
		ID:          "custom-id",
		Name:        "custom-key",
		KeyHash:     keyHash,
		Permissions: []Permission{PermissionAdmin},
		CreatedAt:   time.Now(),
	}

	store.AddAPIKey(key)

	// Validate the key
	retrieved, err := store.ValidateAPIKey("my-secret-key")
	if err != nil {
		t.Fatalf("ValidateAPIKey failed: %v", err)
	}
	if retrieved.Name != "custom-key" {
		t.Error("Key name mismatch")
	}
}

func TestCertFingerprint(t *testing.T) {
	// This is a basic test - in production you'd use a real certificate
	// For now, just ensure the function doesn't panic
	// cert fingerprint requires a real x509.Certificate
	t.Skip("Requires real certificate for testing")
}
