package internal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultSecretsManagerConfig(t *testing.T) {
	config := DefaultSecretsManagerConfig()

	if config.MasterKeyEnvVar != "KARL_MASTER_KEY" {
		t.Errorf("Expected MasterKeyEnvVar 'KARL_MASTER_KEY', got %s", config.MasterKeyEnvVar)
	}
}

func TestNewSecretsManager(t *testing.T) {
	sm, err := NewSecretsManager(nil)
	if err != nil {
		t.Fatalf("NewSecretsManager failed: %v", err)
	}

	if sm == nil {
		t.Fatal("SecretsManager is nil")
	}

	// Should be sealed initially
	if !sm.IsSealed() {
		t.Error("SecretsManager should be sealed initially")
	}
}

func TestSecretsManager_UnsealAndSeal(t *testing.T) {
	sm, _ := NewSecretsManager(nil)

	// Unseal with master key
	masterKey := []byte("test-master-key-32-bytes-long!!")
	err := sm.Unseal(masterKey)
	if err != nil {
		t.Fatalf("Unseal failed: %v", err)
	}

	if sm.IsSealed() {
		t.Error("Should not be sealed after Unseal")
	}

	// Seal
	err = sm.Seal()
	if err != nil {
		t.Fatalf("Seal failed: %v", err)
	}

	if !sm.IsSealed() {
		t.Error("Should be sealed after Seal")
	}
}

func TestSecretsManager_UnsealInvalidKey(t *testing.T) {
	sm, _ := NewSecretsManager(nil)

	err := sm.Unseal(nil)
	if err != ErrInvalidMasterKey {
		t.Errorf("Expected ErrInvalidMasterKey, got %v", err)
	}

	err = sm.Unseal([]byte{})
	if err != ErrInvalidMasterKey {
		t.Errorf("Expected ErrInvalidMasterKey for empty key, got %v", err)
	}
}

func TestSecretsManager_SetAndGetSecret(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	// Set a secret
	err := sm.SetSecret("db-password", []byte("super-secret"), SecretTypePassword)
	if err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}

	// Get the secret
	secret, err := sm.GetSecret("db-password")
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}

	if string(secret.Value) != "super-secret" {
		t.Errorf("Expected 'super-secret', got %s", string(secret.Value))
	}
	if secret.Type != SecretTypePassword {
		t.Errorf("Expected type Password, got %s", secret.Type)
	}
	if secret.Version != 1 {
		t.Errorf("Expected version 1, got %d", secret.Version)
	}
}

func TestSecretsManager_GetSecretValue(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	sm.SetSecret("api-key", []byte("my-api-key"), SecretTypeAPIKey)

	value, err := sm.GetSecretValue("api-key")
	if err != nil {
		t.Fatalf("GetSecretValue failed: %v", err)
	}

	if string(value) != "my-api-key" {
		t.Errorf("Expected 'my-api-key', got %s", string(value))
	}
}

func TestSecretsManager_GetSecretString(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	sm.SetSecret("token", []byte("bearer-token-123"), SecretTypeToken)

	value, err := sm.GetSecretString("token")
	if err != nil {
		t.Fatalf("GetSecretString failed: %v", err)
	}

	if value != "bearer-token-123" {
		t.Errorf("Expected 'bearer-token-123', got %s", value)
	}
}

func TestSecretsManager_GetSecretNotFound(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	_, err := sm.GetSecret("nonexistent")
	if err != ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got %v", err)
	}
}

func TestSecretsManager_OperationsWhileSealed(t *testing.T) {
	sm, _ := NewSecretsManager(nil)

	// All operations should fail while sealed
	_, err := sm.GetSecret("test")
	if err != ErrSecretStoreSealed {
		t.Errorf("Expected ErrSecretStoreSealed, got %v", err)
	}

	err = sm.SetSecret("test", []byte("value"), SecretTypeGeneric)
	if err != ErrSecretStoreSealed {
		t.Errorf("Expected ErrSecretStoreSealed, got %v", err)
	}

	err = sm.DeleteSecret("test")
	if err != ErrSecretStoreSealed {
		t.Errorf("Expected ErrSecretStoreSealed, got %v", err)
	}

	_, err = sm.ListSecrets()
	if err != ErrSecretStoreSealed {
		t.Errorf("Expected ErrSecretStoreSealed, got %v", err)
	}
}

func TestSecretsManager_DeleteSecret(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	sm.SetSecret("to-delete", []byte("value"), SecretTypeGeneric)

	err := sm.DeleteSecret("to-delete")
	if err != nil {
		t.Fatalf("DeleteSecret failed: %v", err)
	}

	_, err = sm.GetSecret("to-delete")
	if err != ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound after delete, got %v", err)
	}
}

