package client

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"kero/internal/proto"
)

type testDialer struct{ net.Conn }

func (d testDialer) Dial() (net.Conn, error) { return d.Conn, nil }

func controlClient(t *testing.T) (*Client, *net.TCPConn) {
	t.Helper()
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	conn, err := net.DialTCP("tcp", nil, ln.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	server, err := ln.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	if err := server.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	hello := make(chan error, 1)
	go func() {
		var h proto.Hello
		err := json.NewDecoder(server).Decode(&h)
		if err == nil {
			err = json.NewEncoder(server).Encode(proto.Welcome{Version: proto.Version})
		}
		hello <- err
	}()
	c, err := ConnectVia(testDialer{conn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if err := <-hello; err != nil {
		t.Fatal(err)
	}
	return c, server
}

func TestClientCallAfterHalfClose(t *testing.T) {
	c, server := controlClient(t)
	if err := server.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Client.Done() remained open after peer EOF")
	}
	result := make(chan error, 1)
	go func() { result <- c.Call("state", nil, nil) }()
	select {
	case err := <-result:
		if err == nil {
			t.Error("Client.Call(state) after EOF = nil, want an error")
		}
	case <-time.After(time.Second):
		t.Error("Client.Call(state) after EOF did not return")
	}
	var b [1]byte
	if n, err := server.Read(b[:]); n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("peer.Read() after Client.Done() = %d, %v, want 0, EOF", n, err)
	}
}

func TestClientHalfCloseUnblocksWrite(t *testing.T) {
	c, server := controlClient(t)
	if err := c.conn.(*net.TCPConn).SetWriteBuffer(1024); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- c.Call("send", strings.Repeat("x", 8<<20), nil) }()
	var b [1]byte
	if _, err := io.ReadFull(server, b[:]); err != nil {
		t.Fatal(err)
	}
	if err := server.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Error("Client.Call(send) during peer EOF = nil, want an error")
		}
	case <-time.After(2 * time.Second):
		t.Error("Client.Call(send) remained blocked writing after peer EOF")
	}
}

func TestClientPendingCallsOnClose(t *testing.T) {
	for _, halfClose := range []bool{true, false} {
		name := "concurrent_close"
		if halfClose {
			name = "half_close"
		}
		t.Run(name, func(t *testing.T) {
			c, server := controlClient(t)
			const calls = 16
			// One result per call, so senders can exit even on a test failure.
			results := make(chan error, 2*calls)
			for range calls {
				go func() { results <- c.Call("wait", nil, nil) }()
			}
			dec := json.NewDecoder(server)
			for range calls {
				var req proto.Request
				if err := dec.Decode(&req); err != nil {
					t.Fatal(err)
				}
			}
			var closing sync.WaitGroup
			start := make(chan struct{})
			closing.Go(func() {
				<-start
				if halfClose {
					server.CloseWrite()
				} else {
					c.Close()
				}
			})
			for range calls {
				go func() {
					<-start
					results <- c.Call("wait", nil, nil)
				}()
			}
			close(start)
			closing.Wait()
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			for i := range 2 * calls {
				select {
				case err := <-results:
					if err == nil {
						t.Errorf("Client.Call(wait) during close, result %d = nil, want an error", i)
					}
				case <-timer.C:
					t.Fatalf("Client.Call(wait) during close returned %d of %d results", i, 2*calls)
				}
			}
		})
	}
}

func TestClientResponseBeforeEOF(t *testing.T) {
	c, server := controlClient(t)
	type reply struct {
		value int
		err   error
	}
	result := make(chan reply, 1)
	go func() {
		var value int
		err := c.Call("state", nil, &value)
		result <- reply{value, err}
	}()
	var req proto.Request
	if err := json.NewDecoder(server).Decode(&req); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(server).Encode(map[string]any{"id": req.ID, "result": 42}); err != nil {
		t.Fatal(err)
	}
	if err := server.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Client.Done() remained open after the final response and EOF")
	}
	select {
	case got := <-result:
		if got.err != nil || got.value != 42 {
			t.Errorf("Client.Call(state) before EOF = %d, %v, want 42, nil", got.value, got.err)
		}
	case <-time.After(time.Second):
		t.Error("Client.Call(state) did not receive the response before EOF")
	}
}
