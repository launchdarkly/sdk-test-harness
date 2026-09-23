package harness

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// TCPProxy is a TCP forwarder that sits between a client and a backend
// server. Tests use it to simulate a network outage for the backend.
// Break drops active connections and refuses new ones. Restore resumes
// normal forwarding.
//
// The proxy listens on the loopback interface. This matches the
// persistence tests, which already require the data stores and the SDK
// test service to share a host with the test harness.
type TCPProxy struct {
	backendAddr string
	listener    net.Listener

	mu     sync.Mutex
	broken bool
	closed bool
	conns  map[net.Conn]struct{}

	// armThreshold is > 0 while BreakAfterBytes is armed. Each connection
	// tracks its own client-to-backend byte count since arming. The
	// first connection to reach armThreshold trips Break for the whole
	// proxy. armEpoch changes every time BreakAfterBytes is called. This
	// makes an already-open connection restart its count from zero
	// instead of counting bytes it forwarded before arming.
	armThreshold int64
	armEpoch     int64
}

// NewTCPProxy starts a proxy on an ephemeral loopback port. The proxy
// forwards TCP traffic to backendAddr (host:port).
func NewTCPProxy(backendAddr string) (*TCPProxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("tcp proxy could not listen: %w", err)
	}
	p := &TCPProxy{
		backendAddr: backendAddr,
		listener:    listener,
		conns:       make(map[net.Conn]struct{}),
	}
	go p.acceptLoop()
	return p, nil
}

// Addr returns the host:port address the proxy listens on.
func (p *TCPProxy) Addr() string {
	return p.listener.Addr().String()
}

// Break starts a simulated outage. It drops all active connections and
// makes the proxy close each new connection immediately.
func (p *TCPProxy) Break() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.armThreshold = 0
	p.broken = true
	p.dropConnsLocked()
}

// BreakAfterBytes arms a delayed outage. Forwarding resumes (or keeps
// running) like after Restore. Once any single connection forwards n or
// more client-to-backend bytes in one sustained transfer, the proxy
// cuts that transfer mid-stream. The proxy then enters the same broken
// state as Break: all connections drop and new ones are refused. Use
// Broken to check whether the cut has fired yet.
//
// The threshold applies to a single sustained transfer on one
// connection, not to the connection's lifetime total. An idle gap of
// about 100ms or more between reads starts a new count from zero. This
// stops periodic small traffic, for example an availability check on a
// keep-alive connection, from building up across gaps and tripping the
// cut on its own.
//
// n <= 0 disarms the proxy. It just resumes forwarding, the same as
// Restore.
func (p *TCPProxy) BreakAfterBytes(n int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.broken = false
	p.armThreshold = n
	p.armEpoch++
}

// Broken reports whether the proxy is currently refusing traffic. This
// is true after Break, after Close, and after a BreakAfterBytes trip.
func (p *TCPProxy) Broken() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.broken || p.closed
}

// Restore ends a simulated outage. New connections forward to the
// backend again. Connections dropped by Break stay closed. Restore
// also disarms a pending BreakAfterBytes threshold.
func (p *TCPProxy) Restore() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.broken = false
	p.armThreshold = 0
}

// Close shuts the proxy down. Do not use the proxy after Close.
// Close is safe to call more than once.
func (p *TCPProxy) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	_ = p.listener.Close()
	p.dropConnsLocked()
}

func (p *TCPProxy) dropConnsLocked() {
	for c := range p.conns {
		// Set linger to 0 so the kernel sends RST instead of a graceful
		// FIN. A graceful close can still flush a buffered backlog to the
		// backend after Break. That would let more of a cut write back
		// land than the client actually sent before the outage.
		if tc, ok := c.(*net.TCPConn); ok {
			_ = tc.SetLinger(0)
		}
		_ = c.Close()
	}
	p.conns = make(map[net.Conn]struct{})
}

// copyChunkSize bounds how much data copyClientToBackend reads before it
// re-checks the armed threshold. A small chunk keeps a BreakAfterBytes
// cut close to the byte boundary.
const copyChunkSize = 4096

