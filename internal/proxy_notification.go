package internal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

func computeHMAC(data []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// ProxyNotificationConfig configures proxy notification behavior
type ProxyNotificationConfig struct {
	// Proxies is the list of SIP proxy addresses to notify
	Proxies []ProxyEndpoint
	// NotificationTimeout is the timeout for notifications
	NotificationTimeout time.Duration
	// RetryAttempts is the number of retry attempts
	RetryAttempts int
	// RetryDelay is the delay between retries
	RetryDelay time.Duration
	// EnableWebhooks enables webhook notifications
	EnableWebhooks bool
	// WebhookURL is the webhook endpoint
	WebhookURL string
	// WebhookSecret is the secret for signing webhooks
	WebhookSecret string
	// EnableNGProtocol enables NG protocol notifications
	EnableNGProtocol bool
	// QueueSize is the notification queue size
	QueueSize int
	// BatchInterval is how often to send batched notifications
	BatchInterval time.Duration
}

// ProxyEndpoint represents a proxy to notify
type ProxyEndpoint struct {
	// Name is a friendly name for the proxy
	Name string `json:"name"`
	// Address is the proxy address (host:port)
	Address string `json:"address"`
	// Protocol is the notification protocol (ng, http, webhook)
	Protocol string `json:"protocol"`
	// Enabled indicates if this proxy should be notified
	Enabled bool `json:"enabled"`
	// Timeout is the timeout for this specific proxy
	Timeout time.Duration `json:"timeout,omitempty"`
}

// DefaultProxyNotificationConfig returns default configuration
func DefaultProxyNotificationConfig() *ProxyNotificationConfig {
	return &ProxyNotificationConfig{
		Proxies:             make([]ProxyEndpoint, 0),
		NotificationTimeout: 5 * time.Second,
		RetryAttempts:       3,
		RetryDelay:          1 * time.Second,
		EnableWebhooks:      false,
		EnableNGProtocol:    true,
		QueueSize:           1000,
		BatchInterval:       100 * time.Millisecond,
	}
}

// ProxyNotifier handles notifying SIP proxies about media server events
type ProxyNotifier struct {
	config     *ProxyNotificationConfig
	nodeID     string
	httpClient *http.Client

	mu          sync.RWMutex
	notifyQueue chan *ProxyNotification
	handlers    map[string]NotificationHandler
	proxies     map[string]*proxyState

	stopChan chan struct{}
	doneChan chan struct{}
}

type proxyState struct {
	endpoint     ProxyEndpoint
	healthy      bool
	lastNotified time.Time
	failures     int
	conn         net.Conn
}

// NotificationHandler handles notifications for specific protocols
type NotificationHandler func(ctx context.Context, endpoint *ProxyEndpoint, notification *ProxyNotification) error

// ProxyNotification represents a notification to send to proxies
type ProxyNotification struct {
	Type      NotificationType       `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	NodeID    string                 `json:"node_id"`
	CallID    string                 `json:"call_id,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	Event     string                 `json:"event"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Priority  NotificationPriority   `json:"priority"`
}

// NotificationType represents the type of notification
type NotificationType string

const (
	NotificationTypeNodeFailover    NotificationType = "node_failover"
	NotificationTypeNodeJoined      NotificationType = "node_joined"
	NotificationTypeNodeLeft        NotificationType = "node_left"
	NotificationTypeSessionTakeover NotificationType = "session_takeover"
	NotificationTypePortChanged     NotificationType = "port_changed"
	NotificationTypeMediaRecovery   NotificationType = "media_recovery"
	NotificationTypeHealthChange    NotificationType = "health_change"
	NotificationTypeCallEnd         NotificationType = "call_end"
	NotificationTypeQualityAlert    NotificationType = "quality_alert"
)

// NotificationPriority represents notification priority
type NotificationPriority int

const (
	NotificationPriorityLow NotificationPriority = iota
	NotificationPriorityNormal
	NotificationPriorityHigh
	NotificationPriorityCritical
)

// NewProxyNotifier creates a new proxy notifier
func NewProxyNotifier(nodeID string, config *ProxyNotificationConfig) *ProxyNotifier {
	if config == nil {
		config = DefaultProxyNotificationConfig()
	}

	pn := &ProxyNotifier{
		config:      config,
		nodeID:      nodeID,
		httpClient:  &http.Client{Timeout: config.NotificationTimeout},
		notifyQueue: make(chan *ProxyNotification, config.QueueSize),
		handlers:    make(map[string]NotificationHandler),
		proxies:     make(map[string]*proxyState),
		stopChan:    make(chan struct{}),
		doneChan:    make(chan struct{}),
	}

	// Initialize proxy states
	for _, proxy := range config.Proxies {
		if proxy.Enabled {
			pn.proxies[proxy.Name] = &proxyState{
				endpoint: proxy,
				healthy:  true,
			}
		}
	}

	// Register default handlers
	pn.handlers["ng"] = pn.handleNGNotification
	pn.handlers["http"] = pn.handleHTTPNotification
	pn.handlers["webhook"] = pn.handleWebhookNotification

	return pn
}

// Start starts the proxy notifier
func (pn *ProxyNotifier) Start() {
	go pn.notificationLoop()
	go pn.healthCheckLoop()
}

// Stop stops the proxy notifier
func (pn *ProxyNotifier) Stop() {
	close(pn.stopChan)
	<-pn.doneChan
}

// AddProxy adds a proxy endpoint
func (pn *ProxyNotifier) AddProxy(endpoint ProxyEndpoint) {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	pn.proxies[endpoint.Name] = &proxyState{
		endpoint: endpoint,
		healthy:  true,
	}
}

// RemoveProxy removes a proxy endpoint
func (pn *ProxyNotifier) RemoveProxy(name string) {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	if state, exists := pn.proxies[name]; exists {
		if state.conn != nil {
			state.conn.Close()
		}
		delete(pn.proxies, name)
	}
}

// Notify sends a notification to all configured proxies
func (pn *ProxyNotifier) Notify(notification *ProxyNotification) error {
	notification.Timestamp = time.Now()
	notification.NodeID = pn.nodeID

	select {
	case pn.notifyQueue <- notification:
		return nil
	default:
		return fmt.Errorf("notification queue full")
	}
}

// NotifyFailover notifies proxies about a node failover
func (pn *ProxyNotifier) NotifyFailover(failedNodeID string, sessionsTaken []string) error {
	return pn.Notify(&ProxyNotification{
		Type:     NotificationTypeNodeFailover,
		Event:    "failover",
		Priority: NotificationPriorityCritical,
		Details: map[string]interface{}{
			"failed_node":    failedNodeID,
			"takeover_node":  pn.nodeID,
			"sessions_taken": sessionsTaken,
			"session_count":  len(sessionsTaken),
		},
	})
}

// NotifySessionTakeover notifies about a specific session takeover
func (pn *ProxyNotifier) NotifySessionTakeover(sessionID, callID, originalNode string, newPorts []int, oldPorts []int) error {
	return pn.Notify(&ProxyNotification{
		Type:      NotificationTypeSessionTakeover,
		CallID:    callID,
		SessionID: sessionID,
		Event:     "session_takeover",
		Priority:  NotificationPriorityHigh,
		Details: map[string]interface{}{
			"original_node": originalNode,
			"new_node":      pn.nodeID,
			"old_ports":     oldPorts,
			"new_ports":     newPorts,
			"ports_changed": !portsEqual(oldPorts, newPorts),
		},
	})
}

// NotifyPortChange notifies about port changes
func (pn *ProxyNotifier) NotifyPortChange(sessionID, callID string, oldPorts, newPorts []int) error {
	return pn.Notify(&ProxyNotification{
		Type:      NotificationTypePortChanged,
		CallID:    callID,
		SessionID: sessionID,
		Event:     "port_change",
		Priority:  NotificationPriorityHigh,
		Details: map[string]interface{}{
			"old_ports": oldPorts,
			"new_ports": newPorts,
		},
	})
}

// NotifyMediaRecovery notifies about media flow recovery
func (pn *ProxyNotifier) NotifyMediaRecovery(sessionID, callID string) error {
	return pn.Notify(&ProxyNotification{
		Type:      NotificationTypeMediaRecovery,
		CallID:    callID,
		SessionID: sessionID,
		Event:     "media_recovered",
		Priority:  NotificationPriorityNormal,
		Details: map[string]interface{}{
			"recovery_node": pn.nodeID,
			"recovery_time": time.Now(),
		},
	})
}

// NotifyNodeJoined notifies about a new node joining the cluster
func (pn *ProxyNotifier) NotifyNodeJoined(joinedNodeID, address string) error {
	return pn.Notify(&ProxyNotification{
		Type:     NotificationTypeNodeJoined,
		Event:    "node_joined",
		Priority: NotificationPriorityNormal,
		Details: map[string]interface{}{
			"joined_node": joinedNodeID,
			"address":     address,
		},
	})
}

// NotifyNodeLeft notifies about a node leaving the cluster
func (pn *ProxyNotifier) NotifyNodeLeft(leftNodeID string, planned bool) error {
	return pn.Notify(&ProxyNotification{
		Type:     NotificationTypeNodeLeft,
		Event:    "node_left",
		Priority: NotificationPriorityHigh,
		Details: map[string]interface{}{
			"left_node":     leftNodeID,
			"planned":       planned,
			"reporting_node": pn.nodeID,
		},
	})
}

// NotifyHealthChange notifies about node health changes
func (pn *ProxyNotifier) NotifyHealthChange(healthy bool, reason string) error {
	priority := NotificationPriorityNormal
	if !healthy {
		priority = NotificationPriorityHigh
	}

	return pn.Notify(&ProxyNotification{
		Type:     NotificationTypeHealthChange,
		Event:    "health_change",
		Priority: priority,
		Details: map[string]interface{}{
			"healthy": healthy,
			"reason":  reason,
		},
	})
}

func (pn *ProxyNotifier) notificationLoop() {
	defer close(pn.doneChan)

	batch := make([]*ProxyNotification, 0, 10)
	batchTimer := time.NewTimer(pn.config.BatchInterval)
	defer batchTimer.Stop()

	for {
		select {
		case <-pn.stopChan:
			// Flush remaining notifications
			pn.sendBatch(batch)
			return

		case notification := <-pn.notifyQueue:
			batch = append(batch, notification)

			// Send critical notifications immediately
			if notification.Priority == NotificationPriorityCritical {
				pn.sendBatch(batch)
				batch = batch[:0]
				batchTimer.Reset(pn.config.BatchInterval)
			} else if len(batch) >= 10 {
				pn.sendBatch(batch)
				batch = batch[:0]
				batchTimer.Reset(pn.config.BatchInterval)
			}

		case <-batchTimer.C:
			if len(batch) > 0 {
				pn.sendBatch(batch)
				batch = batch[:0]
			}
			batchTimer.Reset(pn.config.BatchInterval)
		}
	}
}

func (pn *ProxyNotifier) sendBatch(notifications []*ProxyNotification) {
	if len(notifications) == 0 {
		return
	}

	pn.mu.RLock()
	proxies := make([]*proxyState, 0, len(pn.proxies))
	for _, state := range pn.proxies {
		if state.healthy {
			proxies = append(proxies, state)
		}
	}
	pn.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), pn.config.NotificationTimeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, notification := range notifications {
		for _, proxy := range proxies {
			wg.Add(1)
			go func(p *proxyState, n *ProxyNotification) {
				defer wg.Done()
				pn.sendToProxy(ctx, p, n)
			}(proxy, notification)
		}
	}
	wg.Wait()

	// Send webhook if enabled
	if pn.config.EnableWebhooks && pn.config.WebhookURL != "" {
		for _, notification := range notifications {
			pn.sendWebhook(ctx, notification)
		}
	}
}

