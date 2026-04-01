package internal

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ReloadableConfig defines the interface for reloadable configuration
type ReloadableConfig interface {
	// Validate checks if the configuration is valid
	Validate() error
	// Hash returns a hash of the configuration for change detection
	Hash() string
}

// ConfigChangeHandler is called when configuration changes
type ConfigChangeHandler func(oldConfig, newConfig interface{}) error

// errorWrapper wraps an error for atomic.Value storage (atomic.Value cannot store nil)
type errorWrapper struct {
	err error
}

// HotReloadConfig holds configuration for the hot reload system
type HotReloadConfig struct {
	ConfigPath      string
	PollInterval    time.Duration
	ValidateOnLoad  bool
	SignalReload    bool
	BackupOnReload  bool
	MaxBackups      int
}

// DefaultHotReloadConfig returns sensible defaults
func DefaultHotReloadConfig() *HotReloadConfig {
	return &HotReloadConfig{
		ConfigPath:     "",
		PollInterval:   30 * time.Second,
		ValidateOnLoad: true,
		SignalReload:   true,
		BackupOnReload: true,
		MaxBackups:     5,
	}
}

// HotReloader manages configuration hot reloading
type HotReloader struct {
	config *HotReloadConfig

	// Current configuration state
	currentConfig atomic.Value
	currentHash   atomic.Value
	configMu      sync.RWMutex

	// Handlers
	handlers   map[string]ConfigChangeHandler
	handlersMu sync.RWMutex

	// Loader
	loader ConfigLoader

	// State
	reloadCount  atomic.Int64
	lastReload   atomic.Value // time.Time
	lastError    atomic.Value // error
	isRunning    atomic.Bool

	// Channels
	stopCh   chan struct{}
	reloadCh chan struct{}

	// Callbacks
	onReloadStart   func()
	onReloadSuccess func()
	onReloadError   func(error)
}

// ConfigLoader loads configuration from a source
type ConfigLoader interface {
	Load() (interface{}, error)
	Hash() (string, error)
}

// FileConfigLoader loads configuration from a file
type FileConfigLoader struct {
	path       string
	parser     func([]byte) (interface{}, error)
}

// NewFileConfigLoader creates a new file config loader
func NewFileConfigLoader(path string, parser func([]byte) (interface{}, error)) *FileConfigLoader {
	return &FileConfigLoader{
		path:   path,
		parser: parser,
	}
}

// Load loads configuration from the file
func (l *FileConfigLoader) Load() (interface{}, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config, err := l.parser(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return config, nil
}

// Hash returns the MD5 hash of the file contents
func (l *FileConfigLoader) Hash() (string, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return "", err
	}

	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

// NewHotReloader creates a new hot reloader
func NewHotReloader(config *HotReloadConfig, loader ConfigLoader) *HotReloader {
	if config == nil {
		config = DefaultHotReloadConfig()
	}

	hr := &HotReloader{
		config:   config,
		handlers: make(map[string]ConfigChangeHandler),
		loader:   loader,
		stopCh:   make(chan struct{}),
		reloadCh: make(chan struct{}, 1),
	}

	hr.lastReload.Store(time.Time{})
	// Note: atomic.Value cannot store nil, so we use a wrapper type
	hr.lastError.Store(errorWrapper{})

	return hr
}

// SetCallbacks sets reload callbacks
func (hr *HotReloader) SetCallbacks(onStart, onSuccess func(), onError func(error)) {
	hr.onReloadStart = onStart
	hr.onReloadSuccess = onSuccess
	hr.onReloadError = onError
}

// RegisterHandler registers a configuration change handler
func (hr *HotReloader) RegisterHandler(name string, handler ConfigChangeHandler) {
	hr.handlersMu.Lock()
	defer hr.handlersMu.Unlock()
	hr.handlers[name] = handler
}

// UnregisterHandler removes a configuration change handler
func (hr *HotReloader) UnregisterHandler(name string) {
	hr.handlersMu.Lock()
	defer hr.handlersMu.Unlock()
	delete(hr.handlers, name)
}

// LoadInitial loads the initial configuration
func (hr *HotReloader) LoadInitial() error {
	config, err := hr.loader.Load()
	if err != nil {
		return fmt.Errorf("failed to load initial config: %w", err)
	}

	hash, err := hr.loader.Hash()
	if err != nil {
		return fmt.Errorf("failed to hash initial config: %w", err)
	}

	hr.currentConfig.Store(config)
	hr.currentHash.Store(hash)
	hr.lastReload.Store(time.Now())

	return nil
}

// GetConfig returns the current configuration
func (hr *HotReloader) GetConfig() interface{} {
	return hr.currentConfig.Load()
}

// Start starts the hot reload watcher
func (hr *HotReloader) Start(ctx context.Context) error {
	if hr.isRunning.Load() {
		return errors.New("hot reloader already running")
	}

	hr.isRunning.Store(true)

	// Set up signal handler if enabled
	if hr.config.SignalReload {
		go hr.signalHandler(ctx)
	}

	// Start polling if configured
	if hr.config.PollInterval > 0 {
		go hr.pollLoop(ctx)
	}

	return nil
}

// Stop stops the hot reload watcher
func (hr *HotReloader) Stop() {
	if hr.isRunning.CompareAndSwap(true, false) {
		close(hr.stopCh)
	}
}

// TriggerReload manually triggers a reload
func (hr *HotReloader) TriggerReload() {
	select {
	case hr.reloadCh <- struct{}{}:
	default:
		// Channel full, reload already pending
	}
}

// pollLoop polls for configuration changes
func (hr *HotReloader) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(hr.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hr.stopCh:
			return
		case <-ticker.C:
			hr.checkAndReload()
		case <-hr.reloadCh:
			hr.checkAndReload()
		}
	}
}

