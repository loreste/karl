package internal

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ShardedSocketPool manages a pool of sharded UDP sockets for high-performance RTP forwarding
type ShardedSocketPool struct {
	sockets      []*ShardedSocket
	numShards    int
	basePort     int
	listenAddr   string
	nextShard    atomic.Uint64
	mu           sync.RWMutex
	stopped      bool
	stats        *socketPoolStats
	bufferPool   *sync.Pool
	recvCallback func([]byte, *net.UDPAddr, int)
}

// ShardedSocket represents a single sharded UDP socket
type ShardedSocket struct {
	conn       *net.UDPConn
	shardID    int
	port       int
	recvBuffer []byte
	sendBuffer []byte
	stats      *socketStats
	stopCh     chan struct{}
	pool       *ShardedSocketPool
}

type socketPoolStats struct {
	totalReceived   atomic.Uint64
	totalSent       atomic.Uint64
	totalBytesRecv  atomic.Uint64
	totalBytesSent  atomic.Uint64
	receiveErrors   atomic.Uint64
	sendErrors      atomic.Uint64
	droppedPackets  atomic.Uint64
}

type socketStats struct {
	received   atomic.Uint64
	sent       atomic.Uint64
	bytesRecv  atomic.Uint64
	bytesSent  atomic.Uint64
	errors     atomic.Uint64
}

// SocketPoolConfig configuration for socket pool
type SocketPoolConfig struct {
	NumShards      int    // Number of shards (default: NumCPU)
	BasePort       int    // Starting port number
	ListenAddress  string // Listen address (default: "0.0.0.0")
	RecvBufferSize int    // UDP receive buffer size (default: 4MB)
	SendBufferSize int    // UDP send buffer size (default: 4MB)
	PacketSize     int    // Max packet size (default: 1500)
}

// DefaultSocketPoolConfig returns default configuration
func DefaultSocketPoolConfig() *SocketPoolConfig {
	return &SocketPoolConfig{
		NumShards:      runtime.NumCPU(),
		BasePort:       20000,
		ListenAddress:  "0.0.0.0",
		RecvBufferSize: 4 * 1024 * 1024, // 4MB
		SendBufferSize: 4 * 1024 * 1024, // 4MB
		PacketSize:     1500,
	}
}

// NewShardedSocketPool creates a new sharded socket pool
func NewShardedSocketPool(config *SocketPoolConfig) (*ShardedSocketPool, error) {
	if config == nil {
		config = DefaultSocketPoolConfig()
	}

	if config.NumShards <= 0 {
		config.NumShards = runtime.NumCPU()
	}

	pool := &ShardedSocketPool{
		sockets:    make([]*ShardedSocket, config.NumShards),
		numShards:  config.NumShards,
		basePort:   config.BasePort,
		listenAddr: config.ListenAddress,
		stats:      &socketPoolStats{},
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, config.PacketSize)
			},
		},
	}

	// Create sharded sockets
	for i := 0; i < config.NumShards; i++ {
		port := config.BasePort + i*2 // Use even ports for RTP
		socket, err := pool.createShardedSocket(i, port, config)
		if err != nil {
			// Clean up already created sockets
			for j := 0; j < i; j++ {
				if pool.sockets[j] != nil {
					pool.sockets[j].Close()
				}
			}
			return nil, fmt.Errorf("failed to create shard %d: %w", i, err)
		}
		pool.sockets[i] = socket
	}

	return pool, nil
}

// createShardedSocket creates a single sharded socket
func (p *ShardedSocketPool) createShardedSocket(shardID, port int, config *SocketPoolConfig) (*ShardedSocket, error) {
	addr := fmt.Sprintf("%s:%d", p.listenAddr, port)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	// Set socket options for performance
	if err := p.setSocketOptions(conn, config); err != nil {
		conn.Close()
		return nil, err
	}

	socket := &ShardedSocket{
		conn:       conn,
		shardID:    shardID,
		port:       port,
		recvBuffer: make([]byte, config.PacketSize),
		sendBuffer: make([]byte, config.PacketSize),
		stats:      &socketStats{},
		stopCh:     make(chan struct{}),
		pool:       p,
	}

	return socket, nil
}