func (pn *ProxyNotifier) sendToProxy(ctx context.Context, proxy *proxyState, notification *ProxyNotification) {
	handler, exists := pn.handlers[proxy.endpoint.Protocol]
	if !exists {
		handler = pn.handleHTTPNotification
	}

	var lastErr error
	for attempt := 0; attempt < pn.config.RetryAttempts; attempt++ {
		if err := handler(ctx, &proxy.endpoint, notification); err == nil {
			pn.mu.Lock()
			proxy.lastNotified = time.Now()
			proxy.failures = 0
			pn.mu.Unlock()
			return
		} else {
			lastErr = err
			time.Sleep(pn.config.RetryDelay)
		}
	}

	// Mark proxy as unhealthy after failed attempts
	pn.mu.Lock()
	proxy.failures++
	if proxy.failures >= 3 {
		proxy.healthy = false
	}
	pn.mu.Unlock()

	_ = lastErr // Log error
}

func (pn *ProxyNotifier) handleNGNotification(ctx context.Context, endpoint *ProxyEndpoint, notification *ProxyNotification) error {
	pn.mu.Lock()
	state := pn.proxies[endpoint.Name]
	conn := state.conn
	pn.mu.Unlock()

	// Get or create connection
	if conn == nil {
		var err error
		conn, err = net.DialTimeout("udp", endpoint.Address, pn.config.NotificationTimeout)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		pn.mu.Lock()
		state.conn = conn
		pn.mu.Unlock()
	}

	// Format as NG protocol notification
	ngMessage := pn.formatNGNotification(notification)
	conn.SetWriteDeadline(time.Now().Add(pn.config.NotificationTimeout))
	_, err := conn.Write(ngMessage)
	return err
}

