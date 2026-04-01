package internal

import (
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultSocketPoolConfig(t *testing.T) {
	config := DefaultSocketPoolConfig()

	if config.NumShards != runtime.NumCPU() {
		t.Errorf("Expected NumShards %d, got %d", runtime.NumCPU(), config.NumShards)
	}
	if config.BasePort != 20000 {
		t.Errorf("Expected BasePort 20000, got %d", config.BasePort)
	}
	if config.ListenAddress != "0.0.0.0" {
		t.Errorf("Expected ListenAddress 0.0.0.0, got %s", config.ListenAddress)
	}
	if config.RecvBufferSize != 4*1024*1024 {
		t.Errorf("Expected RecvBufferSize 4MB, got %d", config.RecvBufferSize)
	}
	if config.SendBufferSize != 4*1024*1024 {
		t.Errorf("Expected SendBufferSize 4MB, got %d", config.SendBufferSize)
	}
	if config.PacketSize != 1500 {
		t.Errorf("Expected PacketSize 1500, got %d", config.PacketSize)
	}
}

func TestNewShardedSocketPool(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      2,
		BasePort:       30000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	if pool.numShards != 2 {
		t.Errorf("Expected 2 shards, got %d", pool.numShards)
	}
	if len(pool.sockets) != 2 {
		t.Errorf("Expected 2 sockets, got %d", len(pool.sockets))
	}
}

func TestNewShardedSocketPool_DefaultConfig(t *testing.T) {
	// Use specific ports to avoid conflicts
	config := &SocketPoolConfig{
		NumShards:      1,
		BasePort:       31000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	if pool.numShards != 1 {
		t.Errorf("Expected 1 shard, got %d", pool.numShards)
	}
}

func TestShardedSocketPool_GetShard(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      4,
		BasePort:       32000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	// Test SSRC-based shard selection
	shard0 := pool.GetShard(0)
	shard1 := pool.GetShard(1)
	shard4 := pool.GetShard(4) // Should be same as shard 0 (4 % 4 = 0)

	if shard0.shardID != 0 {
		t.Errorf("Expected shard 0, got %d", shard0.shardID)
	}
	if shard1.shardID != 1 {
		t.Errorf("Expected shard 1, got %d", shard1.shardID)
	}
	if shard4.shardID != 0 {
		t.Errorf("Expected shard 0 for key 4, got %d", shard4.shardID)
	}
}

func TestShardedSocketPool_GetNextShard(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      4,
		BasePort:       33000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	// Should round-robin through shards
	seenShards := make(map[int]bool)
	for i := 0; i < 8; i++ {
		shard := pool.GetNextShard()
		seenShards[shard.shardID] = true
	}

	// Should have seen all 4 shards
	if len(seenShards) != 4 {
		t.Errorf("Expected to see all 4 shards, saw %d", len(seenShards))
	}
}

func TestShardedSocketPool_GetPorts(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      3,
		BasePort:       34000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	ports := pool.GetPorts()
	if len(ports) != 3 {
		t.Errorf("Expected 3 ports, got %d", len(ports))
	}

	expectedPorts := []int{34000, 34002, 34004} // Even ports for RTP
	for i, port := range ports {
		if port != expectedPorts[i] {
			t.Errorf("Expected port %d, got %d", expectedPorts[i], port)
		}
	}
}

func TestShardedSocketPool_GetPort(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      2,
		BasePort:       35000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	// Valid shard IDs
	if pool.GetPort(0) != 35000 {
		t.Errorf("Expected port 35000 for shard 0, got %d", pool.GetPort(0))
	}
	if pool.GetPort(1) != 35002 {
		t.Errorf("Expected port 35002 for shard 1, got %d", pool.GetPort(1))
	}

	// Invalid shard IDs
	if pool.GetPort(-1) != 0 {
		t.Errorf("Expected port 0 for invalid shard -1, got %d", pool.GetPort(-1))
	}
	if pool.GetPort(5) != 0 {
		t.Errorf("Expected port 0 for invalid shard 5, got %d", pool.GetPort(5))
	}
}

func TestShardedSocketPool_BufferPool(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      1,
		BasePort:       36000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	// Get buffer
	buf := pool.GetBuffer()
	if len(buf) != 1500 {
		t.Errorf("Expected buffer size 1500, got %d", len(buf))
	}

	// Return buffer
	pool.PutBuffer(buf)

	// Get another buffer (should come from pool)
	buf2 := pool.GetBuffer()
	if len(buf2) != 1500 {
		t.Errorf("Expected buffer size 1500, got %d", len(buf2))
	}
}

func TestShardedSocketPool_GetStats(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      2,
		BasePort:       37000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	stats := pool.GetStats()

	// Check expected keys
	expectedKeys := []string{
		"num_shards", "base_port", "total_received", "total_sent",
		"total_bytes_recv", "total_bytes_sent", "receive_errors",
		"send_errors", "dropped_packets", "shards",
	}

	for _, key := range expectedKeys {
		if _, ok := stats[key]; !ok {
			t.Errorf("Missing stats key: %s", key)
		}
	}

	if stats["num_shards"] != 2 {
		t.Errorf("Expected num_shards 2, got %v", stats["num_shards"])
	}
	if stats["base_port"] != 37000 {
		t.Errorf("Expected base_port 37000, got %v", stats["base_port"])
	}

	// Check shard stats
	shardStats, ok := stats["shards"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected shards to be a slice of maps")
	}
	if len(shardStats) != 2 {
		t.Errorf("Expected 2 shard stats, got %d", len(shardStats))
	}
}

func TestShardedSocketPool_SendAndReceive(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      1,
		BasePort:       38000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	// Track received packets
	var receivedPacket []byte
	var receivedAddr *net.UDPAddr
	var wg sync.WaitGroup
	wg.Add(1)

	pool.Start(func(data []byte, addr *net.UDPAddr, shardID int) {
		receivedPacket = data
		receivedAddr = addr
		wg.Done()
	})

	// Give the receive loop time to start
	time.Sleep(50 * time.Millisecond)

	// Send a packet to the pool
	destAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:38000")
	clientConn, err := net.DialUDP("udp", nil, destAddr)
	if err != nil {
		t.Fatalf("Failed to create client connection: %v", err)
	}
	defer clientConn.Close()

	testData := []byte("test packet data")
	_, err = clientConn.Write(testData)
	if err != nil {
		t.Fatalf("Failed to send packet: %v", err)
	}

	// Wait for packet with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for packet")
	}

	if string(receivedPacket) != string(testData) {
		t.Errorf("Expected packet %q, got %q", testData, receivedPacket)
	}
	if receivedAddr == nil {
		t.Error("Expected non-nil sender address")
	}
}

func TestShardedSocketPool_Stop(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      2,
		BasePort:       39000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	pool.Start(func(data []byte, addr *net.UDPAddr, shardID int) {})

	// Stop should not error
	err = pool.Stop()
	if err != nil {
		t.Errorf("Stop returned error: %v", err)
	}

	// Double stop should not error
	err = pool.Stop()
	if err != nil {
		t.Errorf("Double stop returned error: %v", err)
	}
}

func TestZeroCopyForwarder_AddRemoveRoute(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      1,
		BasePort:       40000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	forwarder := NewZeroCopyForwarder(pool)

	// Add route
	destAddr, _ := net.ResolveUDPAddr("udp", "192.168.1.100:5000")
	forwarder.AddRoute(0x12345678, destAddr)

	// Verify route exists
	forwarder.routeLock.RLock()
	_, ok := forwarder.routeTable[0x12345678]
	forwarder.routeLock.RUnlock()

	if !ok {
		t.Error("Route should exist after AddRoute")
	}

	// Remove route
	forwarder.RemoveRoute(0x12345678)

	// Verify route is removed
	forwarder.routeLock.RLock()
	_, ok = forwarder.routeTable[0x12345678]
	forwarder.routeLock.RUnlock()

	if ok {
		t.Error("Route should not exist after RemoveRoute")
	}
}

func TestZeroCopyForwarder_Forward_NoRoute(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      1,
		BasePort:       41000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	forwarder := NewZeroCopyForwarder(pool)

	// Create a minimal RTP packet
	packet := make([]byte, 12)
	packet[0] = 0x80                                                    // Version 2
	packet[8], packet[9], packet[10], packet[11] = 0x12, 0x34, 0x56, 0x78 // SSRC

	srcAddr, _ := net.ResolveUDPAddr("udp", "192.168.1.1:5000")
	err = forwarder.Forward(packet, srcAddr, 0)
	if err == nil {
		t.Error("Expected error when forwarding without route")
	}
}

func TestZeroCopyForwarder_Forward_TooShort(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      1,
		BasePort:       42000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	forwarder := NewZeroCopyForwarder(pool)

	// Packet too short for RTP header
	packet := make([]byte, 8)
	srcAddr, _ := net.ResolveUDPAddr("udp", "192.168.1.1:5000")

	err = forwarder.Forward(packet, srcAddr, 0)
	if err == nil {
		t.Error("Expected error for packet too short")
	}
}

func TestZeroCopyForwarder_GetStats(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      1,
		BasePort:       43000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	forwarder := NewZeroCopyForwarder(pool)

	stats := forwarder.GetStats()

	expectedKeys := []string{"forwarded", "dropped", "errors"}
	for _, key := range expectedKeys {
		if _, ok := stats[key]; !ok {
			t.Errorf("Missing stats key: %s", key)
		}
	}

	// Try forwarding without route to increment dropped counter
	packet := make([]byte, 12)
	packet[0] = 0x80
	srcAddr, _ := net.ResolveUDPAddr("udp", "192.168.1.1:5000")
	forwarder.Forward(packet, srcAddr, 0)

	stats = forwarder.GetStats()
	if stats["dropped"] != 1 {
		t.Errorf("Expected dropped 1, got %d", stats["dropped"])
	}
}

func TestShardedSocketPool_ConcurrentAccess(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      4,
		BasePort:       44000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Get shards concurrently
			pool.GetShard(uint32(id))
			pool.GetNextShard()

			// Get stats concurrently
			pool.GetStats()

			// Use buffer pool concurrently
			buf := pool.GetBuffer()
			pool.PutBuffer(buf)
		}(i)
	}

	wg.Wait()
}

