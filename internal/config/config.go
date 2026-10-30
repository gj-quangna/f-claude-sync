package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/tawanorg/claude-sync/internal/storage"
	"gopkg.in/yaml.v3"
)

const (
	StateFile = "state.json"

	// Environment variables holding the location of each of claude-sync's own
	// files. All three are required; there is no default location.
	EnvConfigPath = "CLAUDE_SYNC_CONFIG_PATH"
	EnvAgePath    = "CLAUDE_SYNC_AGE_PATH"
	EnvStatePath  = "CLAUDE_SYNC_STATE_PATH"

	// MCPRemoteKey is the remote storage key for synced MCP server configs.
	// The _external/ prefix separates it from ~/.claude/-relative files.
	MCPRemoteKey = "_external/mcp-servers.json"
)

// MCPSyncConfig is the mcp_sync block of config.yaml.
type MCPSyncConfig struct {
	Enabled bool `yaml:"enabled"`
	// Path is the absolute path to the .claude.json holding mcpServers.
	Path string `yaml:"path"`
}

type Config struct {
	// New storage configuration (preferred)
	Storage *storage.StorageConfig `yaml:"storage,omitempty"`

	// Legacy R2-only fields (for backward compatibility)
	AccountID       string `yaml:"account_id,omitempty"`
	AccessKeyID     string `yaml:"access_key_id,omitempty"`
	SecretAccessKey string `yaml:"secret_access_key,omitempty"`
	Bucket          string `yaml:"bucket,omitempty"`
	Endpoint        string `yaml:"endpoint,omitempty"`

	// EncryptionKey is the age key path, always taken from $CLAUDE_SYNC_AGE_PATH
	// (never read from or written to the config file).
	EncryptionKey string `yaml:"-"`

	// SyncPaths lists absolute paths inside ~/.claude to sync. It is the
	// single source of truth: there is no built-in fallback, and an empty or
	// invalid list is rejected by the syncer (see RequireSyncPaths). init
	// writes DefaultSyncPaths here. Remote keys stay ~/.claude-relative.
	SyncPaths []string `yaml:"sync_paths"`

	// Exclude patterns (doublestar globs) for paths to skip during sync. An
	// absolute pattern inside ~/.claude matches that location; a relative
	// pattern (e.g. "**/node_modules/**") is matched against ~/.claude-relative
	// paths.
	Exclude []string `yaml:"exclude"`

	// MCPSync controls syncing MCP server configs from a .claude.json file.
	// When missing from the file it defaults to enabled with ~/.claude.json.
	MCPSync *MCPSyncConfig `yaml:"mcp_sync"`

	// PathMap maps local directory prefixes to shared token names so project
	// sessions stay resumable across devices with different layouts.
	// The home directory is always mapped (token HOME); add entries here when
	// project roots differ beyond that, e.g.:
	//   path_map:
	//     ~/work: WORK        # this device keeps projects in ~/work
	// with the other device mapping its own location to the same token:
	//   path_map:
	//     ~/Projects: WORK
	PathMap map[string]string `yaml:"path_map,omitempty"`

	// ClaudeDirOverride allows overriding the default ~/.claude path (for testing)
	ClaudeDirOverride string `yaml:"-"`

	// StateDirOverride allows overriding the state file directory (for testing)
	StateDirOverride string `yaml:"-"`

	// ClaudeJSONOverride allows overriding the ~/.claude.json path (for testing)
	ClaudeJSONOverride string `yaml:"-"`
}

// DefaultSyncNames are the entries under ~/.claude that init and 'paths reset'
// put into sync_paths (as absolute paths, see DefaultSyncPaths).
var DefaultSyncNames = []string{
	"CLAUDE.md",
	"settings.json",
	"settings.local.json",
	"agents",
	"commands",
	"skills",
	"plugins",
	"projects",
	"plans",
	"tasks",
	"history.jsonl",
	"rules",
	"workflows",
}

// DefaultSyncPaths returns DefaultSyncNames as absolute paths under claudeDir.
// It is only a template: the syncer reads Config.SyncPaths.
func DefaultSyncPaths(claudeDir string) []string {
	out := make([]string, len(DefaultSyncNames))
	for i, name := range DefaultSyncNames {
		out[i] = filepath.Join(claudeDir, name)
	}
	return out
}

// RelSyncPath validates that p is an absolute path strictly inside claudeDir
// and returns it relative to claudeDir with forward slashes.
func RelSyncPath(p, claudeDir string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("sync path cannot be empty")
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("sync path %q must be absolute, e.g. %s", p, filepath.Join(claudeDir, filepath.Clean(p)))
	}
	rel, err := filepath.Rel(claudeDir, filepath.Clean(p))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("sync path %q must be inside %s", p, claudeDir)
	}
	return filepath.ToSlash(rel), nil
}

