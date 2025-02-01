package internal

import (
	"log"
	"runtime"
	"sync"
)

// WorkerPool settings
var (
	workerPoolSize = runtime.NumCPU() * 2    // Number of concurrent workers (adjust as needed)
	rtpJobs        = make(chan []byte, 1000) // Buffered channel for incoming RTP packets
	wg             sync.WaitGroup
)

// InitWorkerPool initializes a pool of workers to process RTP packets concurrently
func InitWorkerPool() {
	log.Printf("Initializing RTP worker pool with %d workers", workerPoolSize)

	for i := 0; i < workerPoolSize; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for packet := range rtpJobs {
				processRTPPacket(packet, workerID)
			}
		}(i)
	}
}

// processRTPPacket handles an RTP packet (can include transcoding, forwarding, etc.)
func processRTPPacket(packet []byte, workerID int) {
	// Capture packet for debugging if PCAP logging is enabled
	CapturePacket(packet)

	// Placeholder for RTP processing logic
	log.Printf("Worker %d processed RTP packet, size: %d bytes", workerID, len(packet))
}

// AddRTPJob sends an RTP packet to the worker pool for processing
func AddRTPJob(packet []byte) {
	select {
	case rtpJobs <- append([]byte(nil), packet...): // Copy packet before sending to avoid data race
	default:
		log.Println("RTP job queue is full, packet dropped")
	}
}

// StopWorkerPool shuts down the worker pool gracefully
func StopWorkerPool() {
	close(rtpJobs)
	wg.Wait()
	log.Println("RTP worker pool stopped")
}
