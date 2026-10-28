package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/f24aalam/agentsync/internal/config"
)

func TestUpdateTomlMCPServersPreservesRootAssignments(t *testing.T) {
	cases := []struct{ name, content string }{
		{"inline", "[mcp_servers]\n# keep this server\nkeep = { command = \"keepcmd\" }\n"},
		{"replace inline", "[mcp_servers]\nkeep = { command = \"keepcmd\" }\nnew = { command = \"oldcmd\" }\n"},
		{"mixed tables", "[mcp_servers]\nkeep = { command = \"keepcmd\" }\nnew = { command = \"oldcmd\" }\n\n[mcp_servers.other]\ncommand = \"othercmd\"\n\n[settings]\nenabled = true\n"},
		{"quoted dotted keys", "['mcp_servers']\n'keep.with.dot'.command = 'keepcmd'\n\"n\\u0065w\".command = 'oldcmd'\nnew.args = ['old']\n"},
		{"multiline", "[mcp_servers]\n# keep multiline values\nkeep.command = '''\n[mcp_servers.fake]\nkeepcmd\n'''\nkeep.args = [\n 'one', # comment\n 'two',\n]\nnew.command = \"\"\"\noldcmd\n\"\"\"\n"},
		{"parent after child", "[mcp_servers.new]\ncommand = 'oldcmd'\n\n[mcp_servers]\nkeep = { command = 'keepcmd' }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var before map[string]any
			if _, err := toml.Decode(tc.content, &before); err != nil {
				t.Fatal(err)
			}
			servers := map[string]config.MCPServer{"new": {Command: "newcmd"}}
			updated, err := updateTomlMCPServers(tc.content, servers)
			if err != nil {
				t.Fatal(err)
			}
			var after map[string]any
			if _, err := toml.Decode(updated, &after); err != nil {
				t.Fatal(err)
			}
			beforeServers := before["mcp_servers"].(map[string]any)
			afterServers := after["mcp_servers"].(map[string]any)
			for name, value := range beforeServers {
				if name != "new" && !reflect.DeepEqual(value, afterServers[name]) {
					t.Fatalf("unmanaged server %q changed: %s", name, updated)
				}
			}
			for name, value := range before {
				if name != "mcp_servers" && !reflect.DeepEqual(value, after[name]) {
					t.Fatalf("setting %q changed", name)
				}
			}
			if !reflect.DeepEqual(afterServers["new"], map[string]any{"command": "newcmd"}) {
				t.Fatalf("incorrect replacement: %s", updated)
			}
			// Every non-replaced line, including comments and whitespace in values, survives.
			for _, line := range strings.Split(tc.content, "\n") {
				if strings.Contains(line, "keep") && !strings.Contains(updated, line) {
					t.Fatalf("unmanaged line lost: %q", line)
				}
			}
			repeated, err := updateTomlMCPServers(updated, servers)
			if err != nil {
				t.Fatal(err)
			}
			if updated != repeated {
				t.Fatalf("update is not idempotent:\n%s\nthen:\n%s", updated, repeated)
			}
		})
	}
}

func TestMCPRootUnmanagedServerSurvivesInstall(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, ".codex/config.toml")
	mustWrite(t, dest, "[mcp_servers]\nkeep = { command = \"keepcmd\" }\n")
	target, _ := ByID("codex")
	plan := NewInstallPlan() // No ownership and no conflicts: automatic approval installs new.
	cfg := config.MCPConfig{Servers: map[string]config.MCPServer{"new": {Command: "newcmd"}}}
	if err := mergeOrWriteTomlMCP(dest, target, cfg, plan, ".codex/config.toml"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `keep = { command = "keepcmd" }`) {
		t.Fatalf("unmanaged server lost: %s", data)
	}
	if plan.Lock.IsMCPManaged(".codex/config.toml", "keep") {
		t.Fatal("unmanaged server adopted")
	}
	if !plan.Lock.IsMCPManaged(".codex/config.toml", "new") {
		t.Fatal("new server not recorded")
	}
}

func TestSkillPermissionOnlyUpdates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits required")
	}
	root := t.TempDir()
	src := filepath.Join(root, ".ai/skills/tool")
	dst := filepath.Join(root, ".claude/skills/tool")
	mustWrite(t, filepath.Join(src, "SKILL.md"), "tool")
	script := filepath.Join(src, "run.sh")
	mustWrite(t, script, "#!/bin/sh\necho tool\n")
	if err := os.Chmod(script, 0o644); err != nil {
		t.Fatal(err)
	}
	plan := NewInstallPlan()
	if err := syncSkillTreesWithPlan(root, ".claude/skills", plan); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(dst, "run.sh")
	for _, mode := range []os.FileMode{0o755, 0o644} {
		if err := os.Chmod(script, mode); err != nil {
			t.Fatal(err)
		}
		exists, differs, err := compareSkillPackages(src, dst)
		if err != nil || !exists || !differs {
			t.Fatalf("permission difference missed: %v %v %v", exists, differs, err)
		}
		if err := syncSkillTreesWithPlan(root, ".claude/skills", plan); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(installed)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("got mode %o, want %o", info.Mode().Perm(), mode)
		}
		exists, differs, err = compareSkillPackages(src, dst)
		if err != nil || !exists || differs {
			t.Fatalf("installed package differs: %v %v %v", exists, differs, err)
		}
	}
	old := time.Unix(1000000000, 0)
	if err := os.Chtimes(installed, old, old); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(installed)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncSkillTreesWithPlan(root, ".claude/skills", plan); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(installed)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("identical package replaced")
	}
}