// burstIdleGap is the idle gap between reads that starts a new burst
// for BreakAfterBytes accounting. A sustained transfer, such as a store
// write back, has sub-millisecond gaps between reads on loopback. It
// still accumulates past the threshold. Periodic small traffic, for
// example an availability check every 500ms, has gaps at or above this
// value. It can never accumulate across gaps to trip the cut.
const burstIdleGap = 100 * time.Millisecond

// armState returns the current threshold and epoch. A threshold of 0
// means BreakAfterBytes is not armed.
func (p *TCPProxy) armState() (threshold int64, epoch int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.armThreshold, p.armEpoch
}

// tripIfArmed trips Break for the whole proxy, but only if epoch is
// still the current arm epoch and the proxy is still armed. The caller
// takes a threshold/epoch snapshot before the blocking read completes.
// A concurrent Restore, or a new BreakAfterBytes, can land in between.
// Without this recheck under the lock, the trip would override that
// Restore with a spurious outage. It returns false if the arm from
// epoch no longer applies. In that case the caller must not break.
func (p *TCPProxy) tripIfArmed(epoch int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.armEpoch != epoch || p.armThreshold <= 0 {
		return false
	}
	p.armThreshold = 0
	p.broken = true
	p.dropConnsLocked()
	return true
}

// copyClientToBackend forwards clientConn's bytes to backendConn. While
// BreakAfterBytes is armed, it tracks how many bytes this connection has
// forwarded in the current burst. A burst is a run of reads with gaps
// under burstIdleGap since the current arm epoch started. Once that
// count would reach the armed threshold, it forwards only the bytes
// needed to reach the boundary, then trips Break for the whole proxy
// and stops.
func (p *TCPProxy) copyClientToBackend(clientConn, backendConn net.Conn) {
	buf := make([]byte, copyChunkSize)
	var sinceArm int64
	var seenEpoch int64 = -1
	for {
		// Read blocks until data arrives, so the time it spends blocked
		// here is exactly the idle gap since the previous read. Nothing
		// else in this loop waits on the client.
		readStart := time.Now()
		nr, err := clientConn.Read(buf)
		if nr > 0 {
			if time.Since(readStart) >= burstIdleGap {
				sinceArm = 0
			}

			chunk := buf[:nr]
			threshold, epoch := p.armState()
			if epoch != seenEpoch {
				seenEpoch = epoch
				sinceArm = 0
			}
			if threshold > 0 && sinceArm+int64(nr) >= threshold {
				remaining := threshold - sinceArm
				if remaining > 0 {
					_, _ = backendConn.Write(chunk[:remaining])
				}
				if p.tripIfArmed(epoch) {
					return
				}
				// The arm no longer applies (for example, a concurrent
				// Restore). Do not cut. Forward the rest of this chunk
				// normally and keep the loop going.
				if remaining < int64(nr) {
					_, _ = backendConn.Write(chunk[remaining:])
				}
				sinceArm += int64(nr)
				continue
			}
			if _, werr := backendConn.Write(chunk); werr != nil {
				return
			}
			sinceArm += int64(nr)
		}
		if err != nil {
			return
		}
	}
}

func (p *TCPProxy) acceptLoop() {
	for {
		clientConn, err := p.listener.Accept()
		if err != nil {
			return // the listener is closed
		}
		go p.handleConn(clientConn)
	}
}

func (p *TCPProxy) handleConn(clientConn net.Conn) {
	p.mu.Lock()
	if p.broken || p.closed {
		p.mu.Unlock()
		_ = clientConn.Close()
		return
	}
	p.mu.Unlock()

	backendConn, err := net.Dial("tcp", p.backendAddr)
	if err != nil {
		_ = clientConn.Close()
		return
	}

	p.mu.Lock()
	if p.broken || p.closed {
		p.mu.Unlock()
		_ = clientConn.Close()
		_ = backendConn.Close()
		return
	}
	p.conns[clientConn] = struct{}{}
	p.conns[backendConn] = struct{}{}
	p.mu.Unlock()

	done := make(chan struct{}, 2)
	go func() {
		p.copyClientToBackend(clientConn, backendConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(clientConn, backendConn)
		done <- struct{}{}
	}()
	// When one direction ends, close both ends so the other copy stops.
	<-done
	_ = clientConn.Close()
	_ = backendConn.Close()
	<-done

	p.mu.Lock()
	delete(p.conns, clientConn)
	delete(p.conns, backendConn)
	p.mu.Unlock()
}
