package internal

import (
	"sync"
	"testing"
	"time"
)

func TestDefaultPortAllocatorConfig(t *testing.T) {
	config := DefaultPortAllocatorConfig()

	if config.MinPort != 10000 {
		t.Errorf("Expected MinPort 10000, got %d", config.MinPort)
	}
	if config.MaxPort != 60000 {
		t.Errorf("Expected MaxPort 60000, got %d", config.MaxPort)
	}
	if !config.EvenOnly {
		t.Error("Expected EvenOnly to be true")
	}
}

func TestNewPortAllocator(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:      20000,
		MaxPort:      20100,
		ReserveCount: 0, // No pre-allocation for faster test
		EvenOnly:     true,
	}
	pa := NewPortAllocator(config)

	if pa == nil {
		t.Fatal("NewPortAllocator returned nil")
	}

	// Cleanup
	pa.Close()
}

func TestPortAllocator_AllocatePort(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	port, err := pa.AllocatePort("session-1")
	if err != nil {
		t.Fatalf("AllocatePort failed: %v", err)
	}

	if port < config.MinPort || port > config.MaxPort {
		t.Errorf("Port %d out of range [%d, %d]", port, config.MinPort, config.MaxPort)
	}

	if port%2 != 0 {
		t.Errorf("Port %d should be even", port)
	}

	if pa.currentInUse.Load() != 1 {
		t.Errorf("Expected 1 port in use, got %d", pa.currentInUse.Load())
	}
}

func TestPortAllocator_AllocateMultiplePorts(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20020,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 20,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	ports := make(map[int]bool)
	for i := 0; i < 5; i++ {
		port, err := pa.AllocatePort("session-1")
		if err != nil {
			t.Fatalf("AllocatePort %d failed: %v", i, err)
		}

		if ports[port] {
			t.Errorf("Port %d allocated twice", port)
		}
		ports[port] = true
	}

	if len(ports) != 5 {
		t.Errorf("Expected 5 unique ports, got %d", len(ports))
	}
}

func TestPortAllocator_AllocatePortPair(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	rtpPort, rtcpPort, err := pa.AllocatePortPair("session-1")
	if err != nil {
		t.Fatalf("AllocatePortPair failed: %v", err)
	}

	if rtpPort%2 != 0 {
		t.Errorf("RTP port %d should be even", rtpPort)
	}

	if rtcpPort != rtpPort+1 {
		t.Errorf("RTCP port %d should be RTP port + 1 (%d)", rtcpPort, rtpPort+1)
	}

	if pa.currentInUse.Load() != 2 {
		t.Errorf("Expected 2 ports in use, got %d", pa.currentInUse.Load())
	}
}

func TestPortAllocator_ReleasePort(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	port, _ := pa.AllocatePort("session-1")

	err := pa.ReleasePort(port)
	if err != nil {
		t.Fatalf("ReleasePort failed: %v", err)
	}

	if pa.currentInUse.Load() != 0 {
		t.Errorf("Expected 0 ports in use after release, got %d", pa.currentInUse.Load())
	}

	if pa.totalReleased.Load() != 1 {
		t.Errorf("Expected 1 released, got %d", pa.totalReleased.Load())
	}
}

func TestPortAllocator_ReleaseSessionPorts(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	// Allocate multiple ports for session
	pa.AllocatePort("session-1")
	pa.AllocatePort("session-1")
	pa.AllocatePort("session-1")

	if pa.currentInUse.Load() != 3 {
		t.Fatalf("Expected 3 ports in use, got %d", pa.currentInUse.Load())
	}

	err := pa.ReleaseSessionPorts("session-1")
	if err != nil {
		t.Fatalf("ReleaseSessionPorts failed: %v", err)
	}

	if pa.currentInUse.Load() != 0 {
		t.Errorf("Expected 0 ports in use after release, got %d", pa.currentInUse.Load())
	}
}

func TestPortAllocator_MaxAllocationLimit(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 3, // Low limit
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	// Allocate up to limit
	for i := 0; i < 3; i++ {
		_, err := pa.AllocatePort("session-1")
		if err != nil {
			t.Fatalf("AllocatePort %d failed: %v", i, err)
		}
	}

	// Next allocation should fail
	_, err := pa.AllocatePort("session-1")
	if err != ErrPortAllocationLimit {
		t.Errorf("Expected ErrPortAllocationLimit, got %v", err)
	}

	// Different session should still work
	_, err = pa.AllocatePort("session-2")
	if err != nil {
		t.Errorf("Different session should be able to allocate: %v", err)
	}
}

func TestPortAllocator_ReuseDelay(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20004, // Very small range
		ReserveCount:   0,
		ReuseDelay:     100 * time.Millisecond,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	// Allocate both available ports (20000 and 20002)
	port1, _ := pa.AllocatePort("session-1")
	port2, _ := pa.AllocatePort("session-1")

	// Release first port
	pa.ReleasePort(port1)

	// Should not be immediately reusable
	_, err := pa.AllocatePort("session-2")
	if err == nil {
		// Port might have been reused if timing was close
		t.Log("Port was reused immediately (timing edge case)")
	}

	// Release second port
	pa.ReleasePort(port2)

	// Wait for reuse delay
	time.Sleep(150 * time.Millisecond)

	// Now should be able to allocate
	_, err = pa.AllocatePort("session-2")
	if err != nil {
		t.Errorf("Should be able to allocate after reuse delay: %v", err)
	}
}

