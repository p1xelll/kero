package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"kero/internal/git"
	"kero/internal/proto"
)

func TestChangesCacheHostOwnership(t *testing.T) {
	for _, base := range []string{git.BaseHead, git.BaseBranch, git.BaseTurn} {
		t.Run(base, func(t *testing.T) {
			a := testApp(t)
			remote := withHost(t, a, "build")
			other := withHost(t, a, "other")
			a.owner["second-build"] = remote
			a.side.changes.base = base
			panes := []proto.Pane{
				{ID: "a"}, {ID: "b"},
				{ID: "rpa-build"}, {ID: "second-build"},
				{ID: "rpa-other"},
			}
			for i := range panes {
				panes[i].Git = &proto.GitInfo{Root: "/same/repo", Head: "head"}
				panes[i].Turn = &proto.Turn{Root: "/same/repo", Tree: "tree"}
			}
			files := []proto.GitFile{{Path: "a.go", Added: 1}}
			pane := &panes[0]
			tt := ui.NewTester(func(c *ui.Context) {
				a.syncFiles(c, a.changesScope(pane, base), pane, files)
			}, 300, 100)
			seen := map[*host]*sideFile{}
			scopes := map[*host]string{}
			for i, owner := range []*host{a.local(), a.local(), remote, remote, other} {
				pane = &panes[i]
				scope := a.changesScope(pane, base)
				tt.Frame()
				sf := a.side.changes.files[scope+"|a.go"]
				if sf == nil {
					t.Fatalf("syncFiles(%q, %q) has no cached file, want a.go", pane.ID, base)
				}
				if got := a.hostOf(sf.pane); got != owner {
					t.Errorf("syncFiles(%q, %q) owner = %q, want %q", pane.ID, base, got.name, owner.name)
				}
				if first := seen[owner]; first != nil {
					if sf != first || scope != scopes[owner] {
						t.Errorf("syncFiles(%q, %q) cache = (%p, %q), want (%p, %q)", pane.ID, base, sf, scope, first, scopes[owner])
					}
					continue
				}
				for h, first := range seen {
					if sf == first || scope == scopes[h] {
						t.Errorf("syncFiles(%q, %q) shares cache with host %q, want separate scope and file", pane.ID, base, h.name)
					}
				}
				seen[owner], scopes[owner] = sf, scope
			}
		})
	}
}

func TestChangesScopeIdentity(t *testing.T) {
	a := testApp(t)
	pane := proto.Pane{
		ID:   "a",
		Git:  &proto.GitInfo{Root: "/repo"},
		Turn: &proto.Turn{Root: "/repo", Tree: "tree"},
	}
	scope := a.changesScope(&pane, git.BaseHead)
	if got := a.changesScope(&pane, git.BaseBranch); got == scope {
		t.Errorf("changesScope(%q, branch) = %q, want a different scope from head", pane.ID, got)
	}
	pane.Git = &proto.GitInfo{Root: "/other"}
	if got := a.changesScope(&pane, git.BaseHead); got == scope {
		t.Errorf("changesScope(%q, head) = %q after changing root, want a different scope", pane.ID, got)
	}
	scope = a.changesScope(&pane, git.BaseTurn)
	pane.Turn = &proto.Turn{Root: "/repo", Tree: "next-tree"}
	if got := a.changesScope(&pane, git.BaseTurn); got == scope {
		t.Errorf("changesScope(%q, turn) = %q after changing snapshot, want a different scope", pane.ID, got)
	}
	pane.Git, pane.Turn = nil, nil
	scope = a.changesScope(&pane, git.BaseHead)
	pane.ID = "b"
	if got := a.changesScope(&pane, git.BaseHead); got == scope {
		t.Errorf("changesScope(%q, head) = %q without a worktree, want a pane-specific scope", pane.ID, got)
	}
}