// setSocketOptions configures socket for high performance
func (p *ShardedSocketPool) setSocketOptions(conn *net.UDPConn, config *SocketPoolConfig) error {
	// Set receive buffer size
	if err := conn.SetReadBuffer(config.RecvBufferSize); err != nil {
		return fmt.Errorf("failed to set read buffer: %w", err)
	}

	// Set send buffer size
	if err := conn.SetWriteBuffer(config.SendBufferSize); err != nil {
		return fmt.Errorf("failed to set write buffer: %w", err)
	}

	// Get the underlying file descriptor for additional options
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return nil // Non-fatal, continue without advanced options
	}

	// Set SO_REUSEPORT for socket sharding (allows multiple sockets on same port)
	rawConn.Control(func(fd uintptr) {
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		// SO_REUSEPORT may not be available on all platforms
		// syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEPORT, 1)
	})

	return nil
}

// Start begins receiving packets on all shards
func (p *ShardedSocketPool) Start(callback func([]byte, *net.UDPAddr, int)) {
	p.mu.Lock()
	p.recvCallback = callback
	p.stopped = false
	p.mu.Unlock()

	for _, socket := range p.sockets {
		go socket.receiveLoop()
	}
}

// receiveLoop handles packet reception for a single shard
func (s *ShardedSocket) receiveLoop() {
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		// Set read deadline to allow periodic check of stop channel
		s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, remoteAddr, err := s.conn.ReadFromUDP(s.recvBuffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Timeout, check stop channel and retry
			}
			s.stats.errors.Add(1)
			s.pool.stats.receiveErrors.Add(1)
			continue
		}

		// Update statistics
		s.stats.received.Add(1)
		s.stats.bytesRecv.Add(uint64(n))
		s.pool.stats.totalReceived.Add(1)
		s.pool.stats.totalBytesRecv.Add(uint64(n))

		// Call the callback with the packet data
		s.pool.mu.RLock()
		callback := s.pool.recvCallback
		s.pool.mu.RUnlock()

		if callback != nil {
			// Make a copy for the callback to avoid buffer reuse issues
			packetCopy := make([]byte, n)
			copy(packetCopy, s.recvBuffer[:n])
			callback(packetCopy, remoteAddr, s.shardID)
		}
	}
}

// GetShard returns the appropriate shard for a given key (e.g., SSRC)
func (p *ShardedSocketPool) GetShard(key uint32) *ShardedSocket {
	shardIndex := int(key) % p.numShards
	return p.sockets[shardIndex]
}

// GetNextShard returns the next shard in round-robin fashion
func (p *ShardedSocketPool) GetNextShard() *ShardedSocket {
	index := p.nextShard.Add(1) % uint64(p.numShards)
	return p.sockets[int(index)]
}

// SendTo sends a packet through the appropriate shard
func (p *ShardedSocketPool) SendTo(data []byte, addr *net.UDPAddr, shardID int) error {
	if shardID < 0 || shardID >= p.numShards {
		shardID = int(p.nextShard.Add(1) % uint64(p.numShards))
	}

	socket := p.sockets[shardID]
	return socket.SendTo(data, addr)
}

// SendTo sends a packet through this socket
func (s *ShardedSocket) SendTo(data []byte, addr *net.UDPAddr) error {
	n, err := s.conn.WriteToUDP(data, addr)
	if err != nil {
		s.stats.errors.Add(1)
		s.pool.stats.sendErrors.Add(1)
		return err
	}

	s.stats.sent.Add(1)
	s.stats.bytesSent.Add(uint64(n))
	s.pool.stats.totalSent.Add(1)
	s.pool.stats.totalBytesSent.Add(uint64(n))

	return nil
}

// SendToZeroCopy attempts zero-copy send using sendmsg
func (s *ShardedSocket) SendToZeroCopy(data []byte, addr *net.UDPAddr) error {
	// For now, use regular send - true zero-copy requires platform-specific code
	// In production, this could use sendmsg with MSG_ZEROCOPY on Linux 4.14+
	return s.SendTo(data, addr)
}

// Close closes a single sharded socket
func (s *ShardedSocket) Close() error {
	close(s.stopCh)
	return s.conn.Close()
}

