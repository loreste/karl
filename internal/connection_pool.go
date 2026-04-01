package internal

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Connection pool errors
var (
	ErrPoolClosed      = errors.New("connection pool is closed")
	ErrPoolExhausted   = errors.New("connection pool exhausted")
	ErrConnectionStale = errors.New("connection is stale")
	ErrDialFailed      = errors.New("failed to dial connection")
)

// PooledConnection represents a pooled connection
type PooledConnection struct {
	conn       net.Conn
	pool       *ConnectionPool
	createdAt  time.Time
	lastUsedAt time.Time
	usageCount int64
	id         int64
}

// Read reads data from the connection
func (pc *PooledConnection) Read(b []byte) (int, error) {
	return pc.conn.Read(b)
}

// Write writes data to the connection
func (pc *PooledConnection) Write(b []byte) (int, error) {
	return pc.conn.Write(b)
}

// Close returns the connection to the pool
func (pc *PooledConnection) Close() error {
	pc.lastUsedAt = time.Now()
	return pc.pool.put(pc)
}

// ForceClose closes the connection without returning to pool
func (pc *PooledConnection) ForceClose() error {
	return pc.conn.Close()
}

// LocalAddr returns the local network address
func (pc *PooledConnection) LocalAddr() net.Addr {
	return pc.conn.LocalAddr()
}

// RemoteAddr returns the remote network address
func (pc *PooledConnection) RemoteAddr() net.Addr {
	return pc.conn.RemoteAddr()
}

// SetDeadline sets the read and write deadlines
func (pc *PooledConnection) SetDeadline(t time.Time) error {
	return pc.conn.SetDeadline(t)
}

// SetReadDeadline sets the read deadline
func (pc *PooledConnection) SetReadDeadline(t time.Time) error {
	return pc.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline
func (pc *PooledConnection) SetWriteDeadline(t time.Time) error {
	return pc.conn.SetWriteDeadline(t)
}

// ConnectionPoolConfig configures a connection pool
type ConnectionPoolConfig struct {
	// MaxSize is the maximum number of connections
	MaxSize int
	// MinSize is the minimum number of idle connections
	MinSize int
	// MaxIdleTime is how long a connection can be idle
	MaxIdleTime time.Duration
	// MaxLifetime is the maximum lifetime of a connection
	MaxLifetime time.Duration
	// DialTimeout for creating new connections
	DialTimeout time.Duration
	// HealthCheckInterval for periodic health checks
	HealthCheckInterval time.Duration
	// WaitTimeout for waiting for available connections
	WaitTimeout time.Duration
}

// DefaultConnectionPoolConfig returns default pool configuration
func DefaultConnectionPoolConfig() *ConnectionPoolConfig {
	return &ConnectionPoolConfig{
		MaxSize:             100,
		MinSize:             5,
		MaxIdleTime:         5 * time.Minute,
		MaxLifetime:         30 * time.Minute,
		DialTimeout:         5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		WaitTimeout:         10 * time.Second,
	}
}

// DialFunc is a function that creates a new connection
type DialFunc func(ctx context.Context) (net.Conn, error)

// ConnectionPool manages a pool of connections
type ConnectionPool struct {
	config *ConnectionPoolConfig
	dial   DialFunc

	mu           sync.Mutex
	connections  []*PooledConnection
	waiters      []chan *PooledConnection
	closed       bool

	// Stats
	totalCreated   atomic.Int64
	totalReused    atomic.Int64
	totalClosed    atomic.Int64
	totalWaitTime  atomic.Int64 // nanoseconds
	currentActive  atomic.Int64
	nextID         atomic.Int64

	// Health check
	stopChan chan struct{}
	doneChan chan struct{}
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(config *ConnectionPoolConfig, dial DialFunc) *ConnectionPool {
	if config == nil {
		config = DefaultConnectionPoolConfig()
	}

	pool := &ConnectionPool{
		config:      config,
		dial:        dial,
		connections: make([]*PooledConnection, 0, config.MaxSize),
		waiters:     make([]chan *PooledConnection, 0),
		stopChan:    make(chan struct{}),
		doneChan:    make(chan struct{}),
	}

	return pool
}

// Start starts the connection pool
func (p *ConnectionPool) Start(ctx context.Context) error {
	// Pre-create minimum connections
	for i := 0; i < p.config.MinSize; i++ {
		conn, err := p.createConnection(ctx)
		if err != nil {
			continue // Don't fail on initial connection errors
		}
		p.mu.Lock()
		p.connections = append(p.connections, conn)
		p.mu.Unlock()
	}

	// Start health check loop
	go p.healthCheckLoop()

	return nil
}

// Stop stops the connection pool
func (p *ConnectionPool) Stop() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true

	// Close all connections
	for _, conn := range p.connections {
		conn.conn.Close()
		p.totalClosed.Add(1)
	}
	p.connections = nil

	// Cancel all waiters
	for _, waiter := range p.waiters {
		close(waiter)
	}
	p.waiters = nil
	p.mu.Unlock()

	close(p.stopChan)
	<-p.doneChan
}

// Get retrieves a connection from the pool
func (p *ConnectionPool) Get(ctx context.Context) (*PooledConnection, error) {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}

	// Try to get an idle connection
	for len(p.connections) > 0 {
		// Get last connection (LIFO for better cache locality)
		conn := p.connections[len(p.connections)-1]
		p.connections = p.connections[:len(p.connections)-1]

		// Check if connection is still valid
		if p.isConnectionValid(conn) {
			p.mu.Unlock()
			conn.usageCount++
			p.totalReused.Add(1)
			p.currentActive.Add(1)
			return conn, nil
		}

		// Connection is stale, close it
		conn.conn.Close()
		p.totalClosed.Add(1)
	}

	// Check if we can create a new connection
	activeCount := p.currentActive.Load()
	if activeCount < int64(p.config.MaxSize) {
		p.mu.Unlock()
		conn, err := p.createConnection(ctx)
		if err != nil {
			return nil, err
		}
		p.currentActive.Add(1)
		return conn, nil
	}

	// Need to wait for a connection
	if p.config.WaitTimeout == 0 {
		p.mu.Unlock()
		return nil, ErrPoolExhausted
	}

	// Create waiter channel
	waiter := make(chan *PooledConnection, 1)
	p.waiters = append(p.waiters, waiter)
	p.mu.Unlock()

	// Wait for connection
	waitStart := time.Now()
	select {
	case conn := <-waiter:
		p.totalWaitTime.Add(time.Since(waitStart).Nanoseconds())
		if conn == nil {
			return nil, ErrPoolClosed
		}
		conn.usageCount++
		p.totalReused.Add(1)
		p.currentActive.Add(1)
		return conn, nil

	case <-time.After(p.config.WaitTimeout):
		// Remove waiter
		p.mu.Lock()
		for i, w := range p.waiters {
			if w == waiter {
				p.waiters = append(p.waiters[:i], p.waiters[i+1:]...)
				break
			}
		}
		p.mu.Unlock()
		return nil, ErrPoolExhausted

	case <-ctx.Done():
		// Remove waiter
		p.mu.Lock()
		for i, w := range p.waiters {
			if w == waiter {
				p.waiters = append(p.waiters[:i], p.waiters[i+1:]...)
				break
			}
		}
		p.mu.Unlock()
		return nil, ctx.Err()
	}
}

