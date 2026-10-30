package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tawanorg/claude-sync/internal/config"
)

func TestNewManager(t *testing.T) {
	// Empty config lists stay empty: there is no built-in fallback
	m := NewManager(nil, nil, "/tmp/test")
	if len(m.SyncPaths()) != 0 {
		t.Errorf("Expected no paths, got %v", m.SyncPaths())
	}

	custom := []string{"path1", "path2"}
	m = NewManager(custom, nil, "/tmp/test")
	if !reflect.DeepEqual(m.SyncPaths(), custom) {
		t.Errorf("Expected %v, got %v", custom, m.SyncPaths())
	}

	// Mutating the manager must not alias the caller's slice
	m.Remove("path1")
	if custom[0] != "path1" {
		t.Errorf("caller slice was mutated: %v", custom)
	}
}

func TestAddPath(t *testing.T) {
	m := NewManager([]string{"/tmp/nonexistent/existing"}, nil, "/tmp/nonexistent")

	// Add new path
	result := m.Add("/tmp/nonexistent/newpath")
	if !result.Added {
		t.Error("Expected Added=true")
	}
	if result.PathMissing != true {
		t.Error("Expected PathMissing=true for non-existent path")
	}
	if !m.HasPath("/tmp/nonexistent/newpath") {
		t.Error("newpath should be in sync paths")
	}

	// Add existing path
	result = m.Add("/tmp/nonexistent/existing")
	if !result.AlreadyExists {
		t.Error("Expected AlreadyExists=true")
	}
	if result.Added {
		t.Error("Should not add duplicate")
	}
}

func TestAddPathRemovesConflictingExcludes(t *testing.T) {
	m := NewManager([]string{"/tmp/test/a"}, []string{"/tmp/test/b", "/tmp/test/b/*", "c/**"}, "/tmp/test")

	// Add 'b' should remove 'b' and 'b/*' excludes
	result := m.Add("/tmp/test/b")
	if result.ExcludesRemoved != 2 {
		t.Errorf("Expected 2 excludes removed, got %d", result.ExcludesRemoved)
	}
	if m.HasExclude("/tmp/test/b") || m.HasExclude("/tmp/test/b/*") {
		t.Error("Conflicting excludes should be removed")
	}
	if !m.HasExclude("c/**") {
		t.Error("Unrelated exclude should remain")
	}
}

func TestRemovePath(t *testing.T) {
	m := NewManager([]string{"/tmp/test/CLAUDE.md", "/tmp/test/custom"}, nil, "/tmp/test")

	result := m.Remove("nonexistent")
	if !result.NotFound {
		t.Error("Expected NotFound=true")
	}

	result = m.Remove("/tmp/test/CLAUDE.md")
	if !result.Removed {
		t.Error("Expected Removed=true")
	}
	if m.HasPath("/tmp/test/CLAUDE.md") {
		t.Error("CLAUDE.md should be removed")
	}
	// Removing only edits sync_paths; excludes are untouched
	if len(m.Excludes()) != 0 {
		t.Errorf("Remove should not add excludes, got %v", m.Excludes())
	}
}

func TestAddExclude(t *testing.T) {
	m := NewManager([]string{"plugins"}, []string{"existing/*"}, "/tmp/test")

	// Add new exclude
	result := m.AddExclude("plugins/**/node_modules/**")
	if !result.Added {
		t.Error("Expected Added=true")
	}

	// Add duplicate
	result = m.AddExclude("existing/*")
	if !result.AlreadyExists {
		t.Error("Expected AlreadyExists=true")
	}

	// Try to exclude a sync path directly
	result = m.AddExclude("plugins")
	if !result.IsSyncPath {
		t.Error("Expected IsSyncPath=true - should use Remove instead")
	}
	if result.Added {
		t.Error("Should not add sync path as exclude")
	}
}

func TestRemoveExclude(t *testing.T) {
	m := NewManager(nil, []string{"a/*", "b/**"}, "/tmp/test")

	// Remove existing
	result := m.RemoveExclude("a/*")
	if !result.Removed {
		t.Error("Expected Removed=true")
	}
	if m.HasExclude("a/*") {
		t.Error("Exclude should be removed")
	}

	// Remove non-existent
	result = m.RemoveExclude("nonexistent")
	if !result.NotFound {
		t.Error("Expected NotFound=true")
	}
}

func TestReset(t *testing.T) {
	m := NewManager(
		[]string{"custom1", "custom2"},
		[]string{"exclude1", "exclude2"},
		"/tmp/test",
	)

	m.Reset()

	if !reflect.DeepEqual(m.SyncPaths(), config.DefaultSyncPaths("/tmp/test")) {
		t.Errorf("Expected DefaultSyncPaths after reset, got %v", m.SyncPaths())
	}
	if len(m.Excludes()) != 0 {
		t.Errorf("Expected 0 excludes after reset, got %d", len(m.Excludes()))
	}

	// Editing after reset must not mutate the shared DefaultSyncPaths template
	template := append([]string{}, config.DefaultSyncNames...)
	m.Remove(m.SyncPaths()[0])
	if !reflect.DeepEqual(config.DefaultSyncNames, template) {
		t.Errorf("DefaultSyncNames template was mutated: %v", config.DefaultSyncNames)
	}
}

func TestHasPath(t *testing.T) {
	m := NewManager([]string{"a", "b"}, nil, "/tmp/test")

	if !m.HasPath("a") {
		t.Error("Should have path 'a'")
	}
	if m.HasPath("c") {
		t.Error("Should not have path 'c'")
	}
}

func TestHasExclude(t *testing.T) {
	m := NewManager(nil, []string{"*.tmp", "cache/**"}, "/tmp/test")

	if !m.HasExclude("*.tmp") {
		t.Error("Should have exclude '*.tmp'")
	}
	if m.HasExclude("*.log") {
		t.Error("Should not have exclude '*.log'")
	}
}

func TestIntegrationWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	history := filepath.Join(claudeDir, "history.jsonl")

	// Start from what init writes
	m := NewManager(config.DefaultSyncPaths(claudeDir), nil, claudeDir)

	// 1. Remove a path
	if result := m.Remove(history); !result.Removed {
		t.Error("Should remove history.jsonl")
	}

	// 2. Reload from the saved lists (like after config reload)
	m = NewManager(m.SyncPaths(), m.Excludes(), claudeDir)
	if m.HasPath(history) {
		t.Error("history.jsonl should not be in sync paths")
	}

	// 3. Add it back
	if result := m.Add(history); !result.Added {
		t.Error("Should add history.jsonl back")
	}

	// 4. Add sub-path filter
	m.AddExclude("plugins/**/node_modules/**")
	if !m.HasExclude("plugins/**/node_modules/**") {
		t.Error("Should have the exclude")
	}

	// 5. Reset restores the template and clears excludes
	m.Reset()
	if !m.HasPath(history) {
		t.Error("Reset should restore defaults including history.jsonl")
	}
	if len(m.Excludes()) != 0 {
		t.Error("Reset should clear excludes")
	}
}