func (pn *ProxyNotifier) formatNGNotification(notification *ProxyNotification) []byte {
	// Format as bencode notification
	data := map[string]interface{}{
		"command":   "notify",
		"type":      string(notification.Type),
		"node":      notification.NodeID,
		"timestamp": notification.Timestamp.Unix(),
	}

	if notification.CallID != "" {
		data["call-id"] = notification.CallID
	}
	if notification.SessionID != "" {
		data["session-id"] = notification.SessionID
	}

	for k, v := range notification.Details {
		data[k] = v
	}

	// Simple bencode encoding
	var buf bytes.Buffer
	encodeBencode(&buf, data)
	return buf.Bytes()
}

func encodeBencode(w *bytes.Buffer, v interface{}) {
	switch val := v.(type) {
	case string:
		fmt.Fprintf(w, "%d:%s", len(val), val)
	case int:
		fmt.Fprintf(w, "i%de", val)
	case int64:
		fmt.Fprintf(w, "i%de", val)
	case bool:
		if val {
			w.WriteString("i1e")
		} else {
			w.WriteString("i0e")
		}
	case map[string]interface{}:
		w.WriteString("d")
		for k, v := range val {
			encodeBencode(w, k)
			encodeBencode(w, v)
		}
		w.WriteString("e")
	case []interface{}:
		w.WriteString("l")
		for _, item := range val {
			encodeBencode(w, item)
		}
		w.WriteString("e")
	case []string:
		w.WriteString("l")
		for _, item := range val {
			encodeBencode(w, item)
		}
		w.WriteString("e")
	case []int:
		w.WriteString("l")
		for _, item := range val {
			encodeBencode(w, item)
		}
		w.WriteString("e")
	default:
		// Try JSON fallback
		if data, err := json.Marshal(val); err == nil {
			fmt.Fprintf(w, "%d:%s", len(data), data)
		}
	}
}

