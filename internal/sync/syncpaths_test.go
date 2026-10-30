package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tawanorg/claude-sync/internal/config"
)

// TestSyncerHonorsCustomSyncPaths is the regression test for issue #64: the
// sync_paths override was wired into Config but never read by the syncer, so a
// user-supplied path list was silently ignored by push, status, and diff.
func TestSyncerHonorsCustomSyncPaths(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"CLAUDE.md", "my-extra-file.md", "unlisted.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{SyncPaths: []string{filepath.Join(dir, "CLAUDE.md"), filepath.Join(dir, "my-extra-file.md")}}
	s := &Syncer{cfg: cfg, claudeDir: dir}

	got, err := GetLocalFiles(dir, s.syncPaths(), s.isExcluded)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := got["my-extra-file.md"]; !ok {
		t.Errorf("custom sync_paths entry not synced; got %v", sortedKeys(got))
	}
	if _, ok := got["unlisted.md"]; ok {
		t.Errorf("file outside sync_paths was synced; got %v", sortedKeys(got))
	}
}

// TestSyncerExcludeFiltersInsideSyncPath verifies that exclude globs from
// config.yaml drop files inside a synced directory (e.g. plugins/node_modules).
func TestSyncerExcludeFiltersInsideSyncPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "node_modules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins", "node_modules", "big.js"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins", "plugin.json"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, pattern := range []string{"**/node_modules/**", filepath.Join(dir, "plugins", "node_modules", "**")} {
		cfg := &config.Config{
			SyncPaths: []string{filepath.Join(dir, "plugins")},
			Exclude:   []string{pattern},
		}
		s := &Syncer{cfg: cfg, claudeDir: dir}

		got, err := GetLocalFiles(dir, s.syncPaths(), s.isExcluded)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := got["plugins/node_modules/big.js"]; ok {
			t.Fatalf("exclude %q: excluded file was collected: %v", pattern, sortedKeys(got))
		}
		if _, ok := got["plugins/plugin.json"]; !ok {
			t.Errorf("exclude %q: non-excluded file missing: %v", pattern, sortedKeys(got))
		}
	}
}

// TestNewSyncerRejectsRelativeSyncPaths ensures sync_paths entries must be
// absolute paths inside ~/.claude.
func TestNewSyncerRejectsRelativeSyncPaths(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"projects", "/etc", dir} {
		_, err := NewSyncer(&config.Config{SyncPaths: []string{p}, ClaudeDirOverride: dir}, true)
		if err == nil || !strings.Contains(err.Error(), "sync_paths") {
			t.Errorf("sync path %q: err = %v, want sync_paths error", p, err)
		}
	}
}

// TestNewSyncerRejectsEmptySyncPaths guards against an empty sync_paths list,
// which would make push report every tracked file as deleted.
func TestNewSyncerRejectsEmptySyncPaths(t *testing.T) {
	_, err := NewSyncer(&config.Config{}, true)
	if err == nil || !strings.Contains(err.Error(), "sync_paths") {
		t.Errorf("NewSyncer with empty sync_paths: err = %v, want sync_paths error", err)
	}
}

// TestGetLocalFilesRejectsTraversingSyncPath ensures a sync_paths entry cannot
// walk outside ~/.claude. Sync paths became user-controlled input once the
// override was honored, and a traversing entry would otherwise turn files like
// ~/.ssh/id_rsa into remote objects.
func TestGetLocalFilesRejectsTraversingSyncPath(t *testing.T) {
	root := t.TempDir()
	claudeDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(claudeDir, 0700); err != nil {
		t.Fatal(err)
	}

	secretDir := filepath.Join(root, ".ssh")
	if err := os.MkdirAll(secretDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretDir, "id_rsa"), []byte("PRIVATE"), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := GetLocalFiles(claudeDir, []string{"../.ssh", "../.ssh/id_rsa"})
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 0 {
		t.Errorf("traversing sync path escaped the sync root: %v", sortedKeys(got))
	}
}

func sortedKeys(m map[string]os.FileInfo) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