// ErrNoHomeDir is returned when the user's home directory cannot be determined.
var ErrNoHomeDir = fmt.Errorf("could not determine home directory (is $HOME set?)")

// ConfigFilePath returns the config.yaml path from $CLAUDE_SYNC_CONFIG_PATH.
func ConfigFilePath() (string, error) {
	return pathFromEnv(EnvConfigPath)
}

// StateFilePath returns the state.json path from $CLAUDE_SYNC_STATE_PATH.
func StateFilePath() (string, error) {
	return pathFromEnv(EnvStatePath)
}

// AgeKeyFilePath returns the age key path from $CLAUDE_SYNC_AGE_PATH.
func AgeKeyFilePath() (string, error) {
	return pathFromEnv(EnvAgePath)
}

// CheckEnv returns an error naming every required path variable that is unset.
func CheckEnv() error {
	var missing []string
	for _, v := range []string{EnvConfigPath, EnvAgePath, EnvStatePath} {
		if os.Getenv(v) == "" {
			missing = append(missing, v)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("required environment variable(s) not set: %s\n"+
		"  claude-sync has no default location for its files; export each one, e.g.:\n"+
		"    export %s=~/.config/claude-sync/config.yaml\n"+
		"    export %s=~/.config/claude-sync/age-key.txt\n"+
		"    export %s=~/.local/state/claude-sync/state.json",
		strings.Join(missing, ", "), EnvConfigPath, EnvAgePath, EnvStatePath)
}

// pathFromEnv returns the value of envVar with a leading ~ expanded, or an
// error when it is unset.
func pathFromEnv(envVar string) (string, error) {
	p := os.Getenv(envVar)
	if p == "" {
		return "", fmt.Errorf("$%s is not set: claude-sync has no default location for this file, export it first", envVar)
	}
	return expandHome(p), nil
}

// expandHome expands a leading "~" or "~/" to the user's home directory.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}

func ClaudeDir() string {
	path, _ := ClaudeDirE()
	return path
}

// ClaudeDirE returns the Claude directory path or an error if home dir is unavailable.
func ClaudeDirE() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ErrNoHomeDir
	}
	return filepath.Join(home, ".claude"), nil
}

// ClaudeJSONPath returns the path to ~/.claude.json where global MCP servers are configured.
func ClaudeJSONPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude.json")
}

func Load() (*Config, error) {
	configPath, err := ConfigFilePath()
	if err != nil {
		return nil, err
	}
	keyPath, err := AgeKeyFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found: run 'claude-sync init' first")
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cfg.EncryptionKey = keyPath
	if cfg.MCPSync == nil {
		cfg.MCPSync = &MCPSyncConfig{Enabled: true, Path: ClaudeJSONPath()}
	}

	// Expand ~ in path_map keys
	if len(cfg.PathMap) > 0 {
		home, _ := os.UserHomeDir()
		expanded := make(map[string]string, len(cfg.PathMap))
		for p, name := range cfg.PathMap {
			if p != "" && p[0] == '~' {
				p = filepath.Join(home, p[1:])
			}
			expanded[p] = name
		}
		cfg.PathMap = expanded
	}

	// Set default endpoint for Cloudflare R2
	if cfg.Endpoint == "" && cfg.AccountID != "" {
		cfg.Endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)
	}

	return &cfg, nil
}