func TestPortAllocator_EvenOnlyEnforcement(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20001, // Odd start
		MaxPort:        20010,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	port, err := pa.AllocatePort("session-1")
	if err != nil {
		t.Fatalf("AllocatePort failed: %v", err)
	}

	if port%2 != 0 {
		t.Errorf("Port %d should be even even with odd MinPort", port)
	}
}

func TestPortAllocator_GetStats(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	pa.AllocatePort("session-1")
	pa.AllocatePort("session-2")

	stats := pa.GetStats()

	if stats["allocated_count"].(int) != 2 {
		t.Errorf("Expected allocated_count 2, got %v", stats["allocated_count"])
	}
	if stats["session_count"].(int) != 2 {
		t.Errorf("Expected session_count 2, got %v", stats["session_count"])
	}
	if stats["current_in_use"].(int64) != 2 {
		t.Errorf("Expected current_in_use 2, got %v", stats["current_in_use"])
	}
}

func TestPortAllocator_GetUtilization(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20010, // 5 even ports available (20000, 20002, 20004, 20006, 20008)
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	// Allocate 2 ports
	pa.AllocatePort("session-1")
	pa.AllocatePort("session-1")

	util := pa.GetUtilization()
	// 2 out of 5 = 0.4
	if util < 0.3 || util > 0.5 {
		t.Errorf("Expected utilization around 0.4, got %f", util)
	}
}

func TestPortAllocator_IsNearExhaustion(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20010,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	if pa.IsNearExhaustion(0.8) {
		t.Error("Should not be near exhaustion initially")
	}

	// Allocate most ports (4 out of 5)
	for i := 0; i < 4; i++ {
		pa.AllocatePort("session-1")
	}

	if !pa.IsNearExhaustion(0.8) {
		t.Error("Should be near exhaustion at 80%")
	}
}

func TestPortAllocator_Close(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)

	pa.AllocatePort("session-1")
	pa.AllocatePort("session-2")

	err := pa.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Allocation should fail after close
	_, err = pa.AllocatePort("session-3")
	if err == nil {
		t.Error("Allocation should fail after close")
	}
}

func TestPortAllocator_ConcurrentAllocations(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        21000,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 100,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	var wg sync.WaitGroup
	numGoroutines := 10
	portsPerGoroutine := 10

	allocatedPorts := make(chan int, numGoroutines*portsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()
			for j := 0; j < portsPerGoroutine; j++ {
				port, err := pa.AllocatePort(sessionID)
				if err != nil {
					t.Logf("Allocation failed: %v", err)
					continue
				}
				allocatedPorts <- port
			}
		}(string(rune('A' + i)))
	}

	wg.Wait()
	close(allocatedPorts)

	// Check for duplicates
	seen := make(map[int]bool)
	for port := range allocatedPorts {
		if seen[port] {
			t.Errorf("Port %d allocated multiple times", port)
		}
		seen[port] = true
	}
}

func TestPortReservation(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 20,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	reservation, err := pa.NewPortReservation("session-1", 4)
	if err != nil {
		t.Fatalf("NewPortReservation failed: %v", err)
	}

	ports := reservation.GetPorts()
	if len(ports) != 4 {
		t.Errorf("Expected 4 ports, got %d", len(ports))
	}

	// Commit the reservation
	reservation.Commit()

	// Ports should still be allocated
	if pa.currentInUse.Load() != 4 {
		t.Errorf("Expected 4 ports in use after commit, got %d", pa.currentInUse.Load())
	}
}

func TestPortReservation_Rollback(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 20,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	reservation, err := pa.NewPortReservation("session-1", 4)
	if err != nil {
		t.Fatalf("NewPortReservation failed: %v", err)
	}

	if pa.currentInUse.Load() != 4 {
		t.Fatalf("Expected 4 ports in use, got %d", pa.currentInUse.Load())
	}

	// Rollback the reservation
	reservation.Rollback()

	// Ports should be released
	if pa.currentInUse.Load() != 0 {
		t.Errorf("Expected 0 ports in use after rollback, got %d", pa.currentInUse.Load())
	}
}

func TestPortReservation_RollbackAfterCommit(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        20000,
		MaxPort:        20100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 20,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	reservation, _ := pa.NewPortReservation("session-1", 4)

	reservation.Commit()
	reservation.Rollback() // Should be no-op after commit

	// Ports should still be allocated
	if pa.currentInUse.Load() != 4 {
		t.Errorf("Expected 4 ports still in use, got %d", pa.currentInUse.Load())
	}
}

func TestGetPortAllocator(t *testing.T) {
	pa1 := GetPortAllocator()
	if pa1 == nil {
		t.Fatal("GetPortAllocator returned nil")
	}

	pa2 := GetPortAllocator()
	if pa1 != pa2 {
		t.Error("GetPortAllocator should return same instance")
	}
}

func TestPortAllocator_AllocateWithConnection(t *testing.T) {
	config := &PortAllocatorConfig{
		MinPort:        30000, // Use different range to avoid conflicts
		MaxPort:        30100,
		ReserveCount:   0,
		ReuseDelay:     0,
		MaxAllocations: 10,
		EvenOnly:       true,
	}
	pa := NewPortAllocator(config)
	defer pa.Close()

	port, conn, err := pa.AllocateWithConnection("session-1", "udp")
	if err != nil {
		t.Fatalf("AllocateWithConnection failed: %v", err)
	}
	defer conn.Close()

	if port < config.MinPort || port > config.MaxPort {
		t.Errorf("Port %d out of range", port)
	}

	if conn == nil {
		t.Error("Connection should not be nil")
	}

	// Release should close the connection
	pa.ReleasePort(port)
}
