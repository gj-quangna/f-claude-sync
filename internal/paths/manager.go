// Package paths provides business logic for managing sync paths and exclude filters.
// It separates the path management concerns from CLI and config layers.
package paths

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/tawanorg/claude-sync/internal/config"
)

// ValidatePath rejects sync path entries that are not absolute paths inside
// claudeDir. A sync path becomes a filesystem walk root on push and a write
// target on pull, so anything outside ~/.claude must never be accepted.
func ValidatePath(path, claudeDir string) error {
	_, err := config.RelSyncPath(path, claudeDir)
	return err
}

// Manager handles sync path and exclude filter operations on the lists stored
// in config.yaml. It provides the business logic separate from persistence.
type Manager struct {
	syncPaths []string
	excludes  []string
	claudeDir string
}

// NewManager creates a Manager over the sync_paths and exclude lists from
// config.yaml. If claudeDir is empty, it defaults to ~/.claude.
func NewManager(syncPaths, excludes []string, claudeDir string) *Manager {
	if claudeDir == "" {
		home, _ := os.UserHomeDir()
		claudeDir = filepath.Join(home, ".claude")
	}

	return &Manager{
		syncPaths: append([]string{}, syncPaths...),
		excludes:  append([]string{}, excludes...),
		claudeDir: claudeDir,
	}
}

// SyncPaths returns the current sync paths.
func (m *Manager) SyncPaths() []string {
	return m.syncPaths
}

// Excludes returns the current exclude patterns.
func (m *Manager) Excludes() []string {
	return m.excludes
}

// AddResult contains the result of an Add operation.
type AddResult struct {
	Added           bool
	AlreadyExists   bool
	PathMissing     bool
	ExcludesRemoved int
	// Invalid is set when the path escapes ~/.claude; nothing was changed.
	Invalid error
}

// Add adds a path to the sync list.
// If the path was previously excluded, conflicting excludes are removed.
// Returns details about what changed.
func (m *Manager) Add(path string) AddResult {
	result := AddResult{}

	if err := ValidatePath(path, m.claudeDir); err != nil {
		result.Invalid = err
		return result
	}

	// Check if already in list
	for _, p := range m.syncPaths {
		if p == path {
			result.AlreadyExists = true
			return result
		}
	}

	// Check if path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		result.PathMissing = true
	}

	// Add to sync paths
	m.syncPaths = append(m.syncPaths, path)
	result.Added = true

	// Remove conflicting excludes
	newExcludes := m.excludes[:0:0]
	for _, e := range m.excludes {
		base := strings.TrimSuffix(strings.TrimSuffix(e, "/**"), "/*")
		if base == path || e == path {
			result.ExcludesRemoved++
			continue
		}
		newExcludes = append(newExcludes, e)
	}
	m.excludes = newExcludes

	return result
}

// RemoveResult contains the result of a Remove operation.
type RemoveResult struct {
	Removed  bool
	NotFound bool
}

// Remove removes a path from the sync list.
func (m *Manager) Remove(path string) RemoveResult {
	result := RemoveResult{}

	for i, p := range m.syncPaths {
		if p == path {
			m.syncPaths = append(m.syncPaths[:i], m.syncPaths[i+1:]...)
			result.Removed = true
			return result
		}
	}

	result.NotFound = true
	return result
}

// AddExcludeResult contains the result of an AddExclude operation.
type AddExcludeResult struct {
	Added         bool
	AlreadyExists bool
	IsSyncPath    bool
}

// AddExclude adds a glob pattern to the exclude list.
// Returns an error indicator if the pattern matches a top-level sync path
// (user should use Remove instead).
func (m *Manager) AddExclude(pattern string) AddExcludeResult {
	result := AddExcludeResult{}

	// Check if it matches a sync path (should use Remove instead)
	for _, p := range m.syncPaths {
		if p == pattern {
			result.IsSyncPath = true
			return result
		}
	}

	// Check if already excluded
	for _, e := range m.excludes {
		if e == pattern {
			result.AlreadyExists = true
			return result
		}
	}

	m.excludes = append(m.excludes, pattern)
	result.Added = true
	return result
}

// RemoveExcludeResult contains the result of a RemoveExclude operation.
type RemoveExcludeResult struct {
	Removed  bool
	NotFound bool
}

// RemoveExclude removes a pattern from the exclude list.
func (m *Manager) RemoveExclude(pattern string) RemoveExcludeResult {
	result := RemoveExcludeResult{}

	found := -1
	for i, e := range m.excludes {
		if e == pattern {
			found = i
			break
		}
	}

	if found == -1 {
		result.NotFound = true
		return result
	}

	m.excludes = append(m.excludes[:found], m.excludes[found+1:]...)
	result.Removed = true
	return result
}

// Reset replaces the sync list with config.DefaultSyncPaths and clears all excludes.
func (m *Manager) Reset() {
	m.syncPaths = config.DefaultSyncPaths(m.claudeDir)
	m.excludes = nil
}

// HasPath returns true if the path is in the current sync list.
func (m *Manager) HasPath(path string) bool {
	for _, p := range m.syncPaths {
		if p == path {
			return true
		}
	}
	return false
}

// HasExclude returns true if the pattern is in the exclude list.
func (m *Manager) HasExclude(pattern string) bool {
	for _, e := range m.excludes {
		if e == pattern {
			return true
		}
	}
	return false
}
