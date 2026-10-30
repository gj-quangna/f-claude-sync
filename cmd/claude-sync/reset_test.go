package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tawanorg/claude-sync/internal/config"
)

// setupLocalFiles points the CLAUDE_SYNC_*_PATH variables at files inside a
// shared directory that also holds an unrelated file, and creates all of them.
func setupLocalFiles(t *testing.T) (dir string, files map[string]string) {
	t.Helper()
	dir = t.TempDir()
	files = map[string]string{
		config.EnvConfigPath: filepath.Join(dir, "config.yaml"),
		config.EnvAgePath:    filepath.Join(dir, "age-key.txt"),
		config.EnvStatePath:  filepath.Join(dir, "state.json"),
		"unrelated":          filepath.Join(dir, "unrelated.txt"),
	}
	for name, p := range files {
		if name != "unrelated" {
			t.Setenv(name, p)
		}
		if err := os.WriteFile(p, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, files
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestRemoveLocalFilesKeepsStateByDefault(t *testing.T) {
	_, files := setupLocalFiles(t)

	removed, err := removeLocalFiles(false)
	if err != nil {
		t.Fatalf("removeLocalFiles: %v", err)
	}
	if len(removed) != 2 {
		t.Errorf("removed %v, want config and key only", removed)
	}
	if exists(files[config.EnvConfigPath]) || exists(files[config.EnvAgePath]) {
		t.Error("config and key should be removed")
	}
	if !exists(files[config.EnvStatePath]) {
		t.Error("state should be kept without --local")
	}
	if !exists(files["unrelated"]) {
		t.Error("unrelated file in the same directory must be kept")
	}
}

func TestRemoveLocalFilesWithState(t *testing.T) {
	dir, files := setupLocalFiles(t)

	removed, err := removeLocalFiles(true)
	if err != nil {
		t.Fatalf("removeLocalFiles: %v", err)
	}
	if len(removed) != 3 {
		t.Errorf("removed %v, want config, key and state", removed)
	}
	if exists(files[config.EnvStatePath]) {
		t.Error("state should be removed with --local")
	}
	if !exists(dir) || !exists(files["unrelated"]) {
		t.Error("containing directory and unrelated files must be kept")
	}
}

func TestRemoveLocalFilesSkipsMissing(t *testing.T) {
	_, files := setupLocalFiles(t)
	if err := os.Remove(files[config.EnvAgePath]); err != nil {
		t.Fatal(err)
	}

	removed, err := removeLocalFiles(true)
	if err != nil {
		t.Fatalf("removeLocalFiles: %v", err)
	}
	for _, p := range removed {
		if p == files[config.EnvAgePath] {
			t.Errorf("missing file reported as removed: %s", p)
		}
	}
	if len(removed) != 2 {
		t.Errorf("removed %v, want 2 entries", removed)
	}
}

func TestRemoveLocalFilesRequiresEnv(t *testing.T) {
	setupLocalFiles(t)
	t.Setenv(config.EnvAgePath, "")

	_, err := removeLocalFiles(false)
	if err == nil || !strings.Contains(err.Error(), config.EnvAgePath) {
		t.Errorf("err = %v, want mention of %s", err, config.EnvAgePath)
	}
}