// put returns a connection to the pool
func (p *ConnectionPool) put(conn *PooledConnection) error {
	p.currentActive.Add(-1)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		conn.conn.Close()
		p.totalClosed.Add(1)
		return nil
	}

	// Check if connection is still valid
	if !p.isConnectionValid(conn) {
		conn.conn.Close()
		p.totalClosed.Add(1)
		return nil
	}

	// Check if there are waiters
	if len(p.waiters) > 0 {
		waiter := p.waiters[0]
		p.waiters = p.waiters[1:]
		waiter <- conn
		return nil
	}

	// Check if pool is full
	if len(p.connections) >= p.config.MaxSize {
		conn.conn.Close()
		p.totalClosed.Add(1)
		return nil
	}

	// Return to pool
	p.connections = append(p.connections, conn)
	return nil
}

func (p *ConnectionPool) createConnection(ctx context.Context) (*PooledConnection, error) {
	dialCtx := ctx
	if p.config.DialTimeout > 0 {
		var cancel context.CancelFunc
		dialCtx, cancel = context.WithTimeout(ctx, p.config.DialTimeout)
		defer cancel()
	}

	conn, err := p.dial(dialCtx)
	if err != nil {
		return nil, err
	}

	pc := &PooledConnection{
		conn:       conn,
		pool:       p,
		createdAt:  time.Now(),
		lastUsedAt: time.Now(),
		id:         p.nextID.Add(1),
	}

	p.totalCreated.Add(1)
	return pc, nil
}

