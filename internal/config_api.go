package internal

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
)

// Mutex for thread-safe configuration access
var configAPIMutex sync.RWMutex

// GetConfigHandler handles API requests to fetch the current configuration
func GetConfigHandler(w http.ResponseWriter, r *http.Request) {
	configAPIMutex.RLock()
	defer configAPIMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(config)
	if err != nil {
		http.Error(w, "Failed to encode configuration", http.StatusInternalServerError)
		return
	}
}

// UpdateConfigHandler handles API requests to update Karl's configuration dynamically
func UpdateConfigHandler(w http.ResponseWriter, r *http.Request) {
	var newConfig Config
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Acquire lock before modifying config
	configAPIMutex.Lock()
	config = &newConfig
	configAPIMutex.Unlock()

	// Save updated configuration to file
	err := SaveConfig("config/config.json", newConfig)
	if err != nil {
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	// Apply the new configuration dynamically
	ApplyNewConfig(newConfig)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "Configuration updated successfully"}`))
}

// SaveConfig writes the updated configuration to `config.json`
func SaveConfig(filePath string, newConfig Config) error {
	data, err := json.MarshalIndent(newConfig, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644) // ✅ Uses os.WriteFile instead of deprecated ioutil
}

// SetupRoutes registers API endpoints for configuration management
func SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// API Endpoints
	mux.HandleFunc("/config", GetConfigHandler)           // GET - Fetch current config
	mux.HandleFunc("/config/update", UpdateConfigHandler) // POST - Update config dynamically

	return mux
}
