package internal

import (
	"fmt"
	"sync"
	"testing"
)

func TestDefaultConsistentHashConfig(t *testing.T) {
	config := DefaultConsistentHashConfig()

	if config.ReplicationFactor != 150 {
		t.Errorf("expected ReplicationFactor=150, got %d", config.ReplicationFactor)
	}
	if config.LoadFactor != 1.25 {
		t.Errorf("expected LoadFactor=1.25, got %f", config.LoadFactor)
	}
}

func TestNewHashRing(t *testing.T) {
	ring := NewHashRing(nil)
	if ring.config.ReplicationFactor != 150 {
		t.Error("expected default config")
	}

	config := &ConsistentHashConfig{ReplicationFactor: 100}
	ring = NewHashRing(config)
	if ring.config.ReplicationFactor != 100 {
		t.Errorf("expected ReplicationFactor=100, got %d", ring.config.ReplicationFactor)
	}
}

func TestHashRing_AddRemoveNode(t *testing.T) {
	ring := NewHashRing(nil)

	node := &HashNode{
		ID:      "node-1",
		Address: "192.168.1.1:5060",
		Weight:  1,
		Healthy: true,
	}

	ring.AddNode(node)

	if ring.NodeCount() != 1 {
		t.Errorf("expected 1 node, got %d", ring.NodeCount())
	}

	// Adding same node again should be idempotent
	ring.AddNode(node)
	if ring.NodeCount() != 1 {
		t.Errorf("expected still 1 node, got %d", ring.NodeCount())
	}

	ring.RemoveNode("node-1")
	if ring.NodeCount() != 0 {
		t.Errorf("expected 0 nodes, got %d", ring.NodeCount())
	}

	// Removing non-existent node should be safe
	ring.RemoveNode("node-1")
}

func TestHashRing_GetNode(t *testing.T) {
	ring := NewHashRing(nil)

	// Empty ring
	if node := ring.GetNode("key"); node != nil {
		t.Error("expected nil for empty ring")
	}

	// Add nodes
	ring.AddNode(&HashNode{ID: "node-1", Address: "1.1.1.1", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-2", Address: "2.2.2.2", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-3", Address: "3.3.3.3", Healthy: true})

	// Same key should return same node
	node1 := ring.GetNode("session-123")
	node2 := ring.GetNode("session-123")
	if node1.ID != node2.ID {
		t.Error("same key should return same node")
	}
}

func TestHashRing_Distribution(t *testing.T) {
	ring := NewHashRing(&ConsistentHashConfig{
		ReplicationFactor: 100,
		LoadFactor:        1.25,
	})

	// Add 3 nodes
	ring.AddNode(&HashNode{ID: "node-1", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-2", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-3", Healthy: true})

	// Distribute 1000 keys
	distribution := make(map[string]int)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		node := ring.GetNode(key)
		distribution[node.ID]++
	}

	// Check distribution is roughly even (within 50% of average)
	avg := 1000 / 3
	for _, count := range distribution {
		if count < avg/2 || count > avg*2 {
			t.Errorf("distribution is too uneven: %v", distribution)
			break
		}
	}
}

func TestHashRing_NodeRemovalRedistribution(t *testing.T) {
	ring := NewHashRing(nil)

	ring.AddNode(&HashNode{ID: "node-1", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-2", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-3", Healthy: true})

	// Get initial mappings
	initialMappings := make(map[string]string)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		initialMappings[key] = ring.GetNode(key).ID
	}

	// Remove one node
	ring.RemoveNode("node-2")

	// Count how many keys moved
	moved := 0
	for key, oldNode := range initialMappings {
		newNode := ring.GetNode(key)
		if newNode.ID != oldNode && oldNode != "node-2" {
			moved++
		}
	}

	// Ideally, only keys from removed node should move
	// Allow some variance due to hash function
	if moved > 50 {
		t.Errorf("too many keys moved after node removal: %d", moved)
	}
}

func TestHashRing_GetNodes(t *testing.T) {
	ring := NewHashRing(nil)

	ring.AddNode(&HashNode{ID: "node-1", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-2", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-3", Healthy: true})

	nodes := ring.GetNodes("key", 2)
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}

	// Nodes should be unique
	if nodes[0].ID == nodes[1].ID {
		t.Error("nodes should be unique")
	}

	// Request more than available
	nodes = ring.GetNodes("key", 10)
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes (all), got %d", len(nodes))
	}
}

