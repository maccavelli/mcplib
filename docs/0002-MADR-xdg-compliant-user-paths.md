---
status: "accepted"
date: 2026-08-23
decision-makers: maccavelli
consulted: maccavelli
informed: maccavelli
---

# 0002-MADR: Resolve user paths through a shared mcplib package using platform-native locations with XDG overrides

## Context and Problem Statement

Every compiled binary in the fleet decides for itself where the user's configuration, data, and
cache live. There is no shared helper, and the results are inconsistent — including one case
where a database is written into the configuration directory.

Two distinct problems, one of which is a live defect.

### The database is stored in the configuration directory

[`mcp-server-recall/internal/config/config.go:111`](../../mcp-server-recall/internal/config/config.go)
computes the default database location by joining the *config* directory:

```go
v.SetDefault("dbPath", filepath.Join(appConfigDir, DefaultDBName))
```

where `appConfigDir` derives from `os.UserConfigDir()` (`config.go:96`) and
`DefaultDBName = ".mcp_recall"` (`config.go:24`). On the operator's macOS host this resolves to
`~/Library/Application Support/mcp-server-recall/.mcp_recall`, and the observed store occupies
roughly 128 MB.

A database is mutable application state, not configuration. Under the XDG Base Directory
Specification it belongs in `$XDG_DATA_HOME` (default `~/.local/share`), not
`$XDG_CONFIG_HOME`. On Linux the current code therefore writes a growing binary datastore into
`~/.config`, a directory users reasonably expect to hold small, editable, back-up-able text
files.

The cause is a gap in the standard library: Go provides `os.UserConfigDir()` and
`os.UserCacheDir()` but **no `os.UserDataDir()`**. With no obvious place to put data, it went
next to the config.

### Path logic is duplicated across eleven repositories

`os.UserConfigDir`, `os.UserCacheDir`, and `os.UserHomeDir` are called directly from
non-test code in eleven repositories — 94 call sites in roughly 43 files:

| Repository | Files | Call sites |
|---|---|---|
| `mcp-server-magicdev` | 11 | 33 |
| `mcp-server-magictools` | 7 | 19 |
| `mcp-server-recall` | 5 | 9 |
| `mcp-server-brainstorm` | 4 | 7 |
| `mcp-server-magicskills` | 2 | 6 |
| `mcp-server-go-modernizer` | 3 | 5 |
| `mcp-server-duckduckgo` | 2 | 4 |
| `mcp-server-evolve-plan` | 4 | 4 |
| `mcp-server-filesystem` | 2 | 3 |
| `prepare-commit-msg` | 1 | 3 |
| `mcp-server-socratic-thinker` | 1 | 1 |

`mcplib` — the shared library all of these already depend on — offers nothing for this; its only
path-related call is a single `os.UserHomeDir()` in
[`fastpath/fastpath.go:69`](../fastpath/fastpath.go).

Duplicated resolution guarantees divergence. The fleet has already been bitten by exactly this
shape of problem: `scripts/bump-libs.sh` hardcoded a module path prefix, silently matched
nothing after that path changed, and five servers drifted for weeks unnoticed. Eleven
independent copies of path logic will drift the same way.

### What the standard library does and does not give us

`os.UserConfigDir()` and `os.UserCacheDir()` are already correct on Linux — they honour
`XDG_CONFIG_HOME` and `XDG_CACHE_HOME`, falling back to `~/.config` and `~/.cache`. The gaps
are:

* **No data directory** on any platform.
* **No state directory** (`XDG_STATE_HOME`, `~/.local/state`).
* **`XDG_*` variables are ignored on macOS.** Go returns `~/Library/Application Support`
  unconditionally, with no way to override. An operator who deliberately sets `XDG_CONFIG_HOME`
  on a Mac — for a portable dotfiles setup, or to relocate state onto another volume — is
  ignored.

## Decision Drivers

* A database written into `~/.config` is wrong on Linux and will surprise anyone who backs up
  or version-controls that directory.
