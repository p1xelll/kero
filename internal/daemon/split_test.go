package daemon

import (
	"testing"

	"kero/internal/proto"
)

func TestSplitSnapshotsPaneInfo(t *testing.T) {
	d := testDaemon(t)
	s, err := d.create(proto.CreateParams{URL: "localhost:3000", Cols: 96, Rows: 28})
	if err != nil {
		t.Fatal(err)
	}
	p := d.findPane(s.Panes[0].ID)
	id := p.info.ID
	dir := t.TempDir()
	stop, stopped, started := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		close(started)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			p.mu.Lock()
			p.info.Dir = dir
			if i%2 == 0 {
				p.info.Cols, p.info.Rows = 96, 28
			} else {
				p.info.Cols, p.info.Rows = 120, 40
			}
			p.mu.Unlock()
		}
	}()
	t.Cleanup(func() { close(stop); <-stopped })
	<-started
	for range 20 {
		got, err := d.split(proto.CreateParams{Split: id, URL: "localhost:3000", Direction: "right"})
		if err != nil {
			t.Fatal(err)
		}
		pn := got.PaneByID(got.Focus)
		if pn == nil {
			t.Fatalf("split(%q) focus = %q, want a new pane", id, got.Focus)
		}
		if !(pn.Cols == 48 && pn.Rows == 28 || pn.Cols == 60 && pn.Rows == 40) {
			t.Errorf("split(%q) dimensions = %dx%d, want 48x28 or 60x40 from one snapshot", id, pn.Cols, pn.Rows)
		}
	}
}
