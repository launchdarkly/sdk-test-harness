package harness

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startEchoServer runs a TCP server that sends back each line it receives.
func startEchoServer(t *testing.T) net.Listener {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				scanner := bufio.NewScanner(c)
				for scanner.Scan() {
					fmt.Fprintf(c, "%s\n", scanner.Text())
				}
			}(conn)
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

// proxyClient is a test client with one read buffer per connection.
type proxyClient struct {
	conn   net.Conn
	reader *bufio.Reader
}

func dialProxy(t *testing.T, addr string) *proxyClient {
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return &proxyClient{conn: conn, reader: bufio.NewReader(conn)}
}

// roundTrip sends one line and reads one line back.
func (c *proxyClient) roundTrip(message string) (string, error) {
	if err := c.conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(c.conn, "%s\n", message); err != nil {
		return "", err
	}
	reply, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(reply, "\n"), nil
}

func TestTCPProxyForwardsTraffic(t *testing.T) {
	backend := startEchoServer(t)

	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	client := dialProxy(t, proxy.Addr())
	reply, err := client.roundTrip("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", reply)

	reply, err = client.roundTrip("again")
	require.NoError(t, err)
	assert.Equal(t, "again", reply)
}

func TestTCPProxyBreakDropsActiveConnections(t *testing.T) {
	backend := startEchoServer(t)
	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	client := dialProxy(t, proxy.Addr())
	_, err = client.roundTrip("hello")
	require.NoError(t, err)

	proxy.Break()

	_, err = client.roundTrip("hello")
	assert.Error(t, err, "an active connection must fail after Break")
}

func TestTCPProxyBreakRefusesNewConnections(t *testing.T) {
	backend := startEchoServer(t)
	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	proxy.Break()

	// The kernel can accept the dial because of the listen backlog. The
	// proxy still closes the connection before it forwards anything.
	conn, dialErr := net.Dial("tcp", proxy.Addr())
	if dialErr != nil {
		return // refused outright is also a pass
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := &proxyClient{conn: conn, reader: bufio.NewReader(conn)}
	_, err = client.roundTrip("hello")
	assert.Error(t, err, "a new connection must not reach the backend after Break")
}

func TestTCPProxyRestoreResumesForwarding(t *testing.T) {
	backend := startEchoServer(t)
	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	proxy.Break()
	proxy.Restore()

	client := dialProxy(t, proxy.Addr())
	reply, err := client.roundTrip("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", reply)
}

func TestTCPProxyBreakAndRestoreAreIdempotent(t *testing.T) {
	backend := startEchoServer(t)
	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	proxy.Break()
	proxy.Break()
	proxy.Restore()
	proxy.Restore()

	client := dialProxy(t, proxy.Addr())
	reply, err := client.roundTrip("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", reply)
}

func TestTCPProxyBreakAfterBytesAllowsSmallRoundTripsWhileArmed(t *testing.T) {
	backend := startEchoServer(t)
	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	proxy.BreakAfterBytes(1024)

	client := dialProxy(t, proxy.Addr())
	reply, err := client.roundTrip("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", reply)
	assert.False(t, proxy.Broken(), "a small round trip must not trip the armed threshold")
}

// startCountingBackend runs a TCP server that reads until the connection
// closes. It reports the total number of bytes it received.
func startCountingBackend(t *testing.T) (net.Listener, chan int64) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	received := make(chan int64, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var total int64
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			total += int64(n)
			if err != nil {
				break
			}
		}
		received <- total
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return listener, received
}

func TestTCPProxyBreakAfterBytesCutsLargeWrite(t *testing.T) {
	backend, received := startCountingBackend(t)

	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	const threshold = 4096
	const payloadSize = threshold * 4
	proxy.BreakAfterBytes(threshold)

	conn, err := net.Dial("tcp", proxy.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = 'a'
	}
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(2*time.Second)))
	_, writeErr := conn.Write(payload)
	if writeErr == nil {
		// The write itself can succeed into kernel buffers even though
		// the proxy already stopped forwarding. The client only sees an
		// error on a later read or write.
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
		_, writeErr = conn.Read(make([]byte, 1))
	}
	assert.Error(t, writeErr, "the client side must observe a failure after the armed cut fires")

	var total int64
	select {
	case total = <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("backend never saw its connection close")
	}
	assert.Equal(t, int64(threshold), total, "the backend must receive exactly the armed threshold")
	assert.True(t, proxy.Broken(), "the proxy must be broken after the armed cut fires")

	newConn, dialErr := net.Dial("tcp", proxy.Addr())
	if dialErr == nil {
		t.Cleanup(func() { _ = newConn.Close() })
		buf := make([]byte, 1)
		require.NoError(t, newConn.SetReadDeadline(time.Now().Add(200*time.Millisecond)))
		_, readErr := newConn.Read(buf)
		assert.Error(t, readErr, "new connections must not reach the backend once broken")
	}
}