* Eleven copies of the same logic will diverge; only a shared implementation prevents recurrence.
* macOS users expect `~/Library`, not `~/.config`; Linux users expect XDG. Imposing either
  convention on the other platform is worse than respecting each.
* An operator who explicitly sets an `XDG_*` variable has stated an intent that should be
  honoured on every platform, including macOS.
* `mcplib` is consumed by twelve binaries; a dependency added there propagates everywhere, so
  new dependencies must earn their place.
* Existing installations must not silently lose access to their data.

## Considered Options

* **A `paths` package in `mcplib`, implemented in-repo** — platform-native defaults, `XDG_*`
  honoured on all platforms, exposing config, data, cache, and state resolvers.
* **Adopt `github.com/adrg/xdg` in `mcplib`** — a maintained third-party implementation of the
  same idea.
* **Fix the database location in `mcp-server-recall` only** — leave the other ten repositories
  and the duplication alone.
* **A single fleet root, e.g. `$MCP_HOME`** — one directory tree for every server, ignoring
  platform conventions entirely.

## Decision Outcome

Chosen option: **"A `paths` package in `mcplib`, implemented in-repo"**, resolving
platform-native locations by default and honouring `XDG_*` environment variables on all
platforms when explicitly set.

The resulting mapping:

| Resolver | Linux | macOS | Windows |
|---|---|---|---|
| `ConfigDir` | `$XDG_CONFIG_HOME` → `~/.config` | `$XDG_CONFIG_HOME` if set → `~/Library/Application Support` | `%AppData%` |
| `DataDir` | `$XDG_DATA_HOME` → `~/.local/share` | `$XDG_DATA_HOME` if set → `~/Library/Application Support` | `%LocalAppData%` |
| `CacheDir` | `$XDG_CACHE_HOME` → `~/.cache` | `$XDG_CACHE_HOME` if set → `~/Library/Caches` | `%LocalAppData%\<app>\cache` |
| `StateDir` | `$XDG_STATE_HOME` → `~/.local/state` | `$XDG_STATE_HOME` if set → `~/Library/Application Support` | `%LocalAppData%` |

Implementing in-repo rather than taking `adrg/xdg` is a deliberate trade. The logic is small and
fully specified by the table above, `mcplib` currently has no dependency of this kind, and the
macOS `XDG_*` override is a behaviour we want to guarantee precisely rather than inherit. Twelve
binaries consume `mcplib`, so a dependency here is a fleet-wide commitment; this one is not worth
making for approximately 150 lines of code that we can test exhaustively ourselves.

A significant practical consequence of choosing platform-native over uniform-XDG: on macOS,
`DataDir` resolves to `~/Library/Application Support` — **the same directory recall already
uses**. The database therefore does not move on the operator's host, and no migration is
required there. The relocation is real only on Linux, where the database moves out of
`~/.config` and into `~/.local/share`.

### Consequences

* Good, because the database lands in a data directory on every platform, which is what both
  XDG and Apple's conventions call for.
* Good, because one implementation replaces eleven, so the next path change is a single edit
  plus a version bump rather than an eleven-repository sweep.
* Good, because `XDG_*` overrides work on macOS, which the standard library does not permit.
* Good, because on macOS the resolved paths are unchanged, so this host needs no migration and
  the change can be verified without moving anything.
* Neutral, because `mcplib` gains a package but no new external dependency.
* Bad, because adopting it fleet-wide touches eleven repositories and 94 call sites, requiring an
  `mcplib` release and a dependency bump everywhere before any repository can adopt it.
* Bad, because Linux installations that already hold a database under `~/.config` need migration
  or explicit `dbpath` configuration. No such installation is known to exist, but the fleet's
  `servers.yaml` carries Linux paths, so the possibility is real.
* Bad, because `os.UserHomeDir()` remains legitimate for genuinely home-relative paths, so the
  package cannot mechanically replace every call site; each of the 94 needs classifying.

