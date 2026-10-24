//go:build !windows

package tailscale

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWhoIsClosesIdleConnections(t *testing.T) {
	const requests = 20
	const addr = "100.64.0.1:12345"

	// A relative socket path avoids Unix socket path limits in long temp directories.
	t.Chdir(t.TempDir())
	const sock = "tailscaled.sock"
	t.Setenv("KERO_TAILSCALED_SOCKET", sock)

	// Buffer one notification per request so ConnState never blocks during cleanup.
	closed := make(chan struct{}, requests)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/localapi/v0/whois" || r.URL.Query().Get("addr") != addr {
			t.Errorf("WhoIs(%q) requested %s, want /localapi/v0/whois?addr=%s", addr, r.URL, addr)
			http.NotFound(w, r)
			return
		}
		if _, err := io.WriteString(w, `{"Node":{"Name":"laptop."},"UserProfile":{"LoginName":"alice@example.com"}}`); err != nil {
			t.Errorf("WhoIs(%q) response write error = %v, want nil", addr, err)
		}
	}))
	t.Cleanup(srv.Close)
	if err := srv.Listener.Close(); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("net.Listen(unix, %q) error = %v, want nil", sock, err)
	}
	srv.Listener = ln
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	srv.Start()

	for i := 0; i < requests; i++ {
		who, err := WhoIs(t.Context(), addr)
		if err != nil {
			t.Fatalf("WhoIs(%q) request %d error = %v, want nil", addr, i+1, err)
		}
		if who.Node != "laptop" || who.Login != "alice@example.com" {
			t.Errorf("WhoIs(%q) = %+v, want node laptop and login alice@example.com", addr, who)
		}
	}

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for n := 0; n < requests; n++ {
		select {
		case <-closed:
		case <-timer.C:
			t.Fatalf("WhoIs(%q) closed %d of %d connections, want all closed before server shutdown", addr, n, requests)
		}
	}
}