func TestChangesCacheLargeListRebuild(t *testing.T) {
	a := testApp(t)
	ch := &a.side.changes
	ch.base = git.BaseHead
	pane := &proto.Pane{ID: "a", Git: &proto.GitInfo{Root: "/repo", Head: "head"}}
	scope := a.changesScope(pane, ch.base)
	files := make([]proto.GitFile, 401)
	for i := range files {
		files[i] = proto.GitFile{Path: fmt.Sprintf("f%03d.go", i), Added: 1}
	}
	tt := ui.NewTester(func(c *ui.Context) {
		a.syncFiles(c, scope, pane, files)
	}, 300, 100)
	tt.Frame()
	first := ch.files[scope+"|"+files[0].Path]
	if first == nil {
		t.Fatal("syncFiles(401 files) has no first file, want a cached diff")
	}
	a.showText(first, proto.GitText{Old: "old\n", New: "new\n", Hash: "hash"})
	first.stamp = gitStamp(pane.Git)
	for _, f := range files[1:5] {
		ch.files[scope+"|"+f.Path].busy = true
	}
	ch.files[scope+"|"+files[400].Path].writing = true
	pointers := make([]*sideFile, len(files))
	states := make([]sideFile, len(files))
	for i, f := range files {
		sf := ch.files[scope+"|"+f.Path]
		if sf == nil {
			t.Fatalf("syncFiles(401 files)[%q] = nil, want a cached file", f.Path)
		}
		pointers[i], states[i] = sf, *sf
	}
	for frame := range 3 {
		tt.Frame()
		for i, f := range files {
			sf := ch.files[scope+"|"+f.Path]
			if sf != pointers[i] {
				t.Fatalf("syncFiles(401 files), rebuild %d, file %q = %p, want %p", frame, f.Path, sf, pointers[i])
			}
			if *sf != states[i] {
				t.Errorf("syncFiles(401 files), rebuild %d, file %q state = %+v, want %+v", frame, f.Path, *sf, states[i])
			}
		}
	}
}

func TestChangesCachePrunesInactiveEntries(t *testing.T) {
	a := testApp(t)
	ch := &a.side.changes
	ch.base = git.BaseTurn
	pane := &proto.Pane{ID: "a", Turn: &proto.Turn{Root: "/repo", Tree: "first"}}
	scope := a.changesScope(pane, ch.base)
	oldScope := scope
	all := make([]proto.GitFile, 401)
	for i := range all {
		all[i] = proto.GitFile{Path: fmt.Sprintf("f%03d.go", i), Added: 1}
	}
	files := all
	tt := ui.NewTester(func(c *ui.Context) {
		a.syncFiles(c, scope, pane, files)
	}, 300, 100)
	tt.Frame()
	busyKey, writingKey := oldScope+"|"+all[0].Path, oldScope+"|"+all[1].Path
	busy, writing := ch.files[busyKey], ch.files[writingKey]
	if busy == nil || writing == nil {
		t.Fatal("syncFiles(401 files) has no pending files, want cached entries")
	}
	busy.busy, writing.writing = true, true

	pane.Turn = &proto.Turn{Root: "/repo", Tree: "second"}
	scope = a.changesScope(pane, ch.base)
	tt.Frame()
	if ch.files[busyKey] != busy || ch.files[writingKey] != writing || !busy.busy || !writing.writing {
		t.Error("syncFiles(second turn) lost another scope's busy/writing entries, want original pointers and flags")
	}
	if got := len(ch.files); got != 403 {
		t.Errorf("syncFiles(second turn) cache size = %d, want 401 current + 2 pending", got)
	}
	if got := ch.files[oldScope+"|"+all[2].Path]; got != nil {
		t.Errorf("syncFiles(second turn) inactive old file = %p, want nil", got)
	}

	currentKey := scope + "|" + all[0].Path
	current := ch.files[currentKey]
	files = all[:1]
	tt.Frame()
	if got := ch.files[currentKey]; got != current || got == nil {
		t.Errorf("syncFiles(shorter list) current file = %p, want %p", got, current)
	}
	if got := len(ch.files); got != 3 {
		t.Errorf("syncFiles(shorter list) cache size = %d, want 1 current + 2 pending", got)
	}
	if got := ch.files[scope+"|"+all[1].Path]; got != nil {
		t.Errorf("syncFiles(shorter list) removed file = %p, want nil", got)
	}

	pane.Turn = &proto.Turn{Root: "/repo", Tree: "first"}
	scope, files = oldScope, all[:2]
	tt.Frame()
	if ch.files[busyKey] != busy || ch.files[writingKey] != writing {
		t.Error("syncFiles(first turn again) replaced busy/writing files, want the original entries")
	}
	busy.busy, writing.writing = false, false

	files = all
	for i := range 12 {
		pane.Turn = &proto.Turn{Root: "/repo", Tree: fmt.Sprintf("history-%d", i)}
		scope = a.changesScope(pane, ch.base)
		tt.Frame()
		tt.Frame()
		if got := len(ch.files); got != 401 {
			t.Fatalf("syncFiles(history-%d) cache size = %d, want 401 current files without idle history", i, got)
		}
	}
	if ch.files[busyKey] != nil || ch.files[writingKey] != nil {
		t.Error("syncFiles(after completion) retained idle historical entries, want both pruned")
	}
}
