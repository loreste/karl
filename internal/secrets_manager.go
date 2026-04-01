package internal

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Secrets management errors
var (
	ErrSecretNotFound     = errors.New("secret not found")
	ErrSecretExpired      = errors.New("secret has expired")
	ErrInvalidSecretKey   = errors.New("invalid secret key")
	ErrEncryptionFailed   = errors.New("encryption failed")
	ErrDecryptionFailed   = errors.New("decryption failed")
	ErrSecretStoreSealed  = errors.New("secret store is sealed")
	ErrInvalidMasterKey   = errors.New("invalid master key")
)

// SecretType represents the type of secret
type SecretType string

const (
	SecretTypePassword    SecretType = "password"
	SecretTypeAPIKey      SecretType = "api_key"
	SecretTypeCertificate SecretType = "certificate"
	SecretTypePrivateKey  SecretType = "private_key"
	SecretTypeToken       SecretType = "token"
	SecretTypeGeneric     SecretType = "generic"
)

// Secret represents a stored secret
type Secret struct {
	Key         string            `json:"key"`
	Value       []byte            `json:"-"` // Never serialized directly
	Type        SecretType        `json:"type"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ExpiresAt   time.Time         `json:"expires_at,omitempty"`
	Version     int               `json:"version"`

	// Encrypted value for storage
	EncryptedValue []byte `json:"encrypted_value,omitempty"`
}

// IsExpired checks if the secret has expired
func (s *Secret) IsExpired() bool {
	if s.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(s.ExpiresAt)
}

// SecretsManagerConfig holds configuration for the secrets manager
type SecretsManagerConfig struct {
	// Storage backend
	StorePath string

	// Encryption settings
	MasterKeyEnvVar string // Environment variable for master key
	MasterKeyFile   string // File containing master key

	// Auto-rotation
	AutoRotateInterval time.Duration
	RotationCallback   func(key string, oldSecret, newSecret *Secret)

	// Audit logging
	AuditLog func(event *SecretAuditEvent)
}

// DefaultSecretsManagerConfig returns default configuration
func DefaultSecretsManagerConfig() *SecretsManagerConfig {
	return &SecretsManagerConfig{
		StorePath:       "",
		MasterKeyEnvVar: "KARL_MASTER_KEY",
	}
}

// SecretsManager manages secrets securely
type SecretsManager struct {
	config *SecretsManagerConfig

	mu       sync.RWMutex
	secrets  map[string]*Secret
	sealed   bool
	gcm      cipher.AEAD
	masterKey []byte

	// Rotation
	stopRotation chan struct{}
	rotationDone chan struct{}
}

// NewSecretsManager creates a new secrets manager
func NewSecretsManager(config *SecretsManagerConfig) (*SecretsManager, error) {
	if config == nil {
		config = DefaultSecretsManagerConfig()
	}

	sm := &SecretsManager{
		config:       config,
		secrets:      make(map[string]*Secret),
		sealed:       true,
		stopRotation: make(chan struct{}),
		rotationDone: make(chan struct{}),
	}

	return sm, nil
}

// Unseal unseals the secrets manager with the master key
func (sm *SecretsManager) Unseal(masterKey []byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if len(masterKey) == 0 {
		return ErrInvalidMasterKey
	}

	// Derive a 32-byte key using SHA-256
	hash := sha256.Sum256(masterKey)
	key := hash[:]

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	sm.gcm = gcm
	sm.masterKey = key
	sm.sealed = false

	// Load secrets from storage if configured
	if sm.config.StorePath != "" {
		if err := sm.loadFromStorage(); err != nil {
			// Log but don't fail - storage might not exist yet
			sm.logAudit("load_failed", "", err.Error())
		}
	}

	sm.logAudit("unseal", "", "secrets manager unsealed")

	return nil
}

// UnsealFromEnv unseals using the master key from environment
func (sm *SecretsManager) UnsealFromEnv() error {
	key := os.Getenv(sm.config.MasterKeyEnvVar)
	if key == "" {
		return fmt.Errorf("master key not found in environment variable %s", sm.config.MasterKeyEnvVar)
	}

	return sm.Unseal([]byte(key))
}

// UnsealFromFile unseals using the master key from a file
func (sm *SecretsManager) UnsealFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read master key file: %w", err)
	}

	// Trim whitespace
	key := strings.TrimSpace(string(data))

	return sm.Unseal([]byte(key))
}

// Seal seals the secrets manager
func (sm *SecretsManager) Seal() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Clear sensitive data
	sm.gcm = nil
	for i := range sm.masterKey {
		sm.masterKey[i] = 0
	}
	sm.masterKey = nil
	sm.sealed = true

	// Clear decrypted values
	for _, secret := range sm.secrets {
		for i := range secret.Value {
			secret.Value[i] = 0
		}
		secret.Value = nil
	}

	sm.logAudit("seal", "", "secrets manager sealed")

	return nil
}

// IsSealed returns whether the manager is sealed
func (sm *SecretsManager) IsSealed() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sealed
}

// SetSecret stores a secret
func (sm *SecretsManager) SetSecret(key string, value []byte, secretType SecretType, options ...SecretOption) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.sealed {
		return ErrSecretStoreSealed
	}

	if key == "" {
		return ErrInvalidSecretKey
	}

	now := time.Now()
	version := 1

	// Check if updating existing secret
	if existing, exists := sm.secrets[key]; exists {
		version = existing.Version + 1
	}

	secret := &Secret{
		Key:       key,
		Value:     value,
		Type:      secretType,
		CreatedAt: now,
		UpdatedAt: now,
		Version:   version,
		Metadata:  make(map[string]string),
	}

	// Apply options
	for _, opt := range options {
		opt(secret)
	}

	// Encrypt the value
	encrypted, err := sm.encrypt(value)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}
	secret.EncryptedValue = encrypted

	sm.secrets[key] = secret

	// Persist to storage
	if sm.config.StorePath != "" {
		if err := sm.saveToStorage(); err != nil {
			return fmt.Errorf("failed to persist secret: %w", err)
		}
	}

	sm.logAudit("set", key, fmt.Sprintf("secret set (version %d)", version))

	return nil
}

// GetSecret retrieves a secret
func (sm *SecretsManager) GetSecret(key string) (*Secret, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.sealed {
		return nil, ErrSecretStoreSealed
	}

	secret, exists := sm.secrets[key]
	if !exists {
		return nil, ErrSecretNotFound
	}

	if secret.IsExpired() {
		sm.logAudit("get_expired", key, "attempted to access expired secret")
		return nil, ErrSecretExpired
	}

	// Decrypt if needed
	if secret.Value == nil && secret.EncryptedValue != nil {
		decrypted, err := sm.decrypt(secret.EncryptedValue)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
		}
		secret.Value = decrypted
	}

	sm.logAudit("get", key, "secret accessed")

	return secret, nil
}

// GetSecretValue is a convenience method to get just the value
func (sm *SecretsManager) GetSecretValue(key string) ([]byte, error) {
	secret, err := sm.GetSecret(key)
	if err != nil {
		return nil, err
	}
	return secret.Value, nil
}

// GetSecretString is a convenience method to get the value as a string
func (sm *SecretsManager) GetSecretString(key string) (string, error) {
	value, err := sm.GetSecretValue(key)
	if err != nil {
		return "", err
	}
	return string(value), nil
}

// DeleteSecret removes a secret
func (sm *SecretsManager) DeleteSecret(key string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.sealed {
		return ErrSecretStoreSealed
	}

	secret, exists := sm.secrets[key]
	if !exists {
		return ErrSecretNotFound
	}

	// Clear sensitive data
	for i := range secret.Value {
		secret.Value[i] = 0
	}

	delete(sm.secrets, key)

	// Persist to storage
	if sm.config.StorePath != "" {
		if err := sm.saveToStorage(); err != nil {
			return fmt.Errorf("failed to persist deletion: %w", err)
		}
	}

	sm.logAudit("delete", key, "secret deleted")

	return nil
}

// ListSecrets returns all secret keys (not values)
func (sm *SecretsManager) ListSecrets() ([]string, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.sealed {
		return nil, ErrSecretStoreSealed
	}

	keys := make([]string, 0, len(sm.secrets))
	for key := range sm.secrets {
		keys = append(keys, key)
	}

	return keys, nil
}

// ListSecretsWithMetadata returns secret metadata (not values)
func (sm *SecretsManager) ListSecretsWithMetadata() ([]*SecretMetadata, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.sealed {
		return nil, ErrSecretStoreSealed
	}

	result := make([]*SecretMetadata, 0, len(sm.secrets))
	for _, secret := range sm.secrets {
		result = append(result, &SecretMetadata{
			Key:         secret.Key,
			Type:        secret.Type,
			Description: secret.Description,
			CreatedAt:   secret.CreatedAt,
			UpdatedAt:   secret.UpdatedAt,
			ExpiresAt:   secret.ExpiresAt,
			Version:     secret.Version,
			IsExpired:   secret.IsExpired(),
		})
	}

	return result, nil
}

// SecretMetadata holds non-sensitive secret information
type SecretMetadata struct {
	Key         string
	Type        SecretType
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ExpiresAt   time.Time
	Version     int
	IsExpired   bool
}

// RotateSecret rotates a secret with a new value
func (sm *SecretsManager) RotateSecret(key string, newValue []byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.sealed {
		return ErrSecretStoreSealed
	}

	oldSecret, exists := sm.secrets[key]
	if !exists {
		return ErrSecretNotFound
	}

	// Create new version
	newSecret := &Secret{
		Key:         key,
		Value:       newValue,
		Type:        oldSecret.Type,
		Description: oldSecret.Description,
		Metadata:    oldSecret.Metadata,
		CreatedAt:   oldSecret.CreatedAt,
		UpdatedAt:   time.Now(),
		ExpiresAt:   oldSecret.ExpiresAt,
		Version:     oldSecret.Version + 1,
	}

	// Encrypt
	encrypted, err := sm.encrypt(newValue)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}
	newSecret.EncryptedValue = encrypted

	sm.secrets[key] = newSecret

	// Persist
	if sm.config.StorePath != "" {
		if err := sm.saveToStorage(); err != nil {
			return fmt.Errorf("failed to persist rotation: %w", err)
		}
	}

	// Callback
	if sm.config.RotationCallback != nil {
		sm.config.RotationCallback(key, oldSecret, newSecret)
	}

	sm.logAudit("rotate", key, fmt.Sprintf("secret rotated to version %d", newSecret.Version))

	return nil
}

// encrypt encrypts data using AES-GCM
func (sm *SecretsManager) encrypt(plaintext []byte) ([]byte, error) {
	if sm.gcm == nil {
		return nil, errors.New("cipher not initialized")
	}

	nonce := make([]byte, sm.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := sm.gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decrypt decrypts data using AES-GCM
func (sm *SecretsManager) decrypt(ciphertext []byte) ([]byte, error) {
	if sm.gcm == nil {
		return nil, errors.New("cipher not initialized")
	}

	nonceSize := sm.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := sm.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// saveToStorage persists secrets to disk
func (sm *SecretsManager) saveToStorage() error {
	if sm.config.StorePath == "" {
		return nil
	}

	// Create directory if needed
	dir := filepath.Dir(sm.config.StorePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Serialize secrets (without decrypted values)
	data, err := json.MarshalIndent(sm.secrets, "", "  ")
	if err != nil {
		return err
	}

	// Write atomically
	tmpFile := sm.config.StorePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}

	return os.Rename(tmpFile, sm.config.StorePath)
}

// loadFromStorage loads secrets from disk
func (sm *SecretsManager) loadFromStorage() error {
	if sm.config.StorePath == "" {
		return nil
	}

	data, err := os.ReadFile(sm.config.StorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No existing storage
		}
		return err
	}

	var secrets map[string]*Secret
	if err := json.Unmarshal(data, &secrets); err != nil {
		return err
	}

	sm.secrets = secrets
	return nil
}

// logAudit logs an audit event
func (sm *SecretsManager) logAudit(action, key, message string) {
	if sm.config.AuditLog == nil {
		return
	}

	sm.config.AuditLog(&SecretAuditEvent{
		Timestamp: time.Now(),
		Action:    action,
		Key:       key,
		Message:   message,
	})
}

// SecretAuditEvent represents an audit event for secrets
type SecretAuditEvent struct {
	Timestamp time.Time
	Action    string
	Key       string
	Message   string
}

// SecretOption is a functional option for setting secret properties
type SecretOption func(*Secret)

// WithDescription sets the secret description
func WithDescription(desc string) SecretOption {
	return func(s *Secret) {
		s.Description = desc
	}
}

// WithExpiration sets the secret expiration
func WithExpiration(expiresAt time.Time) SecretOption {
	return func(s *Secret) {
		s.ExpiresAt = expiresAt
	}
}

// WithTTL sets the secret TTL
func WithTTL(ttl time.Duration) SecretOption {
	return func(s *Secret) {
		s.ExpiresAt = time.Now().Add(ttl)
	}
}

// WithMetadata sets secret metadata
func WithMetadata(key, value string) SecretOption {
	return func(s *Secret) {
		if s.Metadata == nil {
			s.Metadata = make(map[string]string)
		}
		s.Metadata[key] = value
	}
}

// GenerateRandomSecret generates a random secret value
func GenerateRandomSecret(length int) ([]byte, error) {
	secret := make([]byte, length)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	return secret, nil
}

// GenerateRandomSecretString generates a random secret as base64 string
func GenerateRandomSecretString(length int) (string, error) {
	secret, err := GenerateRandomSecret(length)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(secret), nil
}

// EnvironmentSecretProvider provides secrets from environment variables
type EnvironmentSecretProvider struct {
	prefix string
}

// NewEnvironmentSecretProvider creates a new environment secret provider
func NewEnvironmentSecretProvider(prefix string) *EnvironmentSecretProvider {
	return &EnvironmentSecretProvider{prefix: prefix}
}

// GetSecret gets a secret from environment
func (p *EnvironmentSecretProvider) GetSecret(key string) (string, error) {
	envKey := p.prefix + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
	value := os.Getenv(envKey)
	if value == "" {
		return "", fmt.Errorf("environment variable %s not set", envKey)
	}
	return value, nil
}

// FileSecretProvider provides secrets from files
type FileSecretProvider struct {
	basePath string
}

// NewFileSecretProvider creates a new file secret provider
func NewFileSecretProvider(basePath string) *FileSecretProvider {
	return &FileSecretProvider{basePath: basePath}
}

// GetSecret gets a secret from a file
func (p *FileSecretProvider) GetSecret(key string) (string, error) {
	path := filepath.Join(p.basePath, key)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read secret file %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// SecretReference represents a reference to a secret
type SecretReference struct {
	Type     string // "env", "file", "vault", "manager"
	Key      string
	Provider string // Optional provider name
}

// ParseSecretReference parses a secret reference string
// Format: type:key or type:provider:key
func ParseSecretReference(ref string) (*SecretReference, error) {
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid secret reference: %s", ref)
	}

	sr := &SecretReference{
		Type: parts[0],
	}

	if len(parts) == 2 {
		sr.Key = parts[1]
	} else {
		sr.Provider = parts[1]
		sr.Key = parts[2]
	}

	return sr, nil
}

// ResolveSecretReference resolves a secret reference to its value
func ResolveSecretReference(ctx context.Context, ref *SecretReference, manager *SecretsManager) (string, error) {
	switch ref.Type {
	case "env":
		value := os.Getenv(ref.Key)
		if value == "" {
			return "", fmt.Errorf("environment variable %s not set", ref.Key)
		}
		return value, nil

	case "file":
		data, err := os.ReadFile(ref.Key)
		if err != nil {
			return "", fmt.Errorf("failed to read secret file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil

	case "manager":
		if manager == nil {
			return "", errors.New("secrets manager not available")
		}
		return manager.GetSecretString(ref.Key)

	default:
		return "", fmt.Errorf("unknown secret reference type: %s", ref.Type)
	}
}

// MaskSecret masks a secret value for logging
func MaskSecret(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}

// SecureWipe securely wipes a byte slice
func SecureWipe(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
