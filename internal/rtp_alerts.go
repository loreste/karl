package internal

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// RTPStats stores real-time RTP statistics for monitoring
type RTPStats struct {
	PacketLoss     float64
	Jitter         float64
	BandwidthUsage int
}

var (
	alerts        []RTPAlert
	alertMutex    sync.RWMutex
	alertChan     = make(chan RTPAlert, 10)
	rtpStats      RTPStats
	rtpStatsMutex sync.RWMutex
	alertConfig   AlertSettings
)

// RTPAlert represents an RTP-related issue detected in real-time
type RTPAlert struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
}

// MonitorRTPAlerts continuously checks RTP statistics against alert thresholds
func MonitorRTPAlerts() {
	for {
		time.Sleep(2 * time.Second) // Adjust interval as needed

		rtpStatsMutex.RLock()
		stats := rtpStats
		rtpStatsMutex.RUnlock()

		configMutex.RLock()
		alertConfig = config.AlertSettings
		configMutex.RUnlock()

		checkForAlerts(stats, alertConfig)
	}
}

// checkForAlerts evaluates RTP statistics and triggers alerts if thresholds are exceeded
func checkForAlerts(stats RTPStats, alertConfig AlertSettings) {
	if stats.PacketLoss > alertConfig.PacketLossThreshold {
		triggerAlert("Packet Loss", "High packet loss detected", stats.PacketLoss, alertConfig.PacketLossThreshold)
	}

	if stats.Jitter > alertConfig.JitterThreshold {
		triggerAlert("Jitter", "High jitter detected", stats.Jitter, alertConfig.JitterThreshold)
	}

	if stats.BandwidthUsage > alertConfig.BandwidthThreshold {
		triggerAlert("Bandwidth", "High bandwidth usage detected", float64(stats.BandwidthUsage), float64(alertConfig.BandwidthThreshold))
	}
}

// triggerAlert logs an alert, saves it, and sends a real-time notification
func triggerAlert(alertType, description string, value, threshold float64) {
	alert := RTPAlert{
		Timestamp:   time.Now(),
		Type:        alertType,
		Description: description,
		Value:       value,
		Threshold:   threshold,
	}

	alertMutex.Lock()
	alerts = append(alerts, alert)
	if len(alerts) > 50 {
		alerts = alerts[1:] // Keep the latest 50 alerts
	}
	alertMutex.Unlock()

	alertChan <- alert
	log.Printf("ALERT: %s - %s (Value: %.2f, Threshold: %.2f)", alertType, description, value, threshold)
}

// GetActiveAlerts API to retrieve all active alerts
func GetActiveAlerts(w http.ResponseWriter, r *http.Request) {
	alertMutex.RLock()
	defer alertMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

// UpdateAlertThresholds updates alert thresholds dynamically
func UpdateAlertThresholds(newConfig AlertSettings) {
	configMutex.Lock()
	config.AlertSettings = newConfig
	configMutex.Unlock()

	log.Println("Updated RTP alert thresholds dynamically.")
}
