# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Purpose

`claude-sync` is a Go CLI that syncs `~/.claude/` (Claude Code state: sessions, agents, skills, plugins, settings, history, global `CLAUDE.md`) across devices via encrypted cloud storage (Cloudflare R2 / AWS S3 / S3-compatible / GCS / WebDAV). Files are gzip-compressed, then age-encrypted before upload. The canonical binary is distributed as pre-compiled platform-specific npm packages; a tiny Node wrapper at `bin/claude-sync.js` dispatches to the right binary.

## Common Commands

```bash
make build            # build ./bin/claude-sync for the host platform
make test             # go test -v ./...
make check            # pre-commit equivalent: gofmt -l, go vet, go test -short
make fmt              # go fmt ./...
make lint             # golangci-lint run (requires golangci-lint installed)
make build-all        # cross-compile darwin/linux × arm64/amd64 into ./bin/
make setup-hooks      # install .githooks/pre-commit (runs on every commit)

# Run a single test
go test -v -run TestName ./internal/sync/
go test -v ./internal/crypto/ -run TestGenerateKeyFromPassphrase

# Integration tests (require real R2 credentials; behind build tag)
go test -tags=integration -v ./integration/...
# or: cd integration && docker-compose up --build
```

Every command except `update`, `changelog`, `help` and `completion` fails fast unless all three `CLAUDE_SYNC_*_PATH` variables are set (see On-disk layout), so export them before running the binary by hand.

Version is injected at build time: `-ldflags "-X main.version=<version>"`. `make build` pulls the version from `git describe --tags --always --dirty`.

## Architecture

Layered, with a pluggable storage abstraction:

- **CLI layer** — `cmd/claude-sync/main.go`. Cobra commands (`init`, `push`, `pull`, `status`, `diff`, `conflicts`, `rebuild-history`, `reset`, `migrate`, `update`, `changelog`, `mcp`, `auto`, `paths`) plus Survey-driven interactive wizards. All user-facing output lives here. The root `PersistentPreRunE` runs `config.CheckEnv()`.
- **Sync layer** — `internal/sync/`. `Syncer` orchestrates push/pull; `SyncState` (`state.json`) tracks per-file SHA256 hash + size + mtime + last-uploaded time. `DetectChanges` compares local files against state to produce `add/modify/delete` work items. Push/pull both run uploads/downloads with a worker pool (`defaultWorkers = 10`). `PathMapper` (`paths.go`) rewrites machine-specific project paths to portable tokens (see Sync semantics).
- **Crypto layer** — `internal/crypto/encrypt.go`. Wraps `filippo.io/age` (X25519 + ChaCha20-Poly1305). Supports two key modes: random (`GenerateKey`) or passphrase-derived (`GenerateKeyFromPassphrase`, Argon2id with a **fixed salt** `sha256("claude-sync-v1")` so the same passphrase yields the same key on any device). The derived 32 bytes are clamped for X25519 then Bech32-encoded as an `AGE-SECRET-KEY-…` identity.
- **Storage layer** — `internal/storage/`. `Storage` interface (`Upload`/`Download`/`Delete`/`DeleteBatch`/`List`/`Head`/`BucketExists`) with four adapters: `r2/` (AWS SDK v2 pointed at `<account>.r2.cloudflarestorage.com`), `s3/` (AWS SDK v2, also used for S3-compatible endpoints), `gcs/` (Google Cloud Storage SDK), `webdav/`. Adapters **self-register** via `init()` functions setting package-level `storage.NewR2` / `NewS3` / `NewGCS` / `NewWebDAV` vars; `cmd/claude-sync/main.go` blank-imports them to wire up the factory (`storage.New`). Add new providers by following this pattern.
- **Config layer** — `internal/config/config.go`. YAML at `$CLAUDE_SYNC_CONFIG_PATH` (perms 0600). Supports both new unified `storage:` block and legacy R2-only top-level fields — `GetStorageConfig()` handles migration. What gets synced is defined **only** by the YAML: `sync_paths` (**absolute** paths strictly inside `~/.claude/`), `exclude` (doublestar globs; absolute ones are made `~/.claude`-relative via `IsExcludedUnder`) and the `mcp_sync: {enabled, path}` block (`path` = absolute `.claude.json`; missing block → enabled + `~/.claude.json`). There is no scope concept and no built-in fallback — `DefaultSyncPaths(claudeDir)` (from `DefaultSyncNames`) is just the template `init` and `paths reset` write; `NewSyncer` rejects empty/relative/outside entries via `RequireSyncPaths()`. Internally the syncer works on `RelSyncPaths(claudeDir)`, so remote keys and `GetLocalFiles` stay `~/.claude`-relative. `internal/paths/` + the `paths` command edit these lists in place. `Config.EncryptionKey` is `yaml:"-"`: it is always filled from `$CLAUDE_SYNC_AGE_PATH` by `Load()`, and a legacy `encryption_key_path` in the file is ignored.

### On-disk layout

claude-sync's own files have **no default location** — each comes from a required env var (leading `~` is expanded, missing parent dirs are created 0700 on first write):

| Env var | File | Perms |
|---|---|---|
| `CLAUDE_SYNC_CONFIG_PATH` | config.yaml — storage + sync_paths/exclude/mcp_sync/path_map | 0600 |
| `CLAUDE_SYNC_AGE_PATH` | age-key.txt — encryption identity (derived or random) | 0600 |
| `CLAUDE_SYNC_STATE_PATH` | state.json — per-file hash/size/mtime + last push/pull times + MCP baseline | 0600 |

