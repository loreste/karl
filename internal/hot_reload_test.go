package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultHotReloadConfig(t *testing.T) {
	config := DefaultHotReloadConfig()

	if config.PollInterval != 30*time.Second {
		t.Errorf("Expected PollInterval 30s, got %v", config.PollInterval)
	}
	if !config.ValidateOnLoad {
		t.Error("Expected ValidateOnLoad to be true")
	}
	if !config.SignalReload {
		t.Error("Expected SignalReload to be true")
	}
}

func TestFileConfigLoader(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	config := map[string]interface{}{
		"key": "value",
	}
	data, _ := json.Marshal(config)
	os.WriteFile(configPath, data, 0644)

	loader := NewFileConfigLoader(configPath, func(data []byte) (interface{}, error) {
		var cfg map[string]interface{}
		err := json.Unmarshal(data, &cfg)
		return cfg, err
	})

	// Test Load
	loaded, err := loader.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cfg := loaded.(map[string]interface{})
	if cfg["key"] != "value" {
		t.Errorf("Expected key=value, got %v", cfg["key"])
	}

	// Test Hash
	hash1, err := loader.Hash()
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}
	if hash1 == "" {
		t.Error("Hash should not be empty")
	}

	// Modify file and check hash changes
	config["key"] = "new_value"
	data, _ = json.Marshal(config)
	os.WriteFile(configPath, data, 0644)

	hash2, _ := loader.Hash()
	if hash1 == hash2 {
		t.Error("Hash should change when file changes")
	}
}

func TestNewHotReloader(t *testing.T) {
	loader := &mockConfigLoader{
		config: map[string]string{"test": "value"},
		hash:   "abc123",
	}

	hr := NewHotReloader(nil, loader)

	if hr == nil {
		t.Fatal("NewHotReloader returned nil")
	}
}

func TestHotReloader_LoadInitial(t *testing.T) {
	loader := &mockConfigLoader{
		config: map[string]string{"test": "value"},
		hash:   "abc123",
	}

	hr := NewHotReloader(nil, loader)

	err := hr.LoadInitial()
	if err != nil {
		t.Fatalf("LoadInitial failed: %v", err)
	}

	config := hr.GetConfig().(map[string]string)
	if config["test"] != "value" {
		t.Errorf("Expected test=value, got %v", config["test"])
	}
}

func TestHotReloader_RegisterHandler(t *testing.T) {
	loader := &mockConfigLoader{
		config: map[string]string{"test": "value"},
		hash:   "abc123",
	}

	hr := NewHotReloader(nil, loader)

	handlerCalled := false
	hr.RegisterHandler("test", func(oldConfig, newConfig interface{}) error {
		handlerCalled = true
		return nil
	})

	// Trigger a reload
	hr.LoadInitial()
	loader.hash = "def456"
	loader.config = map[string]string{"test": "new_value"}
	hr.reload()

	if !handlerCalled {
		t.Error("Handler should have been called")
	}
}

func TestHotReloader_UnregisterHandler(t *testing.T) {
	loader := &mockConfigLoader{
		config: map[string]string{"test": "value"},
		hash:   "abc123",
	}

	hr := NewHotReloader(nil, loader)

	var callCount atomic.Int32
	hr.RegisterHandler("test", func(oldConfig, newConfig interface{}) error {
		callCount.Add(1)
		return nil
	})

	hr.LoadInitial()

	// First reload - handler called
	loader.hash = "def456"
	hr.reload()

	if callCount.Load() != 1 {
		t.Error("Handler should have been called once")
	}

	// Unregister handler
	hr.UnregisterHandler("test")

	// Second reload - handler not called
	loader.hash = "ghi789"
	hr.reload()

	if callCount.Load() != 1 {
		t.Error("Handler should not have been called after unregister")
	}
}

func TestHotReloader_StartStop(t *testing.T) {
	loader := &mockConfigLoader{
		config: map[string]string{"test": "value"},
		hash:   "abc123",
	}

	config := &HotReloadConfig{
		PollInterval: 50 * time.Millisecond,
	}
	hr := NewHotReloader(config, loader)
	hr.LoadInitial()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := hr.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !hr.isRunning.Load() {
		t.Error("Should be running after Start")
	}

	// Try starting again
	err = hr.Start(ctx)
	if err == nil {
		t.Error("Second Start should return error")
	}

	hr.Stop()

	if hr.isRunning.Load() {
		t.Error("Should not be running after Stop")
	}
}

