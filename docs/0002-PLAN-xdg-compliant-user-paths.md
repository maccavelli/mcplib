# Implement Resolve user paths through a shared mcplib package using platform-native locations with XDG overrides

Associated MADR: [0002-MADR-xdg-compliant-user-paths.md](0002-MADR-xdg-compliant-user-paths.md) (status: accepted)

## Goal

One path resolver for the fleet. Every binary obtains config, data, cache, and state directories
from `mcplib/paths`, which returns platform-native locations by default and honours `XDG_*`
overrides on all platforms — including macOS, where the standard library ignores them.

Done means all seven acceptance criteria in [Verification](#verification) hold.

## Scope

New package `mcplib/paths`, then adoption across eleven repositories. Out of scope:
`github.com/adrg/xdg` (rejected in the MADR), an `$MCP_HOME` single root, the encryption-key
serialization ([recall MADR 0001](../../mcp-server-recall/docs/0001-MADR-encryptionkey-yaml-tag-round-trip.md)),
and rewriting `os.UserHomeDir()` calls that are legitimately home-relative.

## Preconditions

```bash
cd ~/gitrepos/go/mcplib && git status --porcelain -uno   # empty
go test ./... && golangci-lint run -c .golangci.yml ./... # green
```

Two facts from this tree that shape the work:

* `mcplib` has no path helper today; its only related call is `os.UserHomeDir()` at
  `fastpath/fastpath.go:69`. Existing packages: `fastpath`, `hfsc`, `llmprovider`, `logging`,
  `schema`.
* **`mcp-server-recall` places the database under the config directory in three distinct
  sites**, not one:
  * `internal/config/config.go:111` — `v.SetDefault("dbPath", filepath.Join(appConfigDir, DefaultDBName))`
  * `cmd/mcp-server-recall/configure.go:210-211` — `dbDir := filepath.Join(dirPath, config.DefaultDBName); os.MkdirAll(dbDir, 0700)` in `ensureInitialized`
  * `cmd/mcp-server-recall/configure.go:75` — `dbDir := filepath.Join(configDirPath(), config.DefaultDBName)`, the pre-flight check for existing database contents

  Fixing only the default would leave `configure` still creating `.mcp_recall` in the config
  directory, and the pre-flight guard still inspecting it. All three must change together.

## Implementation Steps

### Step 1 — Build `mcplib/paths`

Create `paths/paths.go` with four resolvers, each returning an application-scoped directory:

```go
func ConfigDir(app string) (string, error)
func DataDir(app string)   (string, error)
func CacheDir(app string)  (string, error)
func StateDir(app string)  (string, error)
```

Resolution order, applied identically by each resolver:

1. If the matching `XDG_*` variable is set **and absolute**, use it — on every platform,
   including macOS. A relative value is ignored, per the XDG specification.
2. Otherwise use the platform default from the MADR mapping table.
3. `filepath.Join(base, app)` and return. **Do not create the directory**; creation stays the
   caller's decision, because callers differ on permissions (`0700` in recall) and on whether
   absence is an error.

Structure:

* `paths.go` — the exported API, the `XDG_*` precedence logic, and a `baseDirs()` seam.
* `paths_darwin.go`, `paths_linux.go`, `paths_windows.go` — platform defaults behind build tags.

Keep `runtime.GOOS` out of `paths.go` entirely; branching by build tag makes each platform's
behaviour independently readable and independently testable. Do **not** call `os.UserConfigDir()`
and then override it — resolve directly, so precedence is explicit rather than emergent.

Return a typed error (`ErrNoHome`) when `HOME` is unset and no override applies. Callers today
degrade to the working directory (`recall config.go:97-99`) and must retain that option.

### Step 2 — Test the package

Table-driven, `t.Setenv` for isolation. Assert the MADR mapping exactly:

* Each resolver × platform default, `XDG_*` unset.
* Each resolver with `XDG_*` set absolute → override wins, **with an explicit macOS case**. This
  is the behaviour the standard library lacks and the reason the package exists; if only Linux is
  covered, the package's whole justification is untested.
* `XDG_*` set to a relative path → ignored, default used.
* `HOME` unset → `ErrNoHome`, not a path with an empty segment.
* `DataDir != ConfigDir` on Linux, `DataDir == ConfigDir` on macOS. That divergence is the point
  of the change and must fail loudly if it regresses.

Platform expectations run under their own build tags; never assert Linux paths from a macOS run.

### Step 3 — Release mcplib

```bash
cd ~/gitrepos/go/mcplib
go test ./... && golangci-lint run -c .golangci.yml ./...
GOOS=linux go build ./... && GOOS=windows go build ./... && GOOS=darwin go build ./...
git tag -a v1.1.0 -m "v1.1.0" && git push origin main && git push origin v1.1.0
```

Minor bump — this adds a package. The tag **must** reach GitHub before any consumer bumps:
`go get` resolves through the module proxy, so a local-only tag cannot be fetched. Confirm with
`curl -s https://proxy.golang.org/github.com/maccavelli/mcplib/@v/list`.

### Step 4 — Convert `mcp-server-recall`

Recall holds the confirmed defect and the clearest acceptance test, so it proves the package
first.

```bash
cd ~/gitrepos/go/mcp-server-recall
go get github.com/maccavelli/mcplib@v1.1.0 && go mod tidy
```

| File:line | Current | Change to |
|---|---|---|
| `internal/config/config.go:96` | `os.UserConfigDir()` | `paths.ConfigDir(Name)` — returns the app-scoped dir, so drop the `filepath.Join(configDir, Name)` at line 100 |
| `internal/config/config.go:111` | `filepath.Join(appConfigDir, DefaultDBName)` | `filepath.Join(dataDir, DefaultDBName)` where `dataDir, err := paths.DataDir(Name)` |
| `cmd/mcp-server-recall/configure.go:210` | `filepath.Join(dirPath, config.DefaultDBName)` | data dir, matching the new default |
| `cmd/mcp-server-recall/configure.go:75` | `filepath.Join(configDirPath(), config.DefaultDBName)` | data dir — otherwise the "existing contents" guard inspects the wrong directory |
| `cmd/mcp-server-recall/paths.go:18` | `os.UserConfigDir()` | `paths.ConfigDir(config.Name)` |
| `cmd/mcp-server-recall/main.go:26` | `os.UserCacheDir()` | `paths.CacheDir(config.Name)` |
| `cmd/mcp-server-recall/config_template.go:12-15` | comments stating the DB lives beside the config | rewrite for the data dir per platform |

Preserve the fallback at `config.go:97-99`: on `ErrNoHome`, warn and fall back to `"."` as today.

**Test-scaffolding interaction — this will break a passing test if unhandled.**
`cmd/mcp-server-recall/paths_test.go:16-26` sandboxes by setting `HOME` *and* `XDG_CONFIG_HOME`
to a `t.TempDir()`, then returns `os.UserConfigDir()`:

```go
t.Setenv("HOME", tempHome)
t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))
base, err := os.UserConfigDir()      // macOS: IGNORES XDG_CONFIG_HOME
```

On macOS that returns `<tmp>/Library/Application Support`, because the standard library ignores
`XDG_CONFIG_HOME`. After Step 1, `paths.ConfigDir` **honours** it and returns `<tmp>/.config`.
The helper's returned base and the code's resolved path then diverge, and every test built on
`sandboxConfigDir` — including `TestConfigureCommand_Sandboxed` and the new round-trip tests from
recall PLAN 0001 — fails with a path mismatch.

Fix the helper to resolve through the same code path it is sandboxing:

```go
base, err := paths.ConfigDir(config.Name)   // not os.UserConfigDir()
```

and return the parent if callers expect an unscoped base. Treat any other test needing a change
as a signal the conversion is wrong.

### Step 5 — Convert the remaining ten repositories

Ascending by call-site count, so the pattern is established on small repositories first. Each
row is a complete, verified inventory of **non-test** sites at the current HEAD:

| # | Repository | Sites | Files:lines |
|---|---|---|---|
| 1 | `mcp-server-socratic-thinker` | 1 | `internal/metrics/store.go:55` (Home) |
| 2 | `prepare-commit-msg` | 3 | `internal/config/config.go:53` (Home), `:54` (Config) |
| 3 | `mcp-server-filesystem` | 3 | `internal/pathutil/pathutil.go:157` (Home), `internal/config/config.go:38` (Config) |
| 4 | `mcp-server-duckduckgo` | 4 | `cmd/.../serve.go:32`, `:92`, `internal/config/config.go:38` (all Cache) |
| 5 | `mcp-server-evolve-plan` | 4 | `cmd/.../init.go:19` (Config), `cmd/.../serve.go:36` (Cache), `internal/config/config.go:51` (Config), `internal/scrubber/ambiguity.go:45` (Config) |
| 6 | `mcp-server-go-modernizer` | 5 | `cmd/.../configure.go:19` (Config), `:42` (Cache), `internal/util/tool.go:82` (Home), `:91` (Cache), `internal/config/config.go:246` (Config) |
| 7 | `mcp-server-magicskills` | 6 | `internal/config/config.go:43` (Config), `:88` (Home), `:140` (Cache), `internal/lifecycle/lifecycle.go:36` (Cache) |
| 8 | `mcp-server-brainstorm` | 7 | `cmd/.../serve.go:33` (Cache), `internal/handler/analytics/generate_report_helpers.go:327`, `:335`, `:341` (Home), `internal/persistent/db.go:20` (Cache), `internal/engine/standards.go:81` (Cache) |
| 9 | `mcp-server-magictools` | 19 | 7 files — enumerate at conversion time |
| 10 | `mcp-server-magicdev` | 33 | 11 files — enumerate at conversion time |

Per repository:

1. `go get github.com/maccavelli/mcplib@v1.1.0 && go mod tidy`
2. Re-enumerate — line numbers above are valid at the current HEAD and will drift:
   `grep -rnE 'os\.User(Config|Cache|Home)Dir' --include='*.go' . | grep -v _test`
3. **Classify before editing.** `UserConfigDir` → `paths.ConfigDir`, `UserCacheDir` →
   `paths.CacheDir` convert mechanically. `UserHomeDir` does **not**: several are legitimately
   home-relative and must stay. Confirmed instances needing review rather than rewriting:
   * `mcp-server-brainstorm/internal/handler/analytics/generate_report_helpers.go:327,335,341` —
     three consecutive Home calls in report-path handling.
   * `mcp-server-magictools` — 11 Home calls, the largest concentration in the fleet.
   * `mcp-server-filesystem/internal/pathutil/pathutil.go:157` — a sandboxing path resolver,
     where changing the root would alter the security boundary.

   Rule of thumb: if the path represents *the user's own files*, keep `UserHomeDir`. If it
   represents *this application's files*, convert.
4. Note where a datastore currently sits under a config or cache dir; those are `DataDir`
   candidates, not mechanical `ConfigDir` swaps. `mcp-server-brainstorm/internal/persistent/db.go:20`
   uses `UserCacheDir` for a database — a cache directory is eligible for OS eviction, so this
   is the same class of defect as recall's and should move to `DataDir`.
5. `go build ./... && go test ./...`, run the pre-commit gate on changed files, commit.

One commit per repository. Do not batch — per-repository commits keep bisection and rollback
per-repository.

## Verification

```bash
# Steps 1-2
cd ~/gitrepos/go/mcplib
go test ./paths/... -v
GOOS=linux go build ./paths/... && GOOS=windows go build ./paths/... && GOOS=darwin go build ./paths/...
golangci-lint run -c .golangci.yml ./...

# Steps 4-5, per consumer
go build ./... && go test ./...
grep -rnE 'os\.User(Config|Cache)Dir' --include='*.go' . | grep -v _test    # expect none
gofmt -l <changed>; golint <changed>; golangci-lint run -c .golangci.yml ./...
```

The operative host check — on macOS the resolved paths must **not** move:

```bash
# before conversion
ls -d "$HOME/Library/Application Support/mcp-server-recall/.mcp_recall"
# after conversion, must be the identical path
./dist/mcp-server-recall-darwin-arm64 serve --help >/dev/null
ls -d "$HOME/Library/Application Support/mcp-server-recall/.mcp_recall"
```

Acceptance criteria:

1. `mcplib/paths` tests pass, **including** the macOS `XDG_CONFIG_HOME` override case.
2. Cross-compilation succeeds for linux, darwin, windows.
3. Recall's default `dbPath` resolves under `DataDir`, asserted by test — and `configure` creates
   `.mcp_recall` in that same directory, not the config directory.
4. On this macOS host the resolved database path is byte-identical to the pre-change path.
5. No non-test `os.UserConfigDir` / `os.UserCacheDir` remains in any adopting repository.
6. All eleven repositories build with green suites against `mcplib v1.1.0`.
7. All four enabled sub-servers reconnect after the launchd restart.

## Rollout and Rollback

**Rollout.** Strictly ordered: package → release → recall → the other ten, smallest first. The
binaries are leaf executables — no repository imports another `mcp-server-*` module — so
consumers convert at any pace with no coordinated cutover. Nothing is pushed or tagged without
explicit approval.

**Rollback.** Per repository: revert the commit and `go get github.com/maccavelli/mcplib@v1.0.1`.
Because macOS paths are unchanged, rollback on this host moves no files and loses no data.

`mcplib v1.1.0` is purely additive and may remain published after a consumer rollback.

**Residual risk — Linux only.** The default database path moves from
`~/.config/mcp-server-recall/.mcp_recall` to `~/.local/share/mcp-server-recall/.mcp_recall`. A
Linux installation upgrading across this change would appear to start with an empty database, and
rolling back after data had been written to the new location would strand it. Recovery is a
directory move, but manual. No Linux installation is known to exist — the Linux paths in
`servers.yaml` were template placeholders — so this is precautionary. Document it in the recall
README: move the directory, or set `dbpath` explicitly.

## Sequencing against recall MADR 0001

Both records edit `cmd/mcp-server-recall/config_template.go` and `internal/config/config.go`;
this plan additionally edits `configure.go:75` and `configure.go:210-211`, which recall PLAN 0001 leaves alone.

Land **recall PLAN 0001 first** — it is self-contained, needs no `mcplib` release, and its
round-trip tests pin key behaviour before path resolution moves underneath it. Then rebase this
plan's Step 4 onto the result, at which point the `sandboxConfigDir` fix above also repairs
0001's new tests. Do not run both in the same working tree concurrently.