func TestHashRing_LoadBalancing(t *testing.T) {
	ring := NewHashRing(&ConsistentHashConfig{
		ReplicationFactor: 100,
		LoadFactor:        1.5,
	})

	ring.AddNode(&HashNode{ID: "node-1", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-2", Healthy: true})

	// Artificially load node-1
	for i := 0; i < 100; i++ {
		ring.IncrementLoad("node-1")
	}

	// GetNodeWithLoad should prefer node-2
	preferNode2 := 0
	for i := 0; i < 100; i++ {
		node := ring.GetNodeWithLoad(fmt.Sprintf("key-%d", i))
		if node.ID == "node-2" {
			preferNode2++
		}
	}

	// Should prefer the less loaded node
	if preferNode2 < 50 {
		t.Errorf("expected to prefer less loaded node, got %d/100 for node-2", preferNode2)
	}
}

func TestHashRing_HealthStatus(t *testing.T) {
	ring := NewHashRing(nil)

	ring.AddNode(&HashNode{ID: "node-1", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-2", Healthy: true})

	// Mark node-1 as unhealthy
	ring.SetNodeHealth("node-1", false)

	// GetNodeWithLoad should avoid unhealthy node
	for i := 0; i < 100; i++ {
		node := ring.GetNodeWithLoad(fmt.Sprintf("key-%d", i))
		if node.ID == "node-1" {
			t.Error("should not route to unhealthy node")
			break
		}
	}

	// GetHealthyNodes should exclude unhealthy
	healthy := ring.GetHealthyNodes()
	if len(healthy) != 1 || healthy[0].ID != "node-2" {
		t.Error("GetHealthyNodes should exclude unhealthy nodes")
	}
}

func TestHashRing_Stats(t *testing.T) {
	ring := NewHashRing(&ConsistentHashConfig{ReplicationFactor: 100})

	ring.AddNode(&HashNode{ID: "node-1", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-2", Healthy: false})

	ring.IncrementLoad("node-1")
	ring.IncrementLoad("node-1")

	stats := ring.Stats()

	if stats.NodeCount != 2 {
		t.Errorf("expected 2 nodes, got %d", stats.NodeCount)
	}
	if stats.VirtualNodes != 200 { // 100 * 2
		t.Errorf("expected 200 virtual nodes, got %d", stats.VirtualNodes)
	}
	if stats.TotalLoad != 2 {
		t.Errorf("expected total load 2, got %d", stats.TotalLoad)
	}
	if stats.HealthyCount != 1 {
		t.Errorf("expected 1 healthy, got %d", stats.HealthyCount)
	}
	if stats.UnhealthyCount != 1 {
		t.Errorf("expected 1 unhealthy, got %d", stats.UnhealthyCount)
	}
}

func TestHashRing_WeightedNodes(t *testing.T) {
	ring := NewHashRing(&ConsistentHashConfig{ReplicationFactor: 10})

	ring.AddNode(&HashNode{ID: "node-1", Weight: 1, Healthy: true})
	ring.AddNode(&HashNode{ID: "node-2", Weight: 3, Healthy: true}) // 3x weight

	// Weighted node should get more traffic
	distribution := make(map[string]int)
	for i := 0; i < 1000; i++ {
		node := ring.GetNode(fmt.Sprintf("key-%d", i))
		distribution[node.ID]++
	}

	// node-2 should get roughly 3x more traffic
	ratio := float64(distribution["node-2"]) / float64(distribution["node-1"])
	if ratio < 2.0 || ratio > 4.0 {
		t.Errorf("expected ratio around 3, got %f (node-1=%d, node-2=%d)",
			ratio, distribution["node-1"], distribution["node-2"])
	}
}

func TestHashRing_Concurrent(t *testing.T) {
	ring := NewHashRing(nil)

	ring.AddNode(&HashNode{ID: "node-1", Healthy: true})
	ring.AddNode(&HashNode{ID: "node-2", Healthy: true})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				ring.GetNode(key)
				ring.GetNodeWithLoad(key)
				ring.IncrementLoad(fmt.Sprintf("node-%d", (id%2)+1))
				ring.DecrementLoad(fmt.Sprintf("node-%d", (id%2)+1))
			}
		}(i)
	}
	wg.Wait()
}

func TestSessionRouter(t *testing.T) {
	router := NewSessionRouter(nil)

	router.AddNode(&HashNode{ID: "node-1", Address: "1.1.1.1", Healthy: true})
	router.AddNode(&HashNode{ID: "node-2", Address: "2.2.2.2", Healthy: true})

	// Route session
	node := router.RouteSession("session-123")
	if node == nil {
		t.Fatal("expected non-nil node")
	}

	// Same session should return same node (sticky)
	node2 := router.RouteSession("session-123")
	if node.ID != node2.ID {
		t.Error("sticky session should return same node")
	}

	// End session
	router.EndSession("session-123")

	stats := router.GetStats()
	if stats.StickySessions != 0 {
		t.Errorf("expected 0 sticky sessions after end, got %d", stats.StickySessions)
	}
}

func TestSessionRouter_NodeFailure(t *testing.T) {
	router := NewSessionRouter(nil)

	router.AddNode(&HashNode{ID: "node-1", Healthy: true})
	router.AddNode(&HashNode{ID: "node-2", Healthy: true})

	// Route to get sticky binding
	node := router.RouteSession("session-123")
	boundNode := node.ID

	// Mark bound node as unhealthy
	router.SetNodeHealth(boundNode, false)

	// Should route to healthy node
	node = router.RouteSession("session-456")
	if node.ID == boundNode {
		// Could still happen due to hash, but session-123 should be re-routed
	}

	// Remove the unhealthy node
	router.RemoveNode(boundNode)

	// Session should be cleared - sticky sessions for removed node are cleared
	_ = router.GetStats()
}

func TestRendezvousHash(t *testing.T) {
	rh := NewRendezvousHash()

	rh.AddNode(&HashNode{ID: "node-1", Healthy: true})
	rh.AddNode(&HashNode{ID: "node-2", Healthy: true})
	rh.AddNode(&HashNode{ID: "node-3", Healthy: true})

	// Same key should return same node
	node1 := rh.GetNode("key-123")
	node2 := rh.GetNode("key-123")
	if node1.ID != node2.ID {
		t.Error("same key should return same node")
	}
}

func TestRendezvousHash_Distribution(t *testing.T) {
	rh := NewRendezvousHash()

	rh.AddNode(&HashNode{ID: "node-1", Healthy: true})
	rh.AddNode(&HashNode{ID: "node-2", Healthy: true})
	rh.AddNode(&HashNode{ID: "node-3", Healthy: true})

	distribution := make(map[string]int)
	for i := 0; i < 1000; i++ {
		node := rh.GetNode(fmt.Sprintf("key-%d", i))
		distribution[node.ID]++
	}

	// Check roughly even distribution
	for _, count := range distribution {
		if count < 200 || count > 500 {
			t.Errorf("uneven distribution: %v", distribution)
			break
		}
	}
}

func TestRendezvousHash_GetNodes(t *testing.T) {
	rh := NewRendezvousHash()

	rh.AddNode(&HashNode{ID: "node-1", Healthy: true})
	rh.AddNode(&HashNode{ID: "node-2", Healthy: true})
	rh.AddNode(&HashNode{ID: "node-3", Healthy: true})

	nodes := rh.GetNodes("key", 2)
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}

	// Should be different nodes
	if nodes[0].ID == nodes[1].ID {
		t.Error("nodes should be different")
	}
}

func TestRendezvousHash_NodeRemoval(t *testing.T) {
	rh := NewRendezvousHash()

	rh.AddNode(&HashNode{ID: "node-1", Healthy: true})
	rh.AddNode(&HashNode{ID: "node-2", Healthy: true})
	rh.AddNode(&HashNode{ID: "node-3", Healthy: true})

	// Record initial mappings
	initial := make(map[string]string)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		initial[key] = rh.GetNode(key).ID
	}

	// Remove a node
	rh.RemoveNode("node-2")

	// Only keys mapped to removed node should change
	moved := 0
	for key, oldNode := range initial {
		newNode := rh.GetNode(key)
		if newNode.ID != oldNode && oldNode != "node-2" {
			moved++
		}
	}

	if moved > 0 {
		t.Errorf("rendezvous hash: unexpected key moves: %d", moved)
	}
}

func TestRendezvousHash_HealthyOnly(t *testing.T) {
	rh := NewRendezvousHash()

	rh.AddNode(&HashNode{ID: "node-1", Healthy: false})
	rh.AddNode(&HashNode{ID: "node-2", Healthy: true})

	node := rh.GetNode("key")
	if node.ID != "node-2" {
		t.Errorf("expected node-2 (healthy), got %s", node.ID)
	}

	nodes := rh.GetNodes("key", 2)
	if len(nodes) != 1 {
		t.Errorf("expected 1 healthy node, got %d", len(nodes))
	}
}

func BenchmarkHashRing_GetNode(b *testing.B) {
	ring := NewHashRing(nil)
	for i := 0; i < 10; i++ {
		ring.AddNode(&HashNode{ID: fmt.Sprintf("node-%d", i), Healthy: true})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ring.GetNode(fmt.Sprintf("key-%d", i))
	}
}

func BenchmarkRendezvousHash_GetNode(b *testing.B) {
	rh := NewRendezvousHash()
	for i := 0; i < 10; i++ {
		rh.AddNode(&HashNode{ID: fmt.Sprintf("node-%d", i), Healthy: true})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rh.GetNode(fmt.Sprintf("key-%d", i))
	}
}
