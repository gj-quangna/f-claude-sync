package paths

import (
	"testing"

	"github.com/tawanorg/claude-sync/internal/config"
)

func TestValidatePath(t *testing.T) {
	claudeDir := "/home/u/.claude"
	valid := []string{
		"/home/u/.claude/CLAUDE.md",
		"/home/u/.claude/agents",
		"/home/u/.claude/projects/foo",
	}
	for _, p := range valid {
		if err := ValidatePath(p, claudeDir); err != nil {
			t.Errorf("ValidatePath(%q) = %v, want nil", p, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"CLAUDE.md",
		"./CLAUDE.md",
		"..",
		"../.ssh",
		"/home/u/.claude",
		"/home/u/.claude/../.ssh/id_rsa",
		"/etc/passwd",
		"/",
	}
	for _, p := range invalid {
		if err := ValidatePath(p, claudeDir); err == nil {
			t.Errorf("ValidatePath(%q) = nil, want error", p)
		}
	}
}

// TestAddRejectsTraversal ensures the CLI cannot persist a sync path that would
// read outside ~/.claude on push or write outside it on pull.
func TestAddRejectsTraversal(t *testing.T) {
	m := NewManager(config.DefaultSyncPaths("/tmp/test"), nil, "/tmp/test")
	before := len(m.SyncPaths())

	result := m.Add("/tmp/test/../.ssh")
	if result.Invalid == nil {
		t.Error("Add(\"/tmp/test/../.ssh\") should be rejected")
	}
	if result.Added {
		t.Error("traversing path must not be added")
	}
	if len(m.SyncPaths()) != before {
		t.Errorf("sync paths mutated on rejected add: %v", m.SyncPaths())
	}
}