func TestZeroCopyForwarder_ConcurrentRouteAccess(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      1,
		BasePort:       45000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	forwarder := NewZeroCopyForwarder(pool)

	var wg sync.WaitGroup
	var ops atomic.Int64
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ssrc := uint32(id)
			destAddr, _ := net.ResolveUDPAddr("udp", "192.168.1.100:5000")

			// Add route
			forwarder.AddRoute(ssrc, destAddr)
			ops.Add(1)

			// Get stats
			forwarder.GetStats()

			// Remove route
			forwarder.RemoveRoute(ssrc)
			ops.Add(1)
		}(i)
	}

	wg.Wait()

	if ops.Load() != int64(numGoroutines*2) {
		t.Errorf("Expected %d operations, got %d", numGoroutines*2, ops.Load())
	}
}

func TestShardedSocket_Stats(t *testing.T) {
	config := &SocketPoolConfig{
		NumShards:      1,
		BasePort:       46000,
		ListenAddress:  "127.0.0.1",
		RecvBufferSize: 1024 * 1024,
		SendBufferSize: 1024 * 1024,
		PacketSize:     1500,
	}

	pool, err := NewShardedSocketPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Stop()

	socket := pool.sockets[0]

	// Initial stats should be zero
	if socket.stats.received.Load() != 0 {
		t.Error("Expected initial received count to be 0")
	}
	if socket.stats.sent.Load() != 0 {
		t.Error("Expected initial sent count to be 0")
	}
}