// signalHandler handles SIGHUP for reload
func (hr *HotReloader) signalHandler(ctx context.Context) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-hr.stopCh:
			return
		case <-sigCh:
			hr.TriggerReload()
		}
	}
}

// checkAndReload checks for changes and reloads if needed
func (hr *HotReloader) checkAndReload() {
	// Check if configuration has changed
	newHash, err := hr.loader.Hash()
	if err != nil {
		hr.lastError.Store(errorWrapper{err: err})
		if hr.onReloadError != nil {
			hr.onReloadError(err)
		}
		return
	}

	currentHash, _ := hr.currentHash.Load().(string)
	if newHash == currentHash {
		return // No change
	}

	// Configuration has changed, reload
	hr.reload()
}

// reload performs the actual reload
func (hr *HotReloader) reload() {
	if hr.onReloadStart != nil {
		hr.onReloadStart()
	}

	// Load new configuration
	newConfig, err := hr.loader.Load()
	if err != nil {
		hr.lastError.Store(errorWrapper{err: err})
		if hr.onReloadError != nil {
			hr.onReloadError(err)
		}
		return
	}

	// Validate if enabled
	if hr.config.ValidateOnLoad {
		if validator, ok := newConfig.(ReloadableConfig); ok {
			if err := validator.Validate(); err != nil {
				hr.lastError.Store(errorWrapper{err: err})
				if hr.onReloadError != nil {
					hr.onReloadError(fmt.Errorf("validation failed: %w", err))
				}
				return
			}
		}
	}

	// Get current config for handlers
	oldConfig := hr.currentConfig.Load()

	// Notify handlers
	hr.handlersMu.RLock()
	handlers := make(map[string]ConfigChangeHandler, len(hr.handlers))
	for k, v := range hr.handlers {
		handlers[k] = v
	}
	hr.handlersMu.RUnlock()

	for name, handler := range handlers {
		if err := handler(oldConfig, newConfig); err != nil {
			hr.lastError.Store(errorWrapper{err: fmt.Errorf("handler %s failed: %w", name, err)})
			if hr.onReloadError != nil {
				hr.onReloadError(err)
			}
			// Continue with other handlers
		}
	}

	// Update current config
	newHash, _ := hr.loader.Hash()
	hr.currentConfig.Store(newConfig)
	hr.currentHash.Store(newHash)
	hr.lastReload.Store(time.Now())
	hr.lastError.Store(errorWrapper{}) // Clear error
	hr.reloadCount.Add(1)

	if hr.onReloadSuccess != nil {
		hr.onReloadSuccess()
	}
}