func (p *ConnectionPool) isConnectionValid(conn *PooledConnection) bool {
	now := time.Now()

	// Check max lifetime
	if p.config.MaxLifetime > 0 && now.Sub(conn.createdAt) > p.config.MaxLifetime {
		return false
	}

	// Check max idle time
	if p.config.MaxIdleTime > 0 && now.Sub(conn.lastUsedAt) > p.config.MaxIdleTime {
		return false
	}

	return true
}

func (p *ConnectionPool) healthCheckLoop() {
	defer close(p.doneChan)

	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.healthCheck()
		}
	}
}

func (p *ConnectionPool) healthCheck() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	// Remove stale connections
	valid := make([]*PooledConnection, 0, len(p.connections))
	for _, conn := range p.connections {
		if p.isConnectionValid(conn) {
			valid = append(valid, conn)
		} else {
			conn.conn.Close()
			p.totalClosed.Add(1)
		}
	}
	p.connections = valid

	// Ensure minimum connections
	needed := p.config.MinSize - len(p.connections) - int(p.currentActive.Load())
	if needed > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), p.config.DialTimeout)
		defer cancel()

		for i := 0; i < needed; i++ {
			conn, err := p.createConnection(ctx)
			if err != nil {
				break
			}
			p.connections = append(p.connections, conn)
		}
	}
}

// Stats returns pool statistics
func (p *ConnectionPool) Stats() *ConnectionPoolStats {
	p.mu.Lock()
	idle := len(p.connections)
	waiters := len(p.waiters)
	p.mu.Unlock()

	return &ConnectionPoolStats{
		TotalCreated:    p.totalCreated.Load(),
		TotalReused:     p.totalReused.Load(),
		TotalClosed:     p.totalClosed.Load(),
		CurrentActive:   p.currentActive.Load(),
		CurrentIdle:     int64(idle),
		CurrentWaiters:  int64(waiters),
		AvgWaitTime:     p.getAvgWaitTime(),
	}
}

func (p *ConnectionPool) getAvgWaitTime() time.Duration {
	reused := p.totalReused.Load()
	if reused == 0 {
		return 0
	}
	return time.Duration(p.totalWaitTime.Load() / reused)
}

// ConnectionPoolStats holds pool statistics
type ConnectionPoolStats struct {
	TotalCreated   int64
	TotalReused    int64
	TotalClosed    int64
	CurrentActive  int64
	CurrentIdle    int64
	CurrentWaiters int64
	AvgWaitTime    time.Duration
}

// UDPConnectionPool manages a pool of UDP connections
type UDPConnectionPool struct {
	config     *ConnectionPoolConfig
	localAddr  *net.UDPAddr
	remoteAddr *net.UDPAddr

	mu          sync.Mutex
	connections []*net.UDPConn
	closed      bool

	totalCreated atomic.Int64
	totalReused  atomic.Int64
}

// NewUDPConnectionPool creates a UDP connection pool
func NewUDPConnectionPool(config *ConnectionPoolConfig, localAddr, remoteAddr *net.UDPAddr) *UDPConnectionPool {
	if config == nil {
		config = DefaultConnectionPoolConfig()
	}

	return &UDPConnectionPool{
		config:      config,
		localAddr:   localAddr,
		remoteAddr:  remoteAddr,
		connections: make([]*net.UDPConn, 0, config.MaxSize),
	}
}

// Get retrieves a UDP connection
func (p *UDPConnectionPool) Get() (*net.UDPConn, error) {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}

	// Try to get existing connection
	if len(p.connections) > 0 {
		conn := p.connections[len(p.connections)-1]
		p.connections = p.connections[:len(p.connections)-1]
		p.mu.Unlock()
		p.totalReused.Add(1)
		return conn, nil
	}

	p.mu.Unlock()

	// Create new connection
	conn, err := net.DialUDP("udp", p.localAddr, p.remoteAddr)
	if err != nil {
		return nil, err
	}

	p.totalCreated.Add(1)
	return conn, nil
}

// Put returns a UDP connection to the pool
func (p *UDPConnectionPool) Put(conn *net.UDPConn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || len(p.connections) >= p.config.MaxSize {
		conn.Close()
		return
	}

	p.connections = append(p.connections, conn)
}

// Close closes the UDP connection pool
func (p *UDPConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	for _, conn := range p.connections {
		conn.Close()
	}
	p.connections = nil
}

// Stats returns pool statistics
func (p *UDPConnectionPool) Stats() *ConnectionPoolStats {
	p.mu.Lock()
	idle := len(p.connections)
	p.mu.Unlock()

	return &ConnectionPoolStats{
		TotalCreated: p.totalCreated.Load(),
		TotalReused:  p.totalReused.Load(),
		CurrentIdle:  int64(idle),
	}
}