### Confirmation

1. `mcplib/paths` has table-driven tests covering all four resolvers on all three platforms,
   with `XDG_*` set and unset, asserting the mapping table above.
2. A test asserts that on macOS an explicitly set `XDG_CONFIG_HOME` is honoured — the behaviour
   the standard library lacks.
3. `mcp-server-recall`'s default `dbPath` resolves under `DataDir`, not `ConfigDir`, asserted by
   test.
4. On the operator's macOS host, the resolved database path after the change is byte-identical
   to the current one, confirming no migration is needed there.
5. `grep -rn 'os\.UserConfigDir\|os\.UserCacheDir' --include='*.go'` returns no non-test hits in
   any adopting repository, except where deliberately documented.
6. All eleven repositories build and their suites pass against the new `mcplib`.

## Pros and Cons of the Options

### A `paths` package in `mcplib`, implemented in-repo

* Good, because it adds no dependency to a library twelve binaries consume.
* Good, because the macOS `XDG_*` override is guaranteed by our own tests rather than by an
  upstream project's choices.
* Good, because it fills the `UserDataDir` and `StateDir` gaps the standard library leaves.
* Neutral, because roughly 150 lines must be written and maintained in-house.
* Bad, because we own the platform edge cases — unset `HOME`, Windows variable expansion — that
  a mature library has already encountered.

### Adopt `github.com/adrg/xdg` in `mcplib`

* Good, because it is maintained, widely used, and already handles the platform edge cases.
* Good, because it defaults `XDG_DATA_HOME` to `~/Library/Application Support` on macOS, closely
  matching the chosen mapping.
* Bad, because it introduces a transitive dependency into every binary in the fleet for a small
  amount of logic.
* Bad, because its macOS override semantics are its choice, not ours, and could change across
  releases in a way that silently relocates user data.

### Fix the database location in `mcp-server-recall` only

* Good, because it is by far the smallest change and resolves the one confirmed defect.
* Good, because it needs no `mcplib` release and no dependency bumps.
* Bad, because it leaves 85 call sites in ten repositories unchanged and the duplication intact.
* Bad, because it guarantees recurrence, which is the failure mode the fleet has already
  experienced with `bump-libs.sh`.

### A single fleet root, e.g. `$MCP_HOME`

* Good, because it is trivially predictable and makes backup and relocation a single operation.
* Good, because it removes all platform branching.
* Bad, because it violates platform conventions on all three operating systems.
* Bad, because it is the opposite of the XDG compliance being sought.
* Neutral, because it could be layered on later as an optional override without conflicting with
  this decision.

## More Information

Implementation steps, the call-site classification, and rollback are in
[0002-PLAN-xdg-compliant-user-paths.md](0002-PLAN-xdg-compliant-user-paths.md).

### Relationship to other records

This record concerns *where* files are resolved. It is independent of
[`mcp-server-recall/docs/0001-MADR-encryptionkey-yaml-tag-round-trip.md`](../../mcp-server-recall/docs/0001-MADR-encryptionkey-yaml-tag-round-trip.md),
which concerns *how* the encryption key is serialized. The two touch neighbouring code in
`internal/config` and `cmd/mcp-server-recall`, so sequencing matters; the plan addresses it.

The operator has confirmed the existing recall database is empty, so the data-loss risk that
would normally attend relocating a datastore does not apply to this host.

### Open questions

* **Whether any Linux installation exists.** The relocation from `~/.config` to
  `~/.local/share` only affects Linux. The fleet's `servers.yaml` contains Linux paths, but they
  are placeholders from a template rather than evidence of a live Linux deployment. If none
  exists, the migration step is precautionary only.
* **Classification of the 94 call sites.** Some `os.UserHomeDir()` uses are legitimately
  home-relative — `mcp-server-magictools` alone has 11 — and must not be mechanically rewritten.
  Each site needs review; the plan treats this as per-repository work, not a sweep.
