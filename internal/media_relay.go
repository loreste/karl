package internal

import (
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// MediaRelay handles per-session RTP/RTCP forwarding between two call legs.
type MediaRelay struct {
	session   *MediaSession
	stopCh    chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup
	OnPacket  func(session *MediaSession, srcLeg *CallLeg, packet []byte)
	OnTimeout func(session *MediaSession)
}

// NewMediaRelay creates a relay for the given session.
func NewMediaRelay(session *MediaSession) *MediaRelay {
	return &MediaRelay{
		session: session,
		stopCh:  make(chan struct{}),
	}
}

// Start launches forwarding goroutines for both directions.
func (r *MediaRelay) Start() {
	s := r.session
	s.mu.RLock()
	caller := s.CallerLeg
	callee := s.CalleeLeg
	mediaTimeout := s.MediaTimeout
	s.mu.RUnlock()

	if caller == nil || callee == nil || caller.Conn == nil || callee.Conn == nil {
		log.Printf("MediaRelay: cannot start, missing legs/conns for session %s", s.ID)
		return
	}

	r.wg.Add(2)
	go r.readLoop(caller, callee)
	go r.readLoop(callee, caller)

	if mediaTimeout > 0 {
		r.wg.Add(1)
		go r.timeoutLoop(time.Duration(mediaTimeout) * time.Second)
	}

	log.Printf("MediaRelay started for session %s (caller %s:%d → callee %s:%d)",
		s.ID, caller.LocalIP, caller.LocalPort, callee.LocalIP, callee.LocalPort)
}

// readLoop reads from src leg's socket and forwards to dst leg's remote address.
func (r *MediaRelay) readLoop(src, dst *CallLeg) {
	defer r.wg.Done()
	defer func() {
		if rv := recover(); rv != nil {
			log.Printf("MediaRelay: panic in readLoop for session %s: %v", r.session.ID, rv)
		}
	}()

	buf := make([]byte, 1500)
	latched := false

	// Read initial expected address from SDP (used by strict-source)
	src.mu.Lock()
	expectedIP := cloneIP(src.IP)
	expectedPort := src.Port
	src.mu.Unlock()

	for {
		select {
		case <-r.stopCh:
			return
		default:
		}

		src.Conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, remoteAddr, err := src.Conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-r.stopCh:
				return
			default:
				return
			}
		}

		// --- NAT / source validation ---

		// Strict source: reject packets not from the SDP-declared address
		if src.StrictSource && !src.MediaHandover && remoteAddr != nil {
			if expectedIP != nil && expectedPort != 0 {
				if !remoteAddr.IP.Equal(expectedIP) || remoteAddr.Port != expectedPort {
					continue // silently drop
				}
			}
		}

		// Port latching / symmetric RTP: learn actual address from first packet
		if (src.Symmetric || src.PortLatching) && remoteAddr != nil {
			if !latched {
				src.mu.Lock()
				src.IP = remoteAddr.IP
				src.Port = remoteAddr.Port
				src.mu.Unlock()
				latched = true
			} else if src.MediaHandover {
				// Media handover: allow re-latch when source changes
				src.mu.Lock()
				currentIP := src.IP
				currentPort := src.Port
				src.mu.Unlock()
				if !remoteAddr.IP.Equal(currentIP) || remoteAddr.Port != currentPort {
					src.mu.Lock()
					src.IP = remoteAddr.IP
					src.Port = remoteAddr.Port
					src.mu.Unlock()
					log.Printf("MediaRelay: media-handover re-latch for session %s: %s", r.session.ID, remoteAddr)
				}
			}
		}

		// Media blocking: drop silently
		src.mu.Lock()
		blocked := src.MediaBlocked
		silenced := src.Silenced
		src.mu.Unlock()
		if blocked {
			continue
		}

		// Silencing: zero payload but keep RTP header (first 12 bytes)
		if silenced && n > 12 {
			for i := 12; i < n; i++ {
				buf[i] = 0
			}
		}

		// Determine destination
		dst.mu.Lock()
		dstIP := dst.IP
		dstPort := dst.Port
		dst.mu.Unlock()
		if dstIP == nil || dstPort == 0 {
			continue
		}

		dstAddr := &net.UDPAddr{IP: dstIP, Port: dstPort}

		_, writeErr := dst.Conn.WriteToUDP(buf[:n], dstAddr)
		if writeErr != nil {
			atomic.AddUint32(&src.PacketsLost, 1)
			continue
		}

		// Stats
		atomic.AddUint64(&src.PacketsRecv, 1)
		atomic.AddUint64(&src.BytesRecv, uint64(n))
		atomic.AddUint64(&dst.PacketsSent, 1)
		atomic.AddUint64(&dst.BytesSent, uint64(n))
		src.mu.Lock()
		src.LastActivity = time.Now()
		src.mu.Unlock()

		// Packet hook (recording, metrics)
		if r.OnPacket != nil {
			pkt := make([]byte, n)
			copy(pkt, buf[:n])
			r.OnPacket(r.session, src, pkt)
		}
	}
}

// timeoutLoop monitors media activity and fires OnTimeout if both legs are idle.
func (r *MediaRelay) timeoutLoop(timeout time.Duration) {
	defer r.wg.Done()
	defer func() {
		if rv := recover(); rv != nil {
			log.Printf("MediaRelay: panic in timeoutLoop for session %s: %v", r.session.ID, rv)
		}
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			s := r.session
			s.mu.RLock()
			caller := s.CallerLeg
			callee := s.CalleeLeg
			s.mu.RUnlock()

			if caller == nil || callee == nil {
				continue
			}

			now := time.Now()
			caller.mu.Lock()
			callerIdle := !caller.LastActivity.IsZero() && now.Sub(caller.LastActivity) > timeout
			caller.mu.Unlock()
			callee.mu.Lock()
			calleeIdle := !callee.LastActivity.IsZero() && now.Sub(callee.LastActivity) > timeout
			callee.mu.Unlock()

			if callerIdle && calleeIdle {
				log.Printf("MediaRelay: media timeout for session %s", r.session.ID)
				if r.OnTimeout != nil {
					r.OnTimeout(s)
				}
				r.Stop()
				return
			}
		}
	}
}

// Stop shuts down the relay and waits for goroutines to exit.
func (r *MediaRelay) Stop() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
	r.wg.Wait()
}

func cloneIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}
