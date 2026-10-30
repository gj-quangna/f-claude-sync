package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tawanorg/claude-sync/internal/storage"
)

// setEnvPaths points all three CLAUDE_SYNC_*_PATH variables into a fresh temp
// dir and returns it.
func setEnvPaths(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(EnvConfigPath, filepath.Join(dir, "config.yaml"))
	t.Setenv(EnvAgePath, filepath.Join(dir, "age-key.txt"))
	t.Setenv(EnvStatePath, filepath.Join(dir, "state.json"))
	return dir
}

func TestClaudeDir(t *testing.T) {
	path := ClaudeDir()
	if path == "" {
		t.Fatal("ClaudeDir should not return empty string")
	}

	if !strings.HasSuffix(path, ".claude") {
		t.Errorf("ClaudeDir should end with '.claude', got '%s'", path)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := setEnvPaths(t)

	cfg := &Config{
		AccountID:       "test-account-id",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
	}

	data := `account_id: test-account-id
access_key_id: test-access-key
secret_access_key: test-secret-key
bucket: test-bucket
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(data), 0600); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Load config
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify loaded config
	if loaded.AccountID != cfg.AccountID {
		t.Errorf("AccountID mismatch: expected '%s', got '%s'", cfg.AccountID, loaded.AccountID)
	}
	if loaded.AccessKeyID != cfg.AccessKeyID {
		t.Errorf("AccessKeyID mismatch: expected '%s', got '%s'", cfg.AccessKeyID, loaded.AccessKeyID)
	}
	if loaded.SecretAccessKey != cfg.SecretAccessKey {
		t.Errorf("SecretAccessKey mismatch: expected '%s', got '%s'", cfg.SecretAccessKey, loaded.SecretAccessKey)
	}
	if loaded.Bucket != cfg.Bucket {
		t.Errorf("Bucket mismatch: expected '%s', got '%s'", cfg.Bucket, loaded.Bucket)
	}

	// Key path always comes from $CLAUDE_SYNC_AGE_PATH
	if want := filepath.Join(dir, "age-key.txt"); loaded.EncryptionKey != want {
		t.Errorf("EncryptionKey = %q, want %q", loaded.EncryptionKey, want)
	}

	// Check that endpoint is auto-populated
	expectedEndpoint := "https://test-account-id.r2.cloudflarestorage.com"
	if loaded.Endpoint != expectedEndpoint {
		t.Errorf("Endpoint mismatch: expected '%s', got '%s'", expectedEndpoint, loaded.Endpoint)
	}
}

func TestLoadNotFound(t *testing.T) {
	setEnvPaths(t)

	_, err := Load()
	if err == nil {
		t.Fatal("Load should fail when config doesn't exist")
	}

	if !strings.Contains(err.Error(), "run 'claude-sync init' first") {
		t.Errorf("Error should mention running init, got: %v", err)
	}
}

func TestLoadRequiresEnv(t *testing.T) {
	for _, v := range []string{EnvConfigPath, EnvAgePath} {
		t.Run(v, func(t *testing.T) {
			setEnvPaths(t)
			t.Setenv(v, "")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), v) {
				t.Errorf("Load error = %v, want mention of %s", err, v)
			}
		})
	}
}

func TestExists(t *testing.T) {
	dir := setEnvPaths(t)

	if Exists() {
		t.Error("Exists should return false when config doesn't exist")
	}

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("test"), 0600); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}
	if !Exists() {
		t.Error("Exists should return true when config exists")
	}

	t.Setenv(EnvConfigPath, "")
	if Exists() {
		t.Error("Exists should return false when $CLAUDE_SYNC_CONFIG_PATH is unset")
	}
}

func TestDefaultSyncPaths(t *testing.T) {
	// Verify the init template contains expected entries
	expectedPaths := map[string]bool{
		"CLAUDE.md":           false,
		"settings.json":       false,
		"settings.local.json": false,
		"agents":              false,
		"commands":            false,
		"skills":              false,
		"plugins":             false,
		"projects":            false,
		"history.jsonl":       false,
		"rules":               false,
	}

	for _, path := range DefaultSyncNames {
		if _, ok := expectedPaths[path]; ok {
			expectedPaths[path] = true
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("Expected path '%s' not found in DefaultSyncNames", path)
		}
	}

	claudeDir := filepath.Join(t.TempDir(), ".claude")
	for i, p := range DefaultSyncPaths(claudeDir) {
		if want := filepath.Join(claudeDir, DefaultSyncNames[i]); p != want {
			t.Errorf("DefaultSyncPaths[%d] = %q, want %q", i, p, want)
		}
	}
}

func TestRelSyncPath(t *testing.T) {
	claudeDir := filepath.Join(t.TempDir(), ".claude")
	valid := map[string]string{
		filepath.Join(claudeDir, "CLAUDE.md"):         "CLAUDE.md",
		filepath.Join(claudeDir, "projects", "foo"):   "projects/foo",
		filepath.Join(claudeDir, "a", "..", "agents"): "agents",
	}
	for in, want := range valid {
		if got, err := RelSyncPath(in, claudeDir); err != nil || got != want {
			t.Errorf("RelSyncPath(%q) = (%q, %v), want %q", in, got, err, want)
		}
	}

	invalid := []string{
		"",
		"CLAUDE.md",             // relative
		"~/.claude/CLAUDE.md",   // not expanded
		claudeDir,               // the root itself
		filepath.Dir(claudeDir), // parent
		filepath.Join(claudeDir, "..", ".ssh"),
		claudeDir + "-other/x", // sibling sharing the prefix
		"/etc/passwd",
	}
	for _, in := range invalid {
		if got, err := RelSyncPath(in, claudeDir); err == nil {
			t.Errorf("RelSyncPath(%q) = %q, want error", in, got)
		}
	}
}

func TestClaudeJSONPath(t *testing.T) {
	path := ClaudeJSONPath()
	if path == "" {
		t.Fatal("ClaudeJSONPath should not return empty string")
	}
	if !strings.HasSuffix(path, ".claude.json") {
		t.Errorf("ClaudeJSONPath should end with .claude.json, got %q", path)
	}
}

func TestGetStorageConfig_NewFormat(t *testing.T) {
	cfg := &Config{
		Storage: &storage.StorageConfig{
			Provider: storage.ProviderS3,
			Bucket:   "my-bucket",
			Region:   "us-east-1",
		},
	}

	sc := cfg.GetStorageConfig()
	if sc.Provider != storage.ProviderS3 {
		t.Errorf("expected provider S3, got %q", sc.Provider)
	}
	if sc.Bucket != "my-bucket" {
		t.Errorf("expected bucket my-bucket, got %q", sc.Bucket)
	}
}

func TestGetStorageConfig_LegacyFormat(t *testing.T) {
	cfg := &Config{
		AccountID:       "abc123",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
		Bucket:          "legacy-bucket",
		Endpoint:        "https://abc123.r2.cloudflarestorage.com",
	}

	sc := cfg.GetStorageConfig()
	if sc.Provider != storage.ProviderR2 {
		t.Errorf("expected provider R2 for legacy config, got %q", sc.Provider)
	}
	if sc.Bucket != "legacy-bucket" {
		t.Errorf("expected bucket legacy-bucket, got %q", sc.Bucket)
	}
	if sc.AccountID != "abc123" {
		t.Errorf("expected account ID abc123, got %q", sc.AccountID)
	}
}

func TestIsLegacyConfig(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		expected bool
	}{
		{"legacy with account ID", Config{AccountID: "abc"}, true},
		{"new format with storage", Config{Storage: &storage.StorageConfig{Provider: "s3"}}, false},
		{"empty config", Config{}, false},
		{"both set uses new format", Config{Storage: &storage.StorageConfig{Provider: "s3"}, AccountID: "abc"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsLegacyConfig(); got != tt.expected {
				t.Errorf("IsLegacyConfig() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsExcluded(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		patterns []string
		expected bool
	}{
		// Directory wildcard patterns with /**
		{"exclude dir with /**", "plugins/cache/foo/bar.js", []string{"plugins/cache/**"}, true},
		{"exclude dir itself", "plugins/cache", []string{"plugins/cache/**"}, true},
		{"exclude nested dir", "plugins/marketplaces/repo/file.txt", []string{"plugins/marketplaces/**"}, true},
		{"non-matching dir", "plugins/installed.json", []string{"plugins/cache/**"}, false},

		// Filename glob patterns
		{"exclude by extension", "projects/foo/debug.tmp", []string{"*.tmp"}, true},
		{"exclude dotfile glob", "projects/.DS_Store", []string{".*"}, true},
		{"non-matching extension", "projects/foo/file.json", []string{"*.tmp"}, false},

		// Exact path patterns
		{"exact file match", "debug/log.txt", []string{"debug/log.txt"}, true},
		{"exact dir pattern with /**", "debug", []string{"debug/**"}, true},

		// Directory prefix (without /**)
		{"dir prefix match", "plugins/marketplace/repo/file.txt", []string{"plugins/marketplace"}, true},
		{"dir prefix exact", "plugins/marketplace", []string{"plugins/marketplace"}, true},

		// Multiple patterns
		{"first pattern matches", "plugins/cache/mod.js", []string{"plugins/cache/**", "*.tmp"}, true},
		{"second pattern matches", "foo.tmp", []string{"plugins/cache/**", "*.tmp"}, true},
		{"no pattern matches", "settings.json", []string{"plugins/cache/**", "*.tmp"}, false},

		// Empty patterns
		{"empty patterns", "anything.txt", []string{}, false},
		{"nil-like empty", "anything.txt", nil, false},

		// Edge cases
		{"partial name no match", "plugins/cachedata/file.txt", []string{"plugins/cache/**"}, false},
		{"shell-snapshots", "shell-snapshots/snap.json", []string{"shell-snapshots/**"}, true},
		{"telemetry dir", "telemetry/data.json", []string{"telemetry/**"}, true},

		// Recursive globstar patterns (Issue #43)
		{"globstar .git at root", ".git/HEAD", []string{"**/.git/**"}, true},
		{"globstar .git nested", "projects/someproject/.git/config", []string{"**/.git/**"}, true},
		{"globstar .git deeply nested", "projects/foo/bar/.git/objects/ab/cd", []string{"**/.git/**"}, true},
		{"globstar .git dir itself", "projects/app/.git", []string{"**/.git/**"}, true},
		{"globstar non-matching", "projects/app/git/config", []string{"**/.git/**"}, false},
		{"globstar node_modules anywhere", "projects/foo/node_modules/lodash/index.js", []string{"**/node_modules/**"}, true},
		{"globstar node_modules deep", "a/b/c/d/node_modules/pkg/lib/file.js", []string{"**/node_modules/**"}, true},
		{"leading ** with extension", "deeply/nested/path/file.log", []string{"**/*.log"}, true},
		{"combined patterns from issue", "projects/foo/.git/objects/ab", []string{"*.tmp", "projects/*/node_modules/*", "**/.git/**"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Exclude: tt.patterns}
			result := cfg.IsExcluded(tt.path)
			if result != tt.expected {
				t.Errorf("IsExcluded(%q) with patterns %v = %v, want %v", tt.path, tt.patterns, result, tt.expected)
			}
		})
	}
}

func TestRequireSyncPaths(t *testing.T) {
	claudeDir := t.TempDir()
	for _, paths := range [][]string{nil, {}, {"projects"}, {filepath.Join(claudeDir, "projects"), "/etc"}} {
		cfg := &Config{SyncPaths: paths, ClaudeDirOverride: claudeDir}
		if err := cfg.RequireSyncPaths(); err == nil || !strings.Contains(err.Error(), "sync_paths") {
			t.Errorf("RequireSyncPaths(%v) = %v, want sync_paths error", paths, err)
		}
	}
	cfg := &Config{SyncPaths: []string{filepath.Join(claudeDir, "projects")}, ClaudeDirOverride: claudeDir}
	if err := cfg.RequireSyncPaths(); err != nil {
		t.Errorf("RequireSyncPaths with absolute entries: %v", err)
	}
	if rel, err := cfg.RelSyncPaths(claudeDir); err != nil || !reflect.DeepEqual(rel, []string{"projects"}) {
		t.Errorf("RelSyncPaths = (%v, %v), want [projects]", rel, err)
	}
}

func TestIsExcludedAbsolutePattern(t *testing.T) {
	claudeDir := t.TempDir()
	cfg := &Config{Exclude: []string{
		filepath.Join(claudeDir, "plugins", "**", "node_modules", "**"),
		"/elsewhere/**", // outside ~/.claude: never matches
	}}
	if !cfg.IsExcludedUnder(claudeDir, "plugins/a/node_modules/x.js") {
		t.Error("absolute pattern inside claudeDir should match")
	}
	if cfg.IsExcludedUnder(claudeDir, "projects/a/node_modules/x.js") {
		t.Error("absolute pattern must only match its own location")
	}
}

func TestIsMCPSyncEnabled(t *testing.T) {
	tests := []struct {
		name    string
		mcpSync *MCPSyncConfig
		want    bool
	}{
		{
			name:    "nil (unset) defaults to true",
			mcpSync: nil,
			want:    true,
		},
		{
			name:    "enabled returns true",
			mcpSync: &MCPSyncConfig{Enabled: true},
			want:    true,
		},
		{
			name:    "disabled returns false",
			mcpSync: &MCPSyncConfig{Enabled: false},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{MCPSync: tt.mcpSync}
			if got := cfg.IsMCPSyncEnabled(); got != tt.want {
				t.Errorf("IsMCPSyncEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetMCPSync(t *testing.T) {
	cfg := &Config{}

	// A missing block is created with ~/.claude.json
	cfg.SetMCPSync(true)
	if cfg.MCPSync == nil || !cfg.MCPSync.Enabled || cfg.MCPSync.Path != ClaudeJSONPath() {
		t.Errorf("SetMCPSync(true) on nil = %+v", cfg.MCPSync)
	}

	// Toggling keeps a custom path
	cfg.MCPSync.Path = "/custom/.claude.json"
	cfg.SetMCPSync(false)
	if cfg.MCPSync.Enabled || cfg.MCPSync.Path != "/custom/.claude.json" {
		t.Errorf("SetMCPSync(false) = %+v, want disabled with custom path", cfg.MCPSync)
	}
}

func TestMCPConfigPath(t *testing.T) {
	if got, err := (&Config{}).MCPConfigPath(); err != nil || got != ClaudeJSONPath() {
		t.Errorf("nil block: (%q, %v), want %q", got, err, ClaudeJSONPath())
	}
	cfg := &Config{MCPSync: &MCPSyncConfig{Enabled: true, Path: "/abs/.claude.json"}}
	if got, err := cfg.MCPConfigPath(); err != nil || got != "/abs/.claude.json" {
		t.Errorf("absolute path: (%q, %v)", got, err)
	}
	for _, p := range []string{"", "relative/.claude.json", "~/.claude.json"} {
		cfg := &Config{MCPSync: &MCPSyncConfig{Enabled: true, Path: p}}
		if _, err := cfg.MCPConfigPath(); err == nil || !strings.Contains(err.Error(), "mcp_sync.path") {
			t.Errorf("path %q: err = %v, want mcp_sync.path error", p, err)
		}
	}
}

func TestFilePathsFromEnv(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}

	tests := []struct {
		envVar string
		fn     func() (string, error)
	}{
		{EnvConfigPath, ConfigFilePath},
		{EnvAgePath, AgeKeyFilePath},
		{EnvStatePath, StateFilePath},
	}

	for _, tt := range tests {
		t.Run(tt.envVar+" unset is an error", func(t *testing.T) {
			t.Setenv(tt.envVar, "")
			_, err := tt.fn()
			if err == nil || !strings.Contains(err.Error(), tt.envVar) {
				t.Errorf("err = %v, want mention of %s", err, tt.envVar)
			}
		})

		t.Run(tt.envVar+" absolute path", func(t *testing.T) {
			want := filepath.Join(t.TempDir(), "custom", "file")
			t.Setenv(tt.envVar, want)
			if got, err := tt.fn(); err != nil || got != want {
				t.Errorf("got (%q, %v), want %q", got, err, want)
			}
		})

		t.Run(tt.envVar+" expands ~", func(t *testing.T) {
			t.Setenv(tt.envVar, "~/elsewhere/file")
			want := filepath.Join(home, "elsewhere", "file")
			if got, err := tt.fn(); err != nil || got != want {
				t.Errorf("got (%q, %v), want %q", got, err, want)
			}
		})
	}
}

func TestCheckEnv(t *testing.T) {
	setEnvPaths(t)
	if err := CheckEnv(); err != nil {
		t.Fatalf("CheckEnv with all vars set: %v", err)
	}

	t.Setenv(EnvAgePath, "")
	t.Setenv(EnvStatePath, "")
	err := CheckEnv()
	if err == nil {
		t.Fatal("CheckEnv should fail when vars are unset")
	}
	for _, v := range []string{EnvAgePath, EnvStatePath} {
		if !strings.Contains(err.Error(), v) {
			t.Errorf("error should name %s, got: %v", v, err)
		}
	}
	if strings.Contains(err.Error(), EnvConfigPath+",") {
		t.Errorf("error should not list %s as missing: %v", EnvConfigPath, err)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	tests := map[string]string{
		"~":           home,
		"~/a/b":       filepath.Join(home, "a", "b"),
		"/abs/path":   "/abs/path",
		"rel/path":    "rel/path",
		"~other/file": "~other/file",
	}
	for in, want := range tests {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSaveAndLoadWithEnvPaths(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "nested", "dir", "config.yaml")
	keyPath := filepath.Join(tmp, "keys", "age-key.txt")
	t.Setenv(EnvConfigPath, configPath)
	t.Setenv(EnvAgePath, keyPath)
	t.Setenv(EnvStatePath, filepath.Join(tmp, "state.json"))

	if err := Save(&Config{EncryptionKey: "/ignored", Bucket: "test-bucket"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("config not written to env path: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("config perms = %o, want 600", perm)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "encryption_key_path") {
		t.Errorf("key path must not be written to config:\n%s", data)
	}

	// A legacy encryption_key_path in the file is ignored in favour of the env var
	legacy := string(data) + "encryption_key_path: ~/.claude-sync/age-key.txt\n"
	if err := os.WriteFile(configPath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Bucket != "test-bucket" {
		t.Errorf("Bucket = %q, want test-bucket", loaded.Bucket)
	}
	if loaded.EncryptionKey != keyPath {
		t.Errorf("EncryptionKey = %q, want %q", loaded.EncryptionKey, keyPath)
	}
}

func TestSyncSettingsRoundTrip(t *testing.T) {
	dir := setEnvPaths(t)
	configPath := filepath.Join(dir, "config.yaml")

	t.Run("mcp_sync defaults to true when missing", func(t *testing.T) {
		if err := os.WriteFile(configPath, []byte("sync_paths: [projects]\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.MCPSync == nil || !cfg.MCPSync.Enabled || cfg.MCPSync.Path != ClaudeJSONPath() {
			t.Errorf("MCPSync = %+v, want enabled with %s", cfg.MCPSync, ClaudeJSONPath())
		}
	})

	t.Run("sync_paths, exclude and mcp_sync are always written", func(t *testing.T) {
		cfg := &Config{
			SyncPaths: []string{"/home/u/.claude/projects"},
			Exclude:   []string{"**/*.tmp"},
			MCPSync:   &MCPSyncConfig{Enabled: false, Path: "/home/u/.claude.json"},
		}
		if err := Save(cfg); err != nil {
			t.Fatalf("Save: %v", err)
		}
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"sync_paths:", "- /home/u/.claude/projects", "exclude:", "mcp_sync:", "enabled: false", "path: /home/u/.claude.json"} {
			if !strings.Contains(string(data), key) {
				t.Errorf("config missing %q:\n%s", key, data)
			}
		}
		if strings.Contains(string(data), "scope:") {
			t.Errorf("scope must no longer be written:\n%s", data)
		}

		loaded, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !reflect.DeepEqual(loaded.SyncPaths, cfg.SyncPaths) || !reflect.DeepEqual(loaded.Exclude, cfg.Exclude) {
			t.Errorf("round trip = %v / %v, want %v / %v", loaded.SyncPaths, loaded.Exclude, cfg.SyncPaths, cfg.Exclude)
		}
		if loaded.IsMCPSyncEnabled() || loaded.MCPSync.Path != "/home/u/.claude.json" {
			t.Errorf("mcp_sync not kept: %+v", loaded.MCPSync)
		}
	})
}