// GetStats returns hot reload statistics
func (hr *HotReloader) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"is_running":   hr.isRunning.Load(),
		"reload_count": hr.reloadCount.Load(),
	}

	if t, ok := hr.lastReload.Load().(time.Time); ok && !t.IsZero() {
		stats["last_reload"] = t.Format(time.RFC3339)
		stats["since_last_reload"] = time.Since(t).String()
	}

	if wrapper, ok := hr.lastError.Load().(errorWrapper); ok && wrapper.err != nil {
		stats["last_error"] = wrapper.err.Error()
	}

	if hash, ok := hr.currentHash.Load().(string); ok {
		stats["config_hash"] = hash
	}

	return stats
}

// ReloadableValue wraps a value that can be atomically updated
type ReloadableValue[T any] struct {
	value atomic.Value
	mu    sync.RWMutex
}

// NewReloadableValue creates a new reloadable value
func NewReloadableValue[T any](initial T) *ReloadableValue[T] {
	rv := &ReloadableValue[T]{}
	rv.value.Store(initial)
	return rv
}

// Get returns the current value
func (rv *ReloadableValue[T]) Get() T {
	return rv.value.Load().(T)
}

// Set atomically sets a new value
func (rv *ReloadableValue[T]) Set(v T) {
	rv.value.Store(v)
}

// Update atomically updates the value with a function
func (rv *ReloadableValue[T]) Update(fn func(T) T) T {
	rv.mu.Lock()
	defer rv.mu.Unlock()

	current := rv.Get()
	newValue := fn(current)
	rv.Set(newValue)
	return newValue
}

// RuntimeConfig holds runtime-reloadable configuration
type RuntimeConfig struct {
	// Logging
	LogLevel  string `json:"log_level"`
	LogFormat string `json:"log_format"`

	// Limits
	MaxConnections   int `json:"max_connections"`
	MaxCallsPerSec   int `json:"max_calls_per_sec"`
	RateLimitEnabled bool `json:"rate_limit_enabled"`

	// Timeouts
	SessionTimeoutSec int `json:"session_timeout_sec"`
	MediaTimeoutSec   int `json:"media_timeout_sec"`

	// Features
	RecordingEnabled  bool `json:"recording_enabled"`
	TranscodingEnabled bool `json:"transcoding_enabled"`
	DebugMode         bool `json:"debug_mode"`
}

// Validate validates the runtime configuration
func (c *RuntimeConfig) Validate() error {
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if c.LogLevel != "" && !validLevels[c.LogLevel] {
		return fmt.Errorf("invalid log level: %s", c.LogLevel)
	}

	if c.MaxConnections < 0 {
		return errors.New("max_connections must be non-negative")
	}

	if c.SessionTimeoutSec < 0 {
		return errors.New("session_timeout_sec must be non-negative")
	}

	return nil
}

// Hash returns a hash of the configuration
func (c *RuntimeConfig) Hash() string {
	data, _ := json.Marshal(c)
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}

// ConfigWatcher watches multiple config files
type ConfigWatcher struct {
	watchers map[string]*HotReloader
	mu       sync.RWMutex
}

// NewConfigWatcher creates a new config watcher
func NewConfigWatcher() *ConfigWatcher {
	return &ConfigWatcher{
		watchers: make(map[string]*HotReloader),
	}
}

// AddWatcher adds a watcher for a config file
func (cw *ConfigWatcher) AddWatcher(name string, reloader *HotReloader) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	cw.watchers[name] = reloader
}

// RemoveWatcher removes a watcher
func (cw *ConfigWatcher) RemoveWatcher(name string) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	if hr, exists := cw.watchers[name]; exists {
		hr.Stop()
		delete(cw.watchers, name)
	}
}

// StartAll starts all watchers
func (cw *ConfigWatcher) StartAll(ctx context.Context) error {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	for name, hr := range cw.watchers {
		if err := hr.Start(ctx); err != nil {
			return fmt.Errorf("failed to start watcher %s: %w", name, err)
		}
	}
	return nil
}

// StopAll stops all watchers
func (cw *ConfigWatcher) StopAll() {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	for _, hr := range cw.watchers {
		hr.Stop()
	}
}

// GetAllStats returns stats for all watchers
func (cw *ConfigWatcher) GetAllStats() map[string]map[string]interface{} {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	stats := make(map[string]map[string]interface{})
	for name, hr := range cw.watchers {
		stats[name] = hr.GetStats()
	}
	return stats
}

// TriggerReloadAll triggers reload on all watchers
func (cw *ConfigWatcher) TriggerReloadAll() {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	for _, hr := range cw.watchers {
		hr.TriggerReload()
	}
}
