package main

import (
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