func TestTCPProxyRestoreDisarmsBreakAfterBytes(t *testing.T) {
	backend, received := startCountingBackend(t)
	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	const threshold = 4096
	const payloadSize = threshold * 2
	proxy.BreakAfterBytes(threshold)
	proxy.Restore()

	conn, err := net.Dial("tcp", proxy.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = 'a'
	}
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(2*time.Second)))
	_, err = conn.Write(payload)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	var total int64
	select {
	case total = <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("backend never saw its connection close")
	}
	assert.Equal(t, int64(payloadSize), total,
		"a disarmed proxy must forward a payload larger than the old threshold in full")
	assert.False(t, proxy.Broken(), "a disarmed proxy must not trip on a later payload")
}

func TestTCPProxyBreakAfterBytesArmsExistingConnection(t *testing.T) {
	backend, received := startCountingBackend(t)
	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	conn, err := net.Dial("tcp", proxy.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(2*time.Second)))

	// Forward bytes on this connection before it is ever armed.
	preArm := make([]byte, 5*1024)
	_, err = conn.Write(preArm)
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond) // let it clear the pipe, well past the idle gap

	const threshold = 8 * 1024
	proxy.BreakAfterBytes(threshold)

	// Bytes forwarded before arming must not count toward the threshold.
	underThreshold := make([]byte, 4*1024)
	_, err = conn.Write(underThreshold)
	require.NoError(t, err)
	time.Sleep(20 * time.Millisecond) // short, stays well under the idle gap
	assert.False(t, proxy.Broken(), "bytes forwarded before arming must not count toward the threshold")

	// The rest, forwarded in one burst, must cross the threshold and trip.
	overThreshold := make([]byte, 5*1024)
	_, err = conn.Write(overThreshold)
	require.NoError(t, err)

	require.Eventually(t, proxy.Broken, time.Second, 10*time.Millisecond,
		"forwarding past the threshold in one burst after arming must trip")

	var total int64
	select {
	case total = <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("backend never saw its connection close")
	}
	assert.Equal(t, int64(len(preArm))+int64(threshold), total,
		"the backend must receive the pre-arm bytes in full, plus exactly the threshold")
}

func TestTCPProxyBreakDisarmsBreakAfterBytes(t *testing.T) {
	backend, received := startCountingBackend(t)
	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	const threshold = 4096
	const payloadSize = threshold * 2
	proxy.BreakAfterBytes(threshold)
	proxy.Break()
	proxy.Restore()

	conn, err := net.Dial("tcp", proxy.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	payload := make([]byte, payloadSize)
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(2*time.Second)))
	_, err = conn.Write(payload)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	var total int64
	select {
	case total = <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("backend never saw its connection close")
	}
	assert.Equal(t, int64(payloadSize), total,
		"Break then Restore must disarm a pending BreakAfterBytes threshold")
	assert.False(t, proxy.Broken())
}

func TestTCPProxyBreakAfterBytesIgnoresIdleChatter(t *testing.T) {
	backend, received := startCountingBackend(t)
	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)
	t.Cleanup(proxy.Close)

	const threshold = 4096
	proxy.BreakAfterBytes(threshold)

	conn, err := net.Dial("tcp", proxy.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(5*time.Second)))

	// Eight 1KB messages, well spaced out. Their sum is above the
	// threshold, but each burst is below it. The gaps between them are
	// well past the idle gap, so they must never accumulate.
	chatter := make([]byte, 1024)
	for i := 0; i < 8; i++ {
		_, err = conn.Write(chatter)
		require.NoError(t, err)
		time.Sleep(250 * time.Millisecond)
	}
	assert.False(t, proxy.Broken(), "periodic chatter below the threshold, spaced by idle gaps, must not trip")

	// One contiguous burst above the threshold must still trip.
	burst := make([]byte, 5*1024)
	_, err = conn.Write(burst)
	require.NoError(t, err)

	require.Eventually(t, proxy.Broken, time.Second, 10*time.Millisecond,
		"one contiguous burst above the threshold must trip")

	var total int64
	select {
	case total = <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("backend never saw its connection close")
	}
	assert.Equal(t, int64(8*len(chatter))+int64(threshold), total,
		"the backend must receive all the chatter, plus exactly the threshold from the burst")
}

func TestTCPProxyCloseStopsProxy(t *testing.T) {
	backend := startEchoServer(t)
	proxy, err := NewTCPProxy(backend.Addr().String())
	require.NoError(t, err)

	client := dialProxy(t, proxy.Addr())
	_, err = client.roundTrip("hello")
	require.NoError(t, err)

	proxy.Close()
	proxy.Close() // a second Close must be safe

	_, err = client.roundTrip("hello")
	assert.Error(t, err, "connections must fail after Close")
}