`~/.claude/` is what gets synced (the `sync_paths` in config.yaml).

Resolve paths only via `config.ConfigFilePath()` / `AgeKeyFilePath()` / `StateFilePath()`, which return an error naming the unset variable; `config.CheckEnv()` reports all missing ones at once. There is no config directory to join paths onto. `reset` removes config + key (and state with `--local`) file by file — never the containing directories.

### Sync semantics

- **Remote keys** are `~/.claude`-relative paths with `.age` appended, after `PathMapper` rewrites machine-specific project dirs to portable tokens (`projects/-Users-alice-app/...` → `projects/${HOME}-app/...`; file content gets the same rewrite). Extra prefixes map via `path_map`. `migrate` converts legacy, unmapped remote keys. Files under `_external/` on remote are reserved for MCP sync (see below) and are filtered out of regular pull/diff.
- **Push** encrypts only files whose current hash differs from state; deletions detected from state are batched via `DeleteBatch`.
- **Pull** downloads when the local file is missing, or when remote `LastModified` is after the state's `Uploaded` time. If the local hash **also** differs from state (both sides changed), it's a **conflict**: local is kept, remote is written to `<path>.conflict.<timestamp>`. `claude-sync conflicts` resolves them (and updates state on resolution).
- **First pull with existing local files** is handled specially in `cmd/claude-sync/main.go` (`handleFirstPullWithExistingFiles`): shows a preview diff and offers backup-to-`~/.claude.backup.<ts>`/overwrite/abort.
- **Backward-compat read path**: decrypt always attempts gzip decompression only if magic `0x1f 0x8b` is present, so older uncompressed remote blobs still work. Write path always compresses.
- **Key verification**: during `init`, after deriving the key, `verifyKeyMatchesRemote` downloads a small remote file and tries to decrypt it. Mismatch triggers the 3-way prompt (retry passphrase / clear remote / abort).

### MCP sync

`~/.claude.json` holds global MCP server configs. Unlike regular sync, MCP uses a **three-way merge** (`internal/sync/mcp.go`) against a baseline stored in `SyncState.MCPBaseline`. On pull, local vs remote vs baseline are merged; conflicting server entries keep local. Paths inside MCP server commands/args are home-relative-normalized (`NormalizeMCPServers`) before upload and resolved back to absolute on pull. Remote key is fixed: `_external/mcp-servers.json.age`. File comes from `mcp_sync.path` (`Config.MCPConfigPath()`); on by default (block missing → enabled); turn off with `mcp_sync.enabled: false` / `claude-sync mcp disable`. `--include-mcp` forces a one-off sync when it is off.

## Distribution & release

> This describes the upstream pipeline. In this fork `main` only mirrors `upstream/main` and development happens on `me`; semantic-release, CI and release workflows trigger on `main` only, so nothing runs for `me` — use `make check` locally.

- Pre-built binaries ship as 6 platform-specific npm packages under `npm/<platform>/` (`darwin-arm64`, `darwin-x64`, `linux-arm64`, `linux-x64`, `win32-arm64`, `win32-x64`). `package.json` lists them as `optionalDependencies`; npm installs only the one matching the host. `bin/claude-sync.js` resolves and execs the right binary.
- Releases are automated via **semantic-release** (`.releaserc.json`) on pushes to `main`. The `prepareCmd` runs `VERSION=${nextRelease.version} make build-all`, then uploads the 4 Unix binaries plus `checksums.txt` to GitHub Releases. `install.js` / `claude-sync update` both download from the GitHub Releases API.
- Version bumps come from Conventional Commits (`feat:` → minor, `fix:` → patch, `feat!:`/`BREAKING CHANGE` → major). Don't hand-edit `CHANGELOG.md` or the version in `package.json` — semantic-release owns both.

## CI & pre-commit

- `.github/workflows/ci.yml` runs `go test -v ./...`, enforces **60% coverage** on `internal/*` packages, builds, and runs golangci-lint. Keep changes to `internal/` covered.
- `.githooks/pre-commit` (enabled by `make setup-hooks`) runs `gofmt -l`, `go vet`, `go test ./... -short`, and golangci-lint if installed. Run `make check` locally before committing to mirror it.

## Gotchas

- **No default file locations**: do not reintroduce a `~/.claude-sync/` fallback or hard-code that path anywhere (CLI, plugin scripts, tests). Tests set the env vars with `t.Setenv` into `t.TempDir()`.
- **Path-based session indexing**: Claude Code keys session dirs by absolute filesystem path. `PathMapper` only makes sessions portable for paths under `$HOME` or a `path_map` prefix; projects elsewhere still need the same absolute path on every device.
- **Storage adapter imports**: anything outside `cmd/claude-sync/` that calls `storage.New(...)` must also blank-import the adapter packages it needs (see `internal/sync/sync.go` top). Forgetting this produces a runtime "unsupported storage provider" error, not a compile error.
- **Symlinks are skipped** by `GetLocalFiles` — don't rely on symlinked content inside `~/.claude/` being synced.
- **The Argon2 salt is intentionally fixed** in `crypto.GenerateKeyFromPassphrase`. Do not "fix" it to per-user random — doing so breaks cross-device sync (the whole point of passphrase mode).
- **Do not add destructive operations** to the default code path without an explicit `--force` or interactive confirm — the CLI is careful about backups (`~/.claude.backup.<ts>`), key-mismatch detection, and `.conflict.<ts>` files for a reason.