func TestSecretsManager_DeleteSecretNotFound(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	err := sm.DeleteSecret("nonexistent")
	if err != ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got %v", err)
	}
}

func TestSecretsManager_ListSecrets(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	sm.SetSecret("secret-1", []byte("value1"), SecretTypeGeneric)
	sm.SetSecret("secret-2", []byte("value2"), SecretTypeGeneric)
	sm.SetSecret("secret-3", []byte("value3"), SecretTypeGeneric)

	keys, err := sm.ListSecrets()
	if err != nil {
		t.Fatalf("ListSecrets failed: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}
}

func TestSecretsManager_ListSecretsWithMetadata(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	sm.SetSecret("secret-1", []byte("value1"), SecretTypePassword,
		WithDescription("Test secret"))

	metadata, err := sm.ListSecretsWithMetadata()
	if err != nil {
		t.Fatalf("ListSecretsWithMetadata failed: %v", err)
	}

	if len(metadata) != 1 {
		t.Fatalf("Expected 1 metadata entry, got %d", len(metadata))
	}

	if metadata[0].Description != "Test secret" {
		t.Error("Description not set correctly")
	}
}

func TestSecretsManager_SecretVersioning(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	// First version
	sm.SetSecret("versioned", []byte("v1"), SecretTypeGeneric)

	secret, _ := sm.GetSecret("versioned")
	if secret.Version != 1 {
		t.Errorf("Expected version 1, got %d", secret.Version)
	}

	// Update - version should increment
	sm.SetSecret("versioned", []byte("v2"), SecretTypeGeneric)

	secret, _ = sm.GetSecret("versioned")
	if secret.Version != 2 {
		t.Errorf("Expected version 2, got %d", secret.Version)
	}
	if string(secret.Value) != "v2" {
		t.Error("Value not updated")
	}
}

func TestSecretsManager_SecretExpiration(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	// Create an expired secret
	sm.SetSecret("expired", []byte("old-value"), SecretTypeGeneric,
		WithExpiration(time.Now().Add(-time.Hour)))

	_, err := sm.GetSecret("expired")
	if err != ErrSecretExpired {
		t.Errorf("Expected ErrSecretExpired, got %v", err)
	}

	// Create a non-expired secret
	sm.SetSecret("valid", []byte("valid-value"), SecretTypeGeneric,
		WithExpiration(time.Now().Add(time.Hour)))

	secret, err := sm.GetSecret("valid")
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if string(secret.Value) != "valid-value" {
		t.Error("Value mismatch")
	}
}

func TestSecretsManager_SecretOptions(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	sm.SetSecret("with-options", []byte("value"), SecretTypeGeneric,
		WithDescription("A test secret"),
		WithTTL(24*time.Hour),
		WithMetadata("env", "production"),
		WithMetadata("owner", "test-team"))

	secret, _ := sm.GetSecret("with-options")

	if secret.Description != "A test secret" {
		t.Error("Description not set")
	}
	if secret.ExpiresAt.IsZero() {
		t.Error("ExpiresAt not set")
	}
	if secret.Metadata["env"] != "production" {
		t.Error("Metadata env not set")
	}
	if secret.Metadata["owner"] != "test-team" {
		t.Error("Metadata owner not set")
	}
}

func TestSecretsManager_RotateSecret(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	var callbackCalled bool
	sm.config.RotationCallback = func(key string, old, new *Secret) {
		callbackCalled = true
		if string(old.Value) != "old-value" {
			t.Error("Old value mismatch in callback")
		}
		if string(new.Value) != "new-value" {
			t.Error("New value mismatch in callback")
		}
	}

	sm.SetSecret("to-rotate", []byte("old-value"), SecretTypePassword)

	err := sm.RotateSecret("to-rotate", []byte("new-value"))
	if err != nil {
		t.Fatalf("RotateSecret failed: %v", err)
	}

	secret, _ := sm.GetSecret("to-rotate")
	if string(secret.Value) != "new-value" {
		t.Error("Value not rotated")
	}
	if secret.Version != 2 {
		t.Errorf("Expected version 2 after rotation, got %d", secret.Version)
	}
	if !callbackCalled {
		t.Error("Rotation callback not called")
	}
}

func TestSecretsManager_RotateSecretNotFound(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	err := sm.RotateSecret("nonexistent", []byte("value"))
	if err != ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got %v", err)
	}
}

func TestSecretsManager_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "secrets.json")

	// Create and populate
	config := &SecretsManagerConfig{StorePath: storePath}
	sm1, _ := NewSecretsManager(config)
	sm1.Unseal([]byte("test-master-key-32-bytes-long!!"))

	sm1.SetSecret("persistent", []byte("stored-value"), SecretTypeGeneric)
	sm1.Seal()

	// Reload
	sm2, _ := NewSecretsManager(config)
	sm2.Unseal([]byte("test-master-key-32-bytes-long!!"))

	secret, err := sm2.GetSecret("persistent")
	if err != nil {
		t.Fatalf("GetSecret after reload failed: %v", err)
	}
	if string(secret.Value) != "stored-value" {
		t.Error("Value not persisted correctly")
	}
}