func Save(cfg *Config) error {
	configPath, err := ConfigFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// Exists reports whether the config file exists. It is false when
// $CLAUDE_SYNC_CONFIG_PATH is unset.
func Exists() bool {
	configPath, err := ConfigFilePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(configPath)
	return err == nil
}

// GetStorageConfig returns the storage configuration, migrating from legacy format if needed
func (c *Config) GetStorageConfig() *storage.StorageConfig {
	// If new format is already configured, use it
	if c.Storage != nil && c.Storage.Provider != "" {
		return c.Storage
	}

	// Migrate from legacy R2 format
	return &storage.StorageConfig{
		Provider:        storage.ProviderR2,
		Bucket:          c.Bucket,
		AccountID:       c.AccountID,
		AccessKeyID:     c.AccessKeyID,
		SecretAccessKey: c.SecretAccessKey,
		Endpoint:        c.Endpoint,
	}
}

// IsLegacyConfig returns true if using the legacy R2-only config format
func (c *Config) IsLegacyConfig() bool {
	return c.Storage == nil && c.AccountID != ""
}

// ClaudeDirPath returns the ~/.claude directory, honoring ClaudeDirOverride.
func (c *Config) ClaudeDirPath() string {
	if c.ClaudeDirOverride != "" {
		return c.ClaudeDirOverride
	}
	return ClaudeDir()
}

// RequireSyncPaths returns an error when sync_paths is empty or holds an entry
// that is not an absolute path inside ~/.claude. Syncing an empty set would
// make DetectChanges observe zero local files and report every tracked file as
// a deletion, wiping the remote on the next push.
func (c *Config) RequireSyncPaths() error {
	if len(c.SyncPaths) == 0 {
		return fmt.Errorf("sync_paths is empty in config.yaml: add entries with 'claude-sync paths add <absolute path>' or restore the defaults with 'claude-sync paths reset'")
	}
	_, err := c.RelSyncPaths(c.ClaudeDirPath())
	return err
}

// RelSyncPaths converts sync_paths to claudeDir-relative paths, the form used
// for walking and for remote keys.
func (c *Config) RelSyncPaths(claudeDir string) ([]string, error) {
	rel := make([]string, 0, len(c.SyncPaths))
	for _, p := range c.SyncPaths {
		r, err := RelSyncPath(p, claudeDir)
		if err != nil {
			return nil, fmt.Errorf("invalid sync_paths entry in config.yaml: %w", err)
		}
		rel = append(rel, r)
	}
	return rel, nil
}

// IsMCPSyncEnabled reports whether MCP sync is enabled. Unset (nil) means
// enabled, matching the mcp_sync default.
func (c *Config) IsMCPSyncEnabled() bool {
	return c.MCPSync == nil || c.MCPSync.Enabled
}

// SetMCPSync sets mcp_sync.enabled, keeping the configured path (or filling
// in ~/.claude.json when the block is missing).
func (c *Config) SetMCPSync(enabled bool) {
	if c.MCPSync == nil {
		c.MCPSync = &MCPSyncConfig{Path: ClaudeJSONPath()}
	}
	c.MCPSync.Enabled = enabled
}

// MCPConfigPath returns mcp_sync.path, the absolute path to the .claude.json
// whose mcpServers are synced. A missing block means ~/.claude.json.
func (c *Config) MCPConfigPath() (string, error) {
	if c.MCPSync == nil {
		return ClaudeJSONPath(), nil
	}
	if c.MCPSync.Path == "" {
		return "", fmt.Errorf("mcp_sync.path is empty in config.yaml: set it to the absolute path of .claude.json")
	}
	if !filepath.IsAbs(c.MCPSync.Path) {
		return "", fmt.Errorf("mcp_sync.path %q in config.yaml must be an absolute path", c.MCPSync.Path)
	}
	return c.MCPSync.Path, nil
}

// IsExcluded returns true if the given relative path matches any exclude pattern.
// Patterns support:
//   - Full doublestar glob syntax including ** for recursive matching
//   - Examples: "**/.git/**", "*.tmp", "plugins/cache/**", "projects/*/node_modules/**"
//   - Directory prefix (e.g. "plugins/marketplace" matches everything under it)
//   - Filename glob (e.g. "*.tmp" matches "foo/bar/file.tmp")
func (c *Config) IsExcluded(relPath string) bool {
	return c.IsExcludedUnder(c.ClaudeDirPath(), relPath)
}

// IsExcludedUnder is IsExcluded with an explicit ~/.claude directory, used to
// turn absolute patterns into claudeDir-relative ones.
func (c *Config) IsExcludedUnder(claudeDir, relPath string) bool {
	// Normalize path separators for consistent matching
	relPath = filepath.ToSlash(relPath)

	for _, pattern := range c.Exclude {
		if filepath.IsAbs(pattern) {
			rel, err := filepath.Rel(claudeDir, pattern)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue // outside ~/.claude: can never match a synced file
			}
			pattern = rel
		}
		// Normalize pattern separators
		pattern = filepath.ToSlash(pattern)

		// Use doublestar for full glob matching including ** support
		matched, err := doublestar.Match(pattern, relPath)
		if err == nil && matched {
			return true
		}

		// Try glob match on filename only (for patterns like "*.tmp")
		// but only if the pattern doesn't contain path separators
		if !strings.Contains(pattern, "/") && (strings.Contains(pattern, "*") || strings.Contains(pattern, "?")) {
			if matched, _ := doublestar.Match(pattern, filepath.Base(relPath)); matched {
				return true
			}
		}

		// Also match if the path starts with the pattern as a directory prefix
		// This lets "plugins/marketplace" exclude everything under that dir
		if !strings.Contains(pattern, "*") && !strings.Contains(pattern, "?") {
			if len(relPath) > len(pattern) && relPath[:len(pattern)] == pattern &&
				relPath[len(pattern)] == '/' {
				return true
			}
			// Exact match for non-glob patterns
			if relPath == pattern {
				return true
			}
		}
	}
	return false
}
