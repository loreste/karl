package internal

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestDefaultConnectionPoolConfig(t *testing.T) {
	config := DefaultConnectionPoolConfig()

	if config.MaxSize != 100 {
		t.Errorf("expected MaxSize=100, got %d", config.MaxSize)
	}
	if config.MinSize != 5 {
		t.Errorf("expected MinSize=5, got %d", config.MinSize)
	}
	if config.MaxIdleTime != 5*time.Minute {
		t.Errorf("expected MaxIdleTime=5m, got %v", config.MaxIdleTime)
	}
	if config.MaxLifetime != 30*time.Minute {
		t.Errorf("expected MaxLifetime=30m, got %v", config.MaxLifetime)
	}
	if config.DialTimeout != 5*time.Second {
		t.Errorf("expected DialTimeout=5s, got %v", config.DialTimeout)
	}
}

// mockConn implements net.Conn for testing
type mockConn struct {
	closed bool
}

func (m *mockConn) Read(b []byte) (int, error)         { return 0, nil }
func (m *mockConn) Write(b []byte) (int, error)        { return len(b), nil }
func (m *mockConn) Close() error                       { m.closed = true; return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestNewConnectionPool(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool := NewConnectionPool(nil, dial)
	if pool.config.MaxSize != 100 {
		t.Error("expected default config")
	}

	config := &ConnectionPoolConfig{MaxSize: 50}
	pool = NewConnectionPool(config, dial)
	if pool.config.MaxSize != 50 {
		t.Errorf("expected MaxSize=50, got %d", pool.config.MaxSize)
	}
}

func TestConnectionPool_GetPut(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxSize:             10,
		MinSize:             0,
		HealthCheckInterval: time.Hour, // Disable for test
	}

	pool := NewConnectionPool(config, dial)
	pool.Start(context.Background())
	defer pool.Stop()

	ctx := context.Background()

	// Get a connection
	conn1, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get connection: %v", err)
	}
	if conn1 == nil {
		t.Fatal("expected non-nil connection")
	}

	stats := pool.Stats()
	if stats.TotalCreated != 1 {
		t.Errorf("expected TotalCreated=1, got %d", stats.TotalCreated)
	}
	if stats.CurrentActive != 1 {
		t.Errorf("expected CurrentActive=1, got %d", stats.CurrentActive)
	}

	// Return connection
	conn1.Close()

	stats = pool.Stats()
	if stats.CurrentIdle != 1 {
		t.Errorf("expected CurrentIdle=1, got %d", stats.CurrentIdle)
	}

	// Get again - should reuse
	conn2, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get connection: %v", err)
	}

	stats = pool.Stats()
	if stats.TotalReused != 1 {
		t.Errorf("expected TotalReused=1, got %d", stats.TotalReused)
	}

	conn2.Close()
}

func TestConnectionPool_MaxSize(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxSize:             2,
		MinSize:             0,
		WaitTimeout:         0, // No waiting
		HealthCheckInterval: time.Hour,
	}

	pool := NewConnectionPool(config, dial)
	pool.Start(context.Background())
	defer pool.Stop()

	ctx := context.Background()

	// Get max connections
	conn1, _ := pool.Get(ctx)
	conn2, _ := pool.Get(ctx)

	// Third should fail
	_, err := pool.Get(ctx)
	if err != ErrPoolExhausted {
		t.Errorf("expected ErrPoolExhausted, got %v", err)
	}

	// Return one
	conn1.Close()

	// Now should work
	conn3, err := pool.Get(ctx)
	if err != nil {
		t.Errorf("expected success after return, got %v", err)
	}

	conn2.Close()
	conn3.Close()
}

func TestConnectionPool_WaitTimeout(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxSize:             1,
		MinSize:             0,
		WaitTimeout:         50 * time.Millisecond,
		HealthCheckInterval: time.Hour,
	}

	pool := NewConnectionPool(config, dial)
	pool.Start(context.Background())
	defer pool.Stop()

	ctx := context.Background()

	// Take the only connection
	conn, _ := pool.Get(ctx)

	// Try to get another with timeout
	start := time.Now()
	_, err := pool.Get(ctx)
	elapsed := time.Since(start)

	if err != ErrPoolExhausted {
		t.Errorf("expected ErrPoolExhausted, got %v", err)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("expected to wait at least 40ms, waited %v", elapsed)
	}

	conn.Close()
}

func TestConnectionPool_WaitSuccess(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxSize:             1,
		MinSize:             0,
		WaitTimeout:         500 * time.Millisecond,
		HealthCheckInterval: time.Hour,
	}

	pool := NewConnectionPool(config, dial)
	pool.Start(context.Background())
	defer pool.Stop()

	ctx := context.Background()

	// Take the only connection
	conn, _ := pool.Get(ctx)

	// Return after delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		conn.Close()
	}()

	// Should get the returned connection
	conn2, err := pool.Get(ctx)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
	if conn2 == nil {
		t.Error("expected non-nil connection")
	}

	conn2.Close()
}

func TestConnectionPool_Closed(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxSize:             10,
		MinSize:             0,
		HealthCheckInterval: time.Hour,
	}

	pool := NewConnectionPool(config, dial)
	pool.Start(context.Background())
	pool.Stop()

	ctx := context.Background()
	_, err := pool.Get(ctx)
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

func TestConnectionPool_ContextCancellation(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxSize:             1,
		MinSize:             0,
		WaitTimeout:         5 * time.Second, // Long timeout
		HealthCheckInterval: time.Hour,
	}

	pool := NewConnectionPool(config, dial)
	pool.Start(context.Background())
	defer pool.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Take the only connection
	conn, _ := pool.Get(context.Background())

	// Try to get with cancellable context
	_, err := pool.Get(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	conn.Close()
}

