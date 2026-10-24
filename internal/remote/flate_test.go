package remote

import (
	"net"
	"sync"
	"testing"
	"time"
)

type readSignalConn struct {
	net.Conn
	started chan struct{}
	once    sync.Once
}

func (c *readSignalConn) Read(p []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	return c.Conn.Read(p)
}

func readingFlateConn(t *testing.T) (*flateConn, net.Conn, <-chan error) {
	t.Helper()
	here, peer := net.Pipe()
	t.Cleanup(func() { here.Close(); peer.Close() })
	transport := &readSignalConn{Conn: here, started: make(chan struct{})}
	c := newFlateConn(transport)
	done := make(chan error, 1)
	go func() {
		var b [1]byte
		_, err := c.Read(b[:])
		done <- err
	}()
	select {
	case <-transport.started:
	case <-time.After(2 * time.Second):
		t.Fatal("flateConn.Read() did not reach the transport within 2s")
	}
	return c, here, done
}

func TestFlateConnCloseUnblocksRead(t *testing.T) {
	c, _, done := readingFlateConn(t)
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("flateConn.Close() = %v, want nil", err)
		}
	case <-timer.C:
		t.Fatal("flateConn.Close() blocked behind an active Read")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("flateConn.Read() after Close = nil, want an error")
		}
	case <-timer.C:
		t.Fatal("flateConn.Read() did not return after Close")
	}
	if err := c.Close(); err != nil {
		t.Errorf("flateConn.Close() repeated on net.Pipe = %v, want nil", err)
	}
}

func TestFlateConnConcurrentCloseAfterTransportClose(t *testing.T) {
	c, transport, done := readingFlateConn(t)
	// Conn.Close closes the raw transport before yamux closes its flate wrapper.
	if err := transport.Close(); err != nil {
		t.Fatal(err)
	}
	const callers = 8
	start := make(chan struct{})
	// One result per caller lets every goroutine finish even if an assertion fails.
	closed := make(chan error, callers)
	for range callers {
		go func() {
			<-start
			closed <- c.Close()
		}()
	}
	close(start)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for i := range callers {
		select {
		case err := <-closed:
			if err != nil {
				t.Errorf("flateConn.Close() caller %d on closed net.Pipe = %v, want nil", i, err)
			}
		case <-timer.C:
			t.Fatalf("flateConn.Close() returned for %d of %d callers, want all", i, callers)
		}
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("flateConn.Read() after transport Close = nil, want an error")
		}
	case <-timer.C:
		t.Fatal("flateConn.Read() did not return after transport Close")
	}
}