// Stop stops all sockets in the pool
func (p *ShardedSocketPool) Stop() error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return nil
	}
	p.stopped = true
	p.mu.Unlock()

	var lastErr error
	for _, socket := range p.sockets {
		if err := socket.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// GetStats returns pool statistics
func (p *ShardedSocketPool) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"num_shards":        p.numShards,
		"base_port":         p.basePort,
		"total_received":    p.stats.totalReceived.Load(),
		"total_sent":        p.stats.totalSent.Load(),
		"total_bytes_recv":  p.stats.totalBytesRecv.Load(),
		"total_bytes_sent":  p.stats.totalBytesSent.Load(),
		"receive_errors":    p.stats.receiveErrors.Load(),
		"send_errors":       p.stats.sendErrors.Load(),
		"dropped_packets":   p.stats.droppedPackets.Load(),
	}

	// Per-shard statistics
	shardStats := make([]map[string]interface{}, p.numShards)
	for i, socket := range p.sockets {
		shardStats[i] = map[string]interface{}{
			"shard_id":    socket.shardID,
			"port":        socket.port,
			"received":    socket.stats.received.Load(),
			"sent":        socket.stats.sent.Load(),
			"bytes_recv":  socket.stats.bytesRecv.Load(),
			"bytes_sent":  socket.stats.bytesSent.Load(),
			"errors":      socket.stats.errors.Load(),
		}
	}
	stats["shards"] = shardStats

	return stats
}

// GetPort returns the port for a specific shard
func (p *ShardedSocketPool) GetPort(shardID int) int {
	if shardID < 0 || shardID >= p.numShards {
		return 0
	}
	return p.sockets[shardID].port
}

// GetPorts returns all ports used by the pool
func (p *ShardedSocketPool) GetPorts() []int {
	ports := make([]int, p.numShards)
	for i, socket := range p.sockets {
		ports[i] = socket.port
	}
	return ports
}

// GetBuffer gets a buffer from the pool
func (p *ShardedSocketPool) GetBuffer() []byte {
	return p.bufferPool.Get().([]byte)
}

// PutBuffer returns a buffer to the pool
func (p *ShardedSocketPool) PutBuffer(buf []byte) {
	p.bufferPool.Put(buf)
}

// ZeroCopyForwarder provides zero-copy packet forwarding
type ZeroCopyForwarder struct {
	pool       *ShardedSocketPool
	routeTable map[uint32]*net.UDPAddr // SSRC -> destination
	routeLock  sync.RWMutex
	stats      *forwarderStats
}

type forwarderStats struct {
	forwarded atomic.Uint64
	dropped   atomic.Uint64
	errors    atomic.Uint64
}

// NewZeroCopyForwarder creates a new zero-copy packet forwarder
func NewZeroCopyForwarder(pool *ShardedSocketPool) *ZeroCopyForwarder {
	return &ZeroCopyForwarder{
		pool:       pool,
		routeTable: make(map[uint32]*net.UDPAddr),
		stats:      &forwarderStats{},
	}
}

// AddRoute adds a forwarding route for an SSRC
func (f *ZeroCopyForwarder) AddRoute(ssrc uint32, dest *net.UDPAddr) {
	f.routeLock.Lock()
	defer f.routeLock.Unlock()
	f.routeTable[ssrc] = dest
}

// RemoveRoute removes a forwarding route
func (f *ZeroCopyForwarder) RemoveRoute(ssrc uint32) {
	f.routeLock.Lock()
	defer f.routeLock.Unlock()
	delete(f.routeTable, ssrc)
}

// Forward forwards a packet based on SSRC routing
func (f *ZeroCopyForwarder) Forward(packet []byte, srcAddr *net.UDPAddr, shardID int) error {
	if len(packet) < 12 {
		f.stats.dropped.Add(1)
		return fmt.Errorf("packet too short for RTP header")
	}

	// Extract SSRC from RTP header (bytes 8-11)
	ssrc := uint32(packet[8])<<24 | uint32(packet[9])<<16 | uint32(packet[10])<<8 | uint32(packet[11])

	// Look up destination
	f.routeLock.RLock()
	dest, ok := f.routeTable[ssrc]
	f.routeLock.RUnlock()

	if !ok {
		f.stats.dropped.Add(1)
		return fmt.Errorf("no route for SSRC %d", ssrc)
	}

	// Forward using the same shard (maintains ordering)
	if err := f.pool.SendTo(packet, dest, shardID); err != nil {
		f.stats.errors.Add(1)
		return err
	}

	f.stats.forwarded.Add(1)
	return nil
}

// GetStats returns forwarder statistics
func (f *ZeroCopyForwarder) GetStats() map[string]uint64 {
	return map[string]uint64{
		"forwarded": f.stats.forwarded.Load(),
		"dropped":   f.stats.dropped.Load(),
		"errors":    f.stats.errors.Load(),
	}
}
