package cli

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skillAgentFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	for key, value := range map[string]string{
		"HOME": home, "USERPROFILE": home,
		"KERO_HOME":           filepath.Join(root, "kero"),
		"KERO_CONFIG":         filepath.Join(root, "config"),
		"CLAUDE_CONFIG_DIR":   filepath.Join(root, "custom claude"),
		"CODEX_HOME":          filepath.Join(home, ".codex"),
		"PI_CODING_AGENT_DIR": filepath.Join(home, ".pi", "agent"),
		"XDG_CONFIG_HOME":     filepath.Join(home, ".config"),
		"VIBE_HOME":           filepath.Join(home, ".vibe"),
	} {
		t.Setenv(key, value)
	}
	for rel, data := range map[string]string{
		"custom claude/skills/kero/SKILL.md":       "Existing Claude skill\n",
		"custom claude/settings.json":              "{\"model\":\"keep\"}\n",
		"home/.agents/skills/kero/SKILL.md":        "Existing shared skill\n",
		"home/.agents/skills/other/keep.txt":       "Another skill\n",
		"home/.codex/hooks.json":                   "{}\n",
		"home/.pi/agent/extensions/other.ts":       "// Another extension\n",
		"home/.config/opencode/plugins/other/x.js": "// Another plugin\n",
		"home/.vibe/hooks.toml":                    "# Existing Vibe hooks\nhooks = []\n",
		"unrelated/keep.txt":                       "Unrelated contents\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func skillAgentTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		var data []byte
		if entry.Type()&os.ModeSymlink != 0 {
			var target string
			target, err = os.Readlink(path)
			data = []byte(target)
		} else if !entry.IsDir() {
			data, err = os.ReadFile(path)
		}
		if err != nil {
			return err
		}
		tree[filepath.ToSlash(rel)] = fmt.Sprintf("%s:%x", info.Mode(), sha256.Sum256(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestSkillRemoveRejectsUnknownAgent(t *testing.T) {
	root := skillAgentFixture(t)
	before := skillAgentTree(t, root)
	args := []string{"remove", "--agent", "vibee"}
	if err := skillCmd(args); !errors.Is(err, errUsage) {
		t.Errorf("skillCmd(%q) error = %v, want a usage error", args, err)
	}
	if after := skillAgentTree(t, root); !maps.Equal(after, before) {
		t.Errorf("skillCmd(%q) changed the filesystem: got %v, want %v", args, after, before)
	}
}

func TestSkillRejectsUnexpectedArguments(t *testing.T) {
	for _, args := range [][]string{
		{"remove", "vibee"},
		{"remove", "--", "--agent", "vibee"},
		{"remove", "--agent", "claude", "extra"},
		{"setup", "claude"},
		{"status", "claude"},
		{"show", "claude"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := skillAgentFixture(t)
			before := skillAgentTree(t, root)
			if err := skillCmd(args); !errors.Is(err, errUsage) {
				t.Errorf("skillCmd(%q) error = %v, want a usage error", args, err)
			}
			if after := skillAgentTree(t, root); !maps.Equal(after, before) {
				t.Errorf("skillCmd(%q) changed the filesystem: got %v, want %v", args, after, before)
			}
		})
	}
}

func TestSkillRejectsInvalidAgent(t *testing.T) {
	for _, action := range []string{"show", "status", "setup", "remove"} {
		t.Run(action, func(t *testing.T) {
			for _, agent := range []struct{ name, value string }{
				{"typo", "vibee"},
				{"uppercase-typo", "VIBEE"},
				{"unknown", "all"},
				{"empty", ""},
				{"whitespace", " "},
				{"path", "../claude"},
			} {
				t.Run(agent.name, func(t *testing.T) {
					for _, equals := range []bool{false, true} {
						for _, dry := range []bool{false, true} {
							t.Run(fmt.Sprintf("equals=%v/dry=%v", equals, dry), func(t *testing.T) {
								root := skillAgentFixture(t)
								before := skillAgentTree(t, root)
								args := []string{action, "--agent", agent.value}
								if equals {
									args = []string{action, "--agent=" + agent.value}
								}
								if dry {
									args = append(args, "--dry-run")
								}
								if err := skillCmd(args); !errors.Is(err, errUsage) {
									t.Errorf("skillCmd(%q) error = %v, want a usage error", args, err)
								}
								if after := skillAgentTree(t, root); !maps.Equal(after, before) {
									t.Errorf("skillCmd(%q) changed the filesystem: got %v, want %v", args, after, before)
								}
							})
						}
					}
				})
			}
		})
	}
}

func TestSkillRemoveAgentScope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		removed []string
	}{
		{"default", []string{"remove"}, []string{"claude", "agents"}},
		{"empty-terminator", []string{"remove", "--"}, []string{"claude", "agents"}},
		{"claude", []string{"remove", "--agent", "claude"}, []string{"claude"}},
		{"shared", []string{"remove", "--agent", "agents"}, []string{"agents"}},
		{"codex", []string{"remove", "--agent", "codex"}, []string{"agents"}},
		{"pi", []string{"remove", "--agent", "pi"}, []string{"agents"}},
		{"opencode", []string{"remove", "--agent", "opencode"}, []string{"agents"}},
		{"uppercase", []string{"remove", "--agent=CLAUDE"}, []string{"claude"}},
		{"mixed-case", []string{"remove", "--agent", "CoDeX"}, []string{"agents"}},
		{"last-value", []string{"remove", "--agent", "vibee", "--agent", "claude"}, []string{"claude"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := skillAgentFixture(t)
			want := skillAgentTree(t, root)
			for _, agent := range tc.removed {
				dir := map[string]string{
					"claude": "custom claude/skills/kero",
					"agents": "home/.agents/skills/kero",
				}[agent]
				for path := range want {
					if path == dir || strings.HasPrefix(path, dir+"/") {
						delete(want, path)
					}
				}
			}
			if err := skillCmd(tc.args); err != nil {
				t.Fatalf("skillCmd(%q) error = %v, want nil", tc.args, err)
			}
			if got := skillAgentTree(t, root); !maps.Equal(got, want) {
				t.Errorf("skillCmd(%q) filesystem = %v, want %v", tc.args, got, want)
			}
		})
	}
}