func TestSecretsManager_UnsealFromEnv(t *testing.T) {
	os.Setenv("KARL_MASTER_KEY", "test-master-key-32-bytes-long!!")
	defer os.Unsetenv("KARL_MASTER_KEY")

	sm, _ := NewSecretsManager(nil)

	err := sm.UnsealFromEnv()
	if err != nil {
		t.Fatalf("UnsealFromEnv failed: %v", err)
	}

	if sm.IsSealed() {
		t.Error("Should be unsealed")
	}
}

func TestSecretsManager_UnsealFromEnvMissing(t *testing.T) {
	os.Unsetenv("KARL_MASTER_KEY")

	sm, _ := NewSecretsManager(nil)

	err := sm.UnsealFromEnv()
	if err == nil {
		t.Error("Should fail when env var not set")
	}
}

func TestSecretsManager_UnsealFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "master.key")

	os.WriteFile(keyFile, []byte("test-master-key-32-bytes-long!!\n"), 0600)

	sm, _ := NewSecretsManager(nil)

	err := sm.UnsealFromFile(keyFile)
	if err != nil {
		t.Fatalf("UnsealFromFile failed: %v", err)
	}

	if sm.IsSealed() {
		t.Error("Should be unsealed")
	}
}

func TestSecretsManager_UnsealFromFileMissing(t *testing.T) {
	sm, _ := NewSecretsManager(nil)

	err := sm.UnsealFromFile("/nonexistent/master.key")
	if err == nil {
		t.Error("Should fail for missing file")
	}
}

func TestSecret_IsExpired(t *testing.T) {
	// Not expired
	s := &Secret{ExpiresAt: time.Now().Add(time.Hour)}
	if s.IsExpired() {
		t.Error("Should not be expired")
	}

	// Expired
	s.ExpiresAt = time.Now().Add(-time.Hour)
	if !s.IsExpired() {
		t.Error("Should be expired")
	}

	// No expiration
	s.ExpiresAt = time.Time{}
	if s.IsExpired() {
		t.Error("Should not be expired when no expiration set")
	}
}

func TestGenerateRandomSecret(t *testing.T) {
	secret1, err := GenerateRandomSecret(32)
	if err != nil {
		t.Fatalf("GenerateRandomSecret failed: %v", err)
	}
	if len(secret1) != 32 {
		t.Errorf("Expected length 32, got %d", len(secret1))
	}

	secret2, _ := GenerateRandomSecret(32)
	if string(secret1) == string(secret2) {
		t.Error("Secrets should be unique")
	}
}

func TestGenerateRandomSecretString(t *testing.T) {
	secret, err := GenerateRandomSecretString(32)
	if err != nil {
		t.Fatalf("GenerateRandomSecretString failed: %v", err)
	}
	if secret == "" {
		t.Error("Secret should not be empty")
	}
}

func TestEnvironmentSecretProvider(t *testing.T) {
	os.Setenv("KARL_DB_PASSWORD", "secret123")
	defer os.Unsetenv("KARL_DB_PASSWORD")

	provider := NewEnvironmentSecretProvider("KARL_")

	value, err := provider.GetSecret("db-password")
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if value != "secret123" {
		t.Errorf("Expected 'secret123', got %s", value)
	}

	// Missing secret
	_, err = provider.GetSecret("missing")
	if err == nil {
		t.Error("Should fail for missing env var")
	}
}

func TestFileSecretProvider(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "api-key"), []byte("my-api-key\n"), 0600)

	provider := NewFileSecretProvider(tmpDir)

	value, err := provider.GetSecret("api-key")
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if value != "my-api-key" {
		t.Errorf("Expected 'my-api-key', got %s", value)
	}

	// Missing file
	_, err = provider.GetSecret("missing")
	if err == nil {
		t.Error("Should fail for missing file")
	}
}