func (pn *ProxyNotifier) handleHTTPNotification(ctx context.Context, endpoint *ProxyEndpoint, notification *ProxyNotification) error {
	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s/api/v1/notifications", endpoint.Address)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-ID", pn.nodeID)

	resp, err := pn.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (pn *ProxyNotifier) handleWebhookNotification(ctx context.Context, endpoint *ProxyEndpoint, notification *ProxyNotification) error {
	return pn.sendWebhook(ctx, notification)
}

func (pn *ProxyNotifier) sendWebhook(ctx context.Context, notification *ProxyNotification) error {
	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", pn.config.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-ID", pn.nodeID)
	req.Header.Set("X-Event-Type", string(notification.Type))

	if pn.config.WebhookSecret != "" {
		// Add HMAC signature
		signature := computeHMAC(data, pn.config.WebhookSecret)
		req.Header.Set("X-Signature", signature)
	}

	resp, err := pn.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}

	return nil
}

func (pn *ProxyNotifier) healthCheckLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pn.stopChan:
			return
		case <-ticker.C:
			pn.checkProxyHealth()
		}
	}
}

func (pn *ProxyNotifier) checkProxyHealth() {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	for name, state := range pn.proxies {
		if !state.healthy {
			// Try to reconnect
			if state.conn != nil {
				state.conn.Close()
				state.conn = nil
			}

			conn, err := net.DialTimeout("udp", state.endpoint.Address, pn.config.NotificationTimeout)
			if err == nil {
				state.conn = conn
				state.healthy = true
				state.failures = 0
			}
		}
		_ = name
	}
}

// GetProxyStatus returns the status of all configured proxies
func (pn *ProxyNotifier) GetProxyStatus() map[string]*ProxyStatus {
	pn.mu.RLock()
	defer pn.mu.RUnlock()

	status := make(map[string]*ProxyStatus)
	for name, state := range pn.proxies {
		status[name] = &ProxyStatus{
			Name:         name,
			Address:      state.endpoint.Address,
			Protocol:     state.endpoint.Protocol,
			Healthy:      state.healthy,
			LastNotified: state.lastNotified,
			Failures:     state.failures,
		}
	}
	return status
}

// ProxyStatus represents the status of a proxy
type ProxyStatus struct {
	Name         string
	Address      string
	Protocol     string
	Healthy      bool
	LastNotified time.Time
	Failures     int
}

// GetStats returns notifier statistics
func (pn *ProxyNotifier) GetStats() *ProxyNotifierStats {
	pn.mu.RLock()
	defer pn.mu.RUnlock()

	healthyCount := 0
	for _, state := range pn.proxies {
		if state.healthy {
			healthyCount++
		}
	}

	return &ProxyNotifierStats{
		TotalProxies:   len(pn.proxies),
		HealthyProxies: healthyCount,
		QueueLength:    len(pn.notifyQueue),
		QueueCapacity:  cap(pn.notifyQueue),
	}
}

// ProxyNotifierStats contains notifier statistics
type ProxyNotifierStats struct {
	TotalProxies   int
	HealthyProxies int
	QueueLength    int
	QueueCapacity  int
}

func portsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