func TestConnectionPool_Concurrent(t *testing.T) {
	var created int64
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxSize:             10,
		MinSize:             0,
		WaitTimeout:         time.Second,
		HealthCheckInterval: time.Hour,
	}

	pool := NewConnectionPool(config, dial)
	pool.Start(context.Background())
	defer pool.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 50
	iterations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				conn, err := pool.Get(ctx)
				if err != nil {
					continue
				}
				time.Sleep(time.Millisecond)
				conn.Close()
			}
		}()
	}

	wg.Wait()

	stats := pool.Stats()
	_ = created
	if stats.TotalCreated == 0 {
		t.Error("expected some connections created")
	}
	if stats.TotalReused == 0 {
		t.Error("expected some connection reuse")
	}
}

func TestPooledConnection_Methods(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxSize:             10,
		MinSize:             0,
		HealthCheckInterval: time.Hour,
	}

	pool := NewConnectionPool(config, dial)
	pool.Start(context.Background())
	defer pool.Stop()

	conn, _ := pool.Get(context.Background())

	// Test interface methods
	if conn.LocalAddr() != nil {
		t.Error("expected nil LocalAddr from mock")
	}
	if conn.RemoteAddr() != nil {
		t.Error("expected nil RemoteAddr from mock")
	}

	data := []byte("test")
	n, err := conn.Write(data)
	if err != nil || n != len(data) {
		t.Error("Write should succeed")
	}

	_, err = conn.Read(make([]byte, 10))
	if err != nil {
		t.Error("Read should succeed")
	}

	err = conn.SetDeadline(time.Now())
	if err != nil {
		t.Error("SetDeadline should succeed")
	}

	conn.Close()
}

func TestConnectionPool_StaleConnections(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxSize:             10,
		MinSize:             0,
		MaxIdleTime:         50 * time.Millisecond,
		HealthCheckInterval: time.Hour,
	}

	pool := NewConnectionPool(config, dial)
	pool.Start(context.Background())
	defer pool.Stop()

	ctx := context.Background()

	// Get and return a connection
	conn, _ := pool.Get(ctx)
	conn.Close()

	// Wait for it to become stale
	time.Sleep(100 * time.Millisecond)

	// Get again - should create new
	conn2, _ := pool.Get(ctx)
	if conn2 == nil {
		t.Fatal("expected non-nil connection")
	}

	stats := pool.Stats()
	if stats.TotalCreated < 2 {
		t.Errorf("expected at least 2 created (stale + new), got %d", stats.TotalCreated)
	}

	conn2.Close()
}

func TestUDPConnectionPool(t *testing.T) {
	// Create a UDP listener for testing
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	remoteAddr := listener.LocalAddr().(*net.UDPAddr)

	config := &ConnectionPoolConfig{
		MaxSize: 5,
	}

	pool := NewUDPConnectionPool(config, nil, remoteAddr)

	// Get connection
	conn1, err := pool.Get()
	if err != nil {
		t.Fatalf("failed to get UDP connection: %v", err)
	}

	stats := pool.Stats()
	if stats.TotalCreated != 1 {
		t.Errorf("expected TotalCreated=1, got %d", stats.TotalCreated)
	}

	// Return and get again
	pool.Put(conn1)
	conn2, err := pool.Get()
	if err != nil {
		t.Fatalf("failed to get UDP connection: %v", err)
	}

	stats = pool.Stats()
	if stats.TotalReused != 1 {
		t.Errorf("expected TotalReused=1, got %d", stats.TotalReused)
	}

	pool.Put(conn2)
	pool.Close()

	// After close, should fail
	_, err = pool.Get()
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

func TestUDPConnectionPool_MaxSize(t *testing.T) {
	listener, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	defer listener.Close()
	remoteAddr := listener.LocalAddr().(*net.UDPAddr)

	config := &ConnectionPoolConfig{
		MaxSize: 2,
	}

	pool := NewUDPConnectionPool(config, nil, remoteAddr)
	defer pool.Close()

	// Get connections
	conn1, _ := pool.Get()
	conn2, _ := pool.Get()

	// Return all
	pool.Put(conn1)
	pool.Put(conn2)

	stats := pool.Stats()
	if stats.CurrentIdle != 2 {
		t.Errorf("expected CurrentIdle=2, got %d", stats.CurrentIdle)
	}

	// Get a third and return - should be closed (over max)
	conn3, _ := pool.Get()
	pool.Put(conn3)

	// Pool should still have max 2
	stats = pool.Stats()
	if stats.CurrentIdle > 2 {
		t.Errorf("expected CurrentIdle<=2, got %d", stats.CurrentIdle)
	}
}

func TestConnectionPool_Stats(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxSize:             10,
		MinSize:             0,
		HealthCheckInterval: time.Hour,
	}

	pool := NewConnectionPool(config, dial)
	pool.Start(context.Background())
	defer pool.Stop()

	ctx := context.Background()

	// Initial stats
	stats := pool.Stats()
	if stats.TotalCreated != 0 {
		t.Errorf("expected TotalCreated=0, got %d", stats.TotalCreated)
	}

	// Get and return
	conn, _ := pool.Get(ctx)
	conn.Close()

	stats = pool.Stats()
	if stats.TotalCreated != 1 {
		t.Errorf("expected TotalCreated=1, got %d", stats.TotalCreated)
	}
	if stats.CurrentIdle != 1 {
		t.Errorf("expected CurrentIdle=1, got %d", stats.CurrentIdle)
	}
}