func TestParseSecretReference(t *testing.T) {
	tests := []struct {
		input    string
		refType  string
		key      string
		provider string
		hasError bool
	}{
		{"env:MY_SECRET", "env", "MY_SECRET", "", false},
		{"file:/path/to/secret", "file", "/path/to/secret", "", false},
		{"manager:db-password", "manager", "db-password", "", false},
		{"vault:prod:api-key", "vault", "api-key", "prod", false},
		{"invalid", "", "", "", true},
	}

	for _, tt := range tests {
		ref, err := ParseSecretReference(tt.input)
		if tt.hasError {
			if err == nil {
				t.Errorf("ParseSecretReference(%q) should return error", tt.input)
			}
			continue
		}

		if err != nil {
			t.Errorf("ParseSecretReference(%q) returned error: %v", tt.input, err)
			continue
		}

		if ref.Type != tt.refType {
			t.Errorf("Type = %s, expected %s", ref.Type, tt.refType)
		}
		if ref.Key != tt.key {
			t.Errorf("Key = %s, expected %s", ref.Key, tt.key)
		}
		if ref.Provider != tt.provider {
			t.Errorf("Provider = %s, expected %s", ref.Provider, tt.provider)
		}
	}
}

func TestResolveSecretReference_Env(t *testing.T) {
	os.Setenv("TEST_SECRET", "env-value")
	defer os.Unsetenv("TEST_SECRET")

	ref := &SecretReference{Type: "env", Key: "TEST_SECRET"}

	value, err := ResolveSecretReference(context.Background(), ref, nil)
	if err != nil {
		t.Fatalf("ResolveSecretReference failed: %v", err)
	}
	if value != "env-value" {
		t.Errorf("Expected 'env-value', got %s", value)
	}
}

func TestResolveSecretReference_File(t *testing.T) {
	tmpDir := t.TempDir()
	secretFile := filepath.Join(tmpDir, "secret")
	os.WriteFile(secretFile, []byte("file-value\n"), 0600)

	ref := &SecretReference{Type: "file", Key: secretFile}

	value, err := ResolveSecretReference(context.Background(), ref, nil)
	if err != nil {
		t.Fatalf("ResolveSecretReference failed: %v", err)
	}
	if value != "file-value" {
		t.Errorf("Expected 'file-value', got %s", value)
	}
}

func TestResolveSecretReference_Manager(t *testing.T) {
	sm := createUnsealedManager(t)
	defer sm.Seal()

	sm.SetSecret("managed", []byte("managed-value"), SecretTypeGeneric)

	ref := &SecretReference{Type: "manager", Key: "managed"}

	value, err := ResolveSecretReference(context.Background(), ref, sm)
	if err != nil {
		t.Fatalf("ResolveSecretReference failed: %v", err)
	}
	if value != "managed-value" {
		t.Errorf("Expected 'managed-value', got %s", value)
	}
}

func TestResolveSecretReference_UnknownType(t *testing.T) {
	ref := &SecretReference{Type: "unknown", Key: "test"}

	_, err := ResolveSecretReference(context.Background(), ref, nil)
	if err == nil {
		t.Error("Should fail for unknown type")
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ab", "****"},
		{"abc", "****"},
		{"abcd", "****"},
		{"abcde", "ab*de"},
		{"password123", "pa*******23"},
	}

	for _, tt := range tests {
		result := MaskSecret(tt.input)
		if result != tt.expected {
			t.Errorf("MaskSecret(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestSecureWipe(t *testing.T) {
	data := []byte("sensitive data")
	SecureWipe(data)

	for _, b := range data {
		if b != 0 {
			t.Error("Data should be wiped to zeros")
			break
		}
	}
}

func TestSecretsManager_AuditLogging(t *testing.T) {
	var events []*SecretAuditEvent

	config := &SecretsManagerConfig{
		AuditLog: func(e *SecretAuditEvent) {
			events = append(events, e)
		},
	}

	sm, _ := NewSecretsManager(config)
	sm.Unseal([]byte("test-master-key-32-bytes-long!!"))
	sm.SetSecret("audited", []byte("value"), SecretTypeGeneric)
	sm.GetSecret("audited")
	sm.DeleteSecret("audited")
	sm.Seal()

	// Should have: unseal, set, get, delete, seal
	if len(events) < 5 {
		t.Errorf("Expected at least 5 audit events, got %d", len(events))
	}

	// Verify some events
	actions := make(map[string]bool)
	for _, e := range events {
		actions[e.Action] = true
	}

	if !actions["unseal"] {
		t.Error("Missing unseal audit event")
	}
	if !actions["set"] {
		t.Error("Missing set audit event")
	}
	if !actions["get"] {
		t.Error("Missing get audit event")
	}
}

// Helper function
func createUnsealedManager(t *testing.T) *SecretsManager {
	t.Helper()
	sm, err := NewSecretsManager(nil)
	if err != nil {
		t.Fatalf("Failed to create secrets manager: %v", err)
	}

	if err := sm.Unseal([]byte("test-master-key-32-bytes-long!!")); err != nil {
		t.Fatalf("Failed to unseal: %v", err)
	}

	return sm
}