func TestHotReloader_TriggerReload(t *testing.T) {
	loader := &mockConfigLoader{
		config: map[string]string{"test": "value"},
		hash:   "abc123",
	}

	config := &HotReloadConfig{
		PollInterval: time.Hour, // Long interval to ensure manual trigger works
	}
	hr := NewHotReloader(config, loader)
	hr.LoadInitial()

	var reloadCount atomic.Int32
	hr.RegisterHandler("counter", func(oldConfig, newConfig interface{}) error {
		reloadCount.Add(1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hr.Start(ctx)
	defer hr.Stop()

	// Change config and trigger reload
	loader.hash = "def456"
	loader.config = map[string]string{"test": "new_value"}
	hr.TriggerReload()

	// Wait for reload to process
	time.Sleep(100 * time.Millisecond)

	if reloadCount.Load() != 1 {
		t.Errorf("Expected 1 reload, got %d", reloadCount.Load())
	}
}

func TestHotReloader_Callbacks(t *testing.T) {
	loader := &mockConfigLoader{
		config: map[string]string{"test": "value"},
		hash:   "abc123",
	}

	hr := NewHotReloader(nil, loader)
	hr.LoadInitial()

	var startCalled, successCalled bool
	hr.SetCallbacks(
		func() { startCalled = true },
		func() { successCalled = true },
		nil,
	)

	loader.hash = "def456"
	hr.reload()

	if !startCalled {
		t.Error("onReloadStart should have been called")
	}
	if !successCalled {
		t.Error("onReloadSuccess should have been called")
	}
}

func TestHotReloader_GetStats(t *testing.T) {
	loader := &mockConfigLoader{
		config: map[string]string{"test": "value"},
		hash:   "abc123",
	}

	hr := NewHotReloader(nil, loader)
	hr.LoadInitial()

	// Trigger a reload
	loader.hash = "def456"
	hr.reload()

	stats := hr.GetStats()

	if stats["reload_count"].(int64) != 1 {
		t.Errorf("Expected reload_count 1, got %v", stats["reload_count"])
	}
	if stats["config_hash"].(string) != "def456" {
		t.Errorf("Expected config_hash def456, got %v", stats["config_hash"])
	}
}

func TestHotReloader_NoChangeNoReload(t *testing.T) {
	loader := &mockConfigLoader{
		config: map[string]string{"test": "value"},
		hash:   "abc123",
	}

	hr := NewHotReloader(nil, loader)
	hr.LoadInitial()

	var reloadCount atomic.Int32
	hr.RegisterHandler("counter", func(oldConfig, newConfig interface{}) error {
		reloadCount.Add(1)
		return nil
	})

	// Check without change
	hr.checkAndReload()

	if reloadCount.Load() != 0 {
		t.Error("Should not reload when config hasn't changed")
	}
}

func TestRuntimeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  RuntimeConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  RuntimeConfig{LogLevel: "info", MaxConnections: 100},
			wantErr: false,
		},
		{
			name:    "invalid log level",
			config:  RuntimeConfig{LogLevel: "invalid"},
			wantErr: true,
		},
		{
			name:    "negative max connections",
			config:  RuntimeConfig{MaxConnections: -1},
			wantErr: true,
		},
		{
			name:    "negative timeout",
			config:  RuntimeConfig{SessionTimeoutSec: -1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRuntimeConfig_Hash(t *testing.T) {
	config1 := RuntimeConfig{LogLevel: "info"}
	config2 := RuntimeConfig{LogLevel: "debug"}
	config3 := RuntimeConfig{LogLevel: "info"}

	hash1 := config1.Hash()
	hash2 := config2.Hash()
	hash3 := config3.Hash()

	if hash1 == hash2 {
		t.Error("Different configs should have different hashes")
	}
	if hash1 != hash3 {
		t.Error("Same configs should have same hash")
	}
}

func TestReloadableValue(t *testing.T) {
	rv := NewReloadableValue(10)

	if rv.Get() != 10 {
		t.Errorf("Expected 10, got %d", rv.Get())
	}

	rv.Set(20)
	if rv.Get() != 20 {
		t.Errorf("Expected 20, got %d", rv.Get())
	}

	result := rv.Update(func(v int) int {
		return v + 5
	})
	if result != 25 {
		t.Errorf("Expected 25, got %d", result)
	}
}

func TestConfigWatcher(t *testing.T) {
	cw := NewConfigWatcher()

	loader1 := &mockConfigLoader{config: "config1", hash: "hash1"}
	loader2 := &mockConfigLoader{config: "config2", hash: "hash2"}

	hr1 := NewHotReloader(&HotReloadConfig{PollInterval: time.Hour}, loader1)
	hr2 := NewHotReloader(&HotReloadConfig{PollInterval: time.Hour}, loader2)

	cw.AddWatcher("config1", hr1)
	cw.AddWatcher("config2", hr2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := cw.StartAll(ctx)
	if err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}

	stats := cw.GetAllStats()
	if len(stats) != 2 {
		t.Errorf("Expected 2 watchers in stats, got %d", len(stats))
	}

	cw.RemoveWatcher("config1")

	stats = cw.GetAllStats()
	if len(stats) != 1 {
		t.Errorf("Expected 1 watcher after removal, got %d", len(stats))
	}

	cw.StopAll()
}

func TestConfigWatcher_TriggerReloadAll(t *testing.T) {
	cw := NewConfigWatcher()

	var reloadCount atomic.Int32

	loader := &mockConfigLoader{config: "config", hash: "hash1"}
	config := &HotReloadConfig{PollInterval: time.Hour}
	hr := NewHotReloader(config, loader)
	hr.LoadInitial()
	hr.RegisterHandler("counter", func(oldConfig, newConfig interface{}) error {
		reloadCount.Add(1)
		return nil
	})

	cw.AddWatcher("config", hr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cw.StartAll(ctx)
	defer cw.StopAll()

	// Change and trigger reload
	loader.hash = "hash2"
	cw.TriggerReloadAll()

	time.Sleep(100 * time.Millisecond)

	if reloadCount.Load() != 1 {
		t.Errorf("Expected 1 reload, got %d", reloadCount.Load())
	}
}

func TestFileConfigLoader_Error(t *testing.T) {
	loader := NewFileConfigLoader("/nonexistent/path", func(data []byte) (interface{}, error) {
		var cfg interface{}
		err := json.Unmarshal(data, &cfg)
		return cfg, err
	})

	_, err := loader.Load()
	if err == nil {
		t.Error("Load should fail for nonexistent file")
	}

	_, err = loader.Hash()
	if err == nil {
		t.Error("Hash should fail for nonexistent file")
	}
}

// mockConfigLoader is a test helper
type mockConfigLoader struct {
	config interface{}
	hash   string
	loadErr error
}

func (m *mockConfigLoader) Load() (interface{}, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.config, nil
}

func (m *mockConfigLoader) Hash() (string, error) {
	return m.hash, nil
}
