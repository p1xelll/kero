package remote

import (
	"errors"
	"io"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func localBridge(t *testing.T, script string) *cmdConn {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip(err)
	}
	c, err := startBridge(exec.Command(sh, "-c", script), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c.cmd.Process.Kill()
		c.ended()
		select {
		case <-c.done:
		case <-time.After(4 * time.Second):
			t.Error("local bridge process was not reaped during cleanup")
		}
	})
	return c
}

func TestCmdConnCloseAfterStdoutEOF(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip(err)
	}
	c := localBridge(t, "exec 1>&-; exec "+shellQuote(sleep)+" 30")
	var b [1]byte
	if n, err := c.Read(b[:]); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("cmdConn.Read() = %d, %v, want 0, EOF", n, err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.done:
		if c.err == nil {
			t.Error("cmdConn.Close() after stdout EOF let sleep exit normally, want it killed")
		}
	case <-time.After(time.Second):
		t.Error("cmdConn.Close() after stdout EOF left the process running")
	}
}

func TestCmdConnCloseWithoutRead(t *testing.T) {
	c := localBridge(t, "while read line; do :; done")
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.done:
		if c.err != nil {
			t.Errorf("cmdConn.Close() without reading stdout = %v, want a clean exit on stdin EOF", c.err)
		}
	case <-time.After(time.Second):
		t.Error("cmdConn.Close() without reading stdout did not reap the process")
	}
}
