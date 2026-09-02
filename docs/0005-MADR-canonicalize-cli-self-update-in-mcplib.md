---
status: accepted
date: 2026-09-01
decision-makers: mcplib maintainers
consulted: magic-cli-remote, mcp-server-recall, prepare-commit-msg, mcp-server-magictools
informed: all mcplib consumers
---

# Canonicalize CLI Self-Update in `mcplib` Behind Policy and Lifecycle Seams

<!-- markdownlint-disable MD013 MD024 -->

## Context and Problem Statement

The four assessed repositories need one dependable CLI update capability, but they currently
have two independent implementations and two missing implementations:

| Repository | Current CLI update surface | Production implementation | Release assets |
|---|---|---:|---|
| `magic-cli-remote` | `mcremote update` and `mcrelay update`; `--check`, `--yes`, `--force`; check-available exits 10 | 992 lines across `internal/update`, `internal/cli/update.go`, and `internal/relay/update.go` | Version-suffixed binaries plus unversioned aliases; versioned and unversioned checksum manifests |
| `prepare-commit-msg` | `prepare-commit-msg update`; `--check`, `--force`, `--version`, `--yes`/`-y` | 831 lines across `internal/selfupdate` | Exact unversioned platform binary names plus `SHA256SUMS` |
| `mcp-server-recall` | No update command | None | Build output uses exact unversioned names; publication adds version-suffixed copies and a second manifest |
| `mcp-server-magictools` | No update command | None | Build output uses exact unversioned names; publication adds version-suffixed copies and a second manifest |

The two existing implementations therefore contain 1,823 production lines before their test
suites, yet the next two products would still start from nothing. Thirteen sibling modules
already import `github.com/maccavelli/mcplib`; it is the natural ownership boundary for the
portable update mechanism.

The hard part is not moving files into a shared directory. The implementations disagree about
version identity, release asset naming, confirmation, check-mode exit status, target-version
selection, service restart, rollback, token lookup, and output routing. Some of those differences
are product requirements, while others are drift or defects. A shared package that merely
generalizes every difference would canonicalize code without canonicalizing behavior.

This record decides both:

* the common CLI and release contract that new adopters follow;
* the renderer-, CLI-framework-, source-, and service-lifecycle seams that remain configurable;
* which existing behavior becomes the shared baseline;
* which legacy behavior is migrated instead of becoming permanent public API.

The requested compatibility floor is Go 1.26.5. `mcplib`, `magic-cli-remote`,
`mcp-server-recall`, and `mcp-server-magictools` declare `go 1.26.5`.
`prepare-commit-msg` currently declares `go 1.26.6`; a library that requires 1.26.5 remains
consumable there because a module's `go` directive is its minimum required toolchain version.
No Go 1.27 API is required by this decision.

### Scope

In scope:

* user-invoked CLI checks, upgrades, exact-version installs, reinstalls, and explicit rollbacks;
* GitHub Releases discovery and asset download;
* strict release-version and asset selection;
* bounded download, integrity verification, staging, replacement, backup, and rollback;
* optional managed-service stop/reconcile/start/health behavior;
* consistent result, error, confirmation, output, and exit-code semantics;
* release-workflow changes needed to make those semantics common;
* migration of both existing updater implementations and addition to Recall and MagicTools.

Out of scope:

* background or automatic update polling;
* Android application update behavior in `magic-cli-remote`;
* updating source checkouts, Go module dependencies, configuration files, or databases;
* silently taking ownership of Homebrew, Nix, Scoop, Chocolatey, system-package, or other
  package-manager installations;
* a complete TUF repository or a mandatory Sigstore verifier in the first shared release;
* implementation work before a separate approved implementation plan exists.

### Investigation Method

The findings below are grounded in the working trees as inspected on 2026-09-01, targeted
existing tests, repository history, the Go 1.26.5 standard-library documentation, and primary
external specifications. The following read-only test runs passed:

```text
magic-cli-remote:     go test ./internal/update ./internal/cli ./internal/relay
prepare-commit-msg:   go test ./internal/selfupdate .
mcp-server-recall:    go test ./cmd/mcp-server-recall
mcp-server-magictools: go test ./cmd/mcp-server-magictools
```

Passing tests establish the current behavior; they do not resolve the cross-repository contract
differences or the uncovered cases recorded below.

### Existing Implementation Findings

#### F1 — Magic Remote has the stronger transactional service behavior, but it is product-coupled

`magic-cli-remote/internal/update/run.go:48-168` performs the complete sequence:
release lookup, platform asset selection, version comparison, optional confirmation, checksum
lookup, download, verification, executable-path resolution, service-state probe, swap, service
definition refresh, restart, and result output.

Its service behavior contains important production knowledge:

* it distinguishes a service definition being installed from its process currently being active;
* an installed-but-down service is healed after update;
* a product with no service definition is a binary-only update;
* the new binary renders the new service definition after swap;
* a failed post-swap start restores the previous executable and attempts to restore service state;
* `mcremote` and `mcrelay` are independently keyed by product.

That behavior was not correct on its first implementation. The history of `internal/update`
contains subsequent fixes for published `BASE.N` parsing, installed-service detection, Windows
asset naming, and rollback restart failure reporting. The repeated fixes demonstrate why this
logic should be tested once, but also why the shared library must model lifecycle explicitly
instead of treating every executable as a daemon.

The portable core must not import Magic Remote's service templates or command packages.
`prepare-commit-msg` has no daemon, Recall normally runs over MCP stdio, and MagicTools owns a
different service command surface. Service handling is a composition around executable apply,
not a required stage of every update.

#### F2 — `prepare-commit-msg` exposes a richer flag set but its CLI contract is internally inconsistent

`prepare-commit-msg/main.go:109-137` parses `--check`, `--force`, `--version`, and
`--yes`/`-y`, then calls `selfupdate.Run`. The value of `yes` is never read or passed. The updater
has no confirmation step, so a plain `prepare-commit-msg update` replaces the executable
non-interactively even though the help describes `--yes` as the non-interactive mode.

`internal/selfupdate/updater.go:162-173` reports an available update in check mode but returns a
nil error. Consequently both "current" and "update available" exit 0. Magic Remote instead
returns a typed `ErrUpdateAvailable`, and both main packages map it to exit 10. The latter is the
more useful automation contract and is already documented and tested in that project.

Exact target versions are useful and should be retained. However, current target behavior needs
clearer semantics:

* an older `--version` is installed without `--force`, making an exact target an implicit rollback;
* `--check --version <older>` prints "up to date" rather than reporting that the requested target
  differs and is older;
* `--force` overlaps both same-version reinstall and generic overwrite;
* `--yes` suggests a prompt that does not exist.

Magic Remote has the inverse gaps: it confirms by default and has scriptable check status, but it
cannot select an exact release and builds its operation context from `context.Background()`
rather than `cmd.Context()` (`internal/cli/update.go:34-57` and
`internal/relay/update.go:23-46`). Cancellation from the CLI is therefore not propagated.

#### F3 — the version models should not both become shared public API

`prepare-commit-msg/internal/selfupdate/semver.go` claims SemVer 2.0.0 behavior but implements a
partial parser. It accepts one- and two-component versions, does not reject leading zeroes, does
not validate empty or illegal prerelease/build identifiers, discards build metadata, and orders
arbitrary invalid strings lexically. Those choices are tolerant, but they are not SemVer 2.0.0.

The [SemVer 2.0.0 specification](https://semver.org/) requires exactly `MAJOR.MINOR.PATCH`,
forbids leading zeroes in numeric fields and numeric prerelease identifiers, defines allowed
identifier characters, and excludes build metadata from precedence. The Go project already
ships `golang.org/x/mod/semver`, which implements precedence and validation with documented Go
module extensions. A small strict wrapper can require the full `vMAJOR.MINOR.PATCH` release-tag
shape already enforced by the three single-binary repositories' release scripts.

Magic Remote is intentionally not using SemVer for the running binary. Its GitHub release tag is
`vMAJOR.MINOR.PATCH`, but each release run stamps and publishes a separate
`MAJOR.MINOR.PATCH.N` build serial. `docs/0103-MADR-update-tracks-release-build-and-active-service.md`
records how treating the fourth field incorrectly broke ordinary updates and then required a
dedicated parser and comparison rule. A four-component version is not SemVer. Rebuilding and
replacing assets under the same release tag also conflicts with SemVer's requirement that a
released version's contents not be modified.

The common contract should therefore be strict SemVer, not an interface that preserves
indefinite `BASE.N` publication. Magic Remote's next update-bearing release will be the migration
boundary:

* the immutable GitHub tag is the release identity;
* release binaries are stamped from that tag, with optional build information kept outside
  precedence;
* a new fix receives a new patch tag instead of a new `N` under an old tag;
* a narrow consumer-side legacy normalizer recognizes already-installed `BASE.N` binaries long
  enough to compare their three-part base with the first canonical release;
* the legacy normalizer is not exported from `mcplib` as a general version scheme.

This removes an avoidable permanent policy seam and lets every future consumer share one
standards-based comparator.

#### F4 — three repositories already build one asset convention, but two inflate it during publication

Recall, MagicTools, and `prepare-commit-msg` build raw executables named:

```text
<product>-<goos>-<goarch>[.exe]
SHA256SUMS
```

Their tag pipelines require strict `vMAJOR.MINOR.PATCH` releases. Recall additionally ships
Linux ARM64; MagicTools currently does not. `prepare-commit-msg` ships six Linux/macOS/Windows
AMD64/ARM64 combinations. Platform support is product data and must remain configurable, but the
name template does not need one code path per matrix. `prepare-commit-msg` publishes that build
set unchanged. Recall and MagicTools currently copy those binaries to version-suffixed names,
generate a second version-suffixed manifest, then publish both forms with
`gh release upload --clobber` (`mcp-server-recall/.github/workflows/ci.yml:307-350` and
`mcp-server-magictools/.github/workflows/ci.yml:305-342`). Those publication-only copies are
drift to remove, not another asset policy to preserve.

Magic Remote already publishes equivalent unversioned aliases for its installer. Its updater,
however, selects a version-suffixed asset by prefix
(`internal/update/github.go:81-111`) and finds a checksum manifest through a permissive fallback
(`github.go:114-128`). This can match more than the intended executable or an unrelated
`SHA256SUMS*` asset; multiple executable matches fail closed, but checksum fallback takes the
first matching prefix. The asset-derived version is also not checked for consistency with the
release tag's base.

The canonical contract is an exact asset name derived from product, `GOOS`, `GOARCH`, and the
Windows extension, with one exact `SHA256SUMS`. A GitHub release already supplies the version
namespace, so embedding the version in each asset name adds parsing and alias machinery without
adding identity.

#### F5 — both HTTP clients work, but neither is yet the reusable hardened client

Positive behavior already present across the two implementations includes contexts, finite HTTP
timeouts, required `User-Agent` headers, optional GitHub tokens, limited error-body reads,
streaming binary hashing in `prepare-commit-msg`, and injected clients or URLs in tests.

The reusable implementation still needs the following corrections:

* Magic Remote constructs separate `http.Client` values for metadata, manifest, and binary
  requests. Go documents clients and transports as safe for concurrent reuse and recommends
  reusing them.
* Neither client sends an explicit `X-GitHub-Api-Version`. GitHub's REST guidance says callers
  should pin one; the current documented version is `2026-03-10`.
* Magic Remote recognizes only `GITHUB_TOKEN`; `prepare-commit-msg` recognizes
  `GITHUB_TOKEN` and `GH_TOKEN`.
* Both implementations attach the token through a generic request helper to URLs supplied by
  release metadata. The standard `http.Client` protects sensitive headers across unrelated
  redirect domains, but the initial browser-download request should still not send a GitHub API
  token to an arbitrary metadata-provided origin. The official release-assets API supports
  authenticated downloads through the asset API URL with `Accept: application/octet-stream` and
  a 200-or-302 response.
* Metadata and checksum response bounds are inconsistent. Magic Remote truncates one in-memory
  read without reporting overflow; `prepare-commit-msg` scans the checksum body with Scanner's
  implicit token limit and streams the executable without an explicit maximum.
* Neither implementation enforces the API-declared asset size or captures the release asset's
  native `digest` field, now exposed as `sha256:<hex>` by GitHub.
* Rate-limit errors are reduced to generic HTTP status failures instead of retaining
  `Retry-After`, `X-RateLimit-Remaining`, and `X-RateLimit-Reset` guidance.

The [GitHub Releases API](https://docs.github.com/en/rest/releases/releases) returns asset IDs,
sizes, states, download URLs, and SHA-256 digests. The
[release-assets endpoint](https://docs.github.com/en/rest/releases/assets) is the canonical API
download path. The shared source implementation should model and validate those fields rather
than retain the smallest common JSON subset.

#### F6 — checksum verification is necessary but is not release authenticity

Both implementations download `SHA256SUMS` from the same GitHub release as the executable. This
detects corruption and a mismatch between two release assets. It does not independently prove
that the release came from the maintainer if the repository or release publisher is compromised.
Magic Remote's own MADR 0065 already records this limitation.

The shared package must describe this honestly and expose a verifier chain rather than bake in a
claim of cryptographic provenance. The first supported policy requires:

* HTTPS GitHub API release discovery;
* an uploaded, non-draft, non-prerelease release; the initial canonical CLI accepts stable
  complete tags only and rejects prerelease targets;
* GitHub release immutability enabled at repository scope before the first canonical release,
  with each asset set assembled in a draft and locked when that complete draft is published;
* exact match between the downloaded content, GitHub's asset digest when supplied, and the exact
  `SHA256SUMS` entry;
* release-workflow artifact attestations for independently inspectable build provenance.

GitHub documents both [immutable releases](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/establish-provenance-and-integrity/prevent-release-changes)
and [artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations).
SLSA recommends distributing provenance attestations alongside release artifacts. Runtime
signature or attestation verification can be added as another mandatory `Verifier` without
changing discovery, staging, CLI, or installation APIs.

TUF explicitly addresses rollback, freeze, and mix-and-match attacks. It is the stronger complete
update framework, but adopting its repository roles, trusted-root rotation, expiry, and metadata
operations for these single-repository tools would be disproportionate in this decision. It is a
documented future option, not something the shared API should prevent.

#### F7 — staging and replacement need one cross-platform correctness contract

`prepare-commit-msg` has the safer download staging primitive. It uses `os.CreateTemp` in the
target directory, receives mode `0600`, hashes while streaming, calls `File.Sync`, closes the
file, and cleans it on error (`internal/selfupdate/client.go:236-301`). Magic Remote uses a
predictable `<asset>.staging` path, removes it, and opens it without `O_EXCL`
(`internal/update/download.go:58-86`, `110-137`). A predictable path in a writable executable
directory creates avoidable collision and symlink-following risk, and concurrent invocations can
interfere.

Neither implementation has a cross-process per-target update lock. Both use fixed backup or
staging names, so two update commands can race discovery, replacement, and cleanup.

Replacement behavior also differs:

* `prepare-commit-msg` uses one POSIX `os.Rename(staged, target)` and has no retained backup or
  post-rename rollback (`apply_unix.go:15-38`).
* Magic Remote renames target to `.prev`, then staged to target, retaining rollback for later
  service failures (`internal/update/swap.go:70-194`). There is a window in which the target name
  does not exist.
* Both Windows implementations use sequences of `os.Rename`, retries, and a backup name. Magic
  Remote's comments call the operation atomic, but Go 1.26.5 explicitly documents that even a
  same-directory `os.Rename` is not atomic on non-Unix platforms.
* Only `prepare-commit-msg` syncs the downloaded file; neither implementation has one explicit
  contract for syncing the containing directory/rename state before declaring durable success.
* Both resolve symlinks, but Magic Remote separately combines the resolved directory with the
  unresolved executable basename (`run.go:117-130`), which can select the wrong destination when
  an invocation symlink has a different basename from its target.

Go 1.26.5 provides the right portable building blocks: `os.CreateTemp`, `File.Sync`,
`os.Executable`, `filepath.EvalSymlinks`, typed wrapped errors, and `os.Root` for anchoring
relative operations beneath the already-resolved executable directory. It does not provide one
portable atomic executable-replace guarantee. The replacement step therefore needs OS-specific
files and native tests:

* Unix: same-directory exclusive staging, synced content, a recoverable backup, atomic rename
  over the target, and directory sync where supported;
* Windows: a documented Win32 replacement/move primitive through `golang.org/x/sys/windows`, a
  backup and rollback path, and no claim stronger than the native API provides;
* every OS: regular-file and basename validation, a per-target update lock, explicit symlink
  resolution policy, cleanup that cannot follow an attacker-chosen staging symlink, and a native
  test that updates a temporary copy while it is executing.

#### F8 — macOS code signing is a transformation, not part of checksum verification

Magic Remote optionally runs `codesign` against the staged binary after checksum verification
(`internal/update/swap.go:58-68`). That modifies the verified release bytes. The current code also
attempts `codesign` on any OS when the environment variable is set, and prints a macOS TCC note
on non-macOS systems when it is unset.

The shared pipeline must distinguish verification from a consumer-authorized post-verification
transformation. A macOS-only transform may re-sign a verified staged binary, must verify the
resulting signature, and must record that the installed digest intentionally differs from the
release digest. It must not be a generic core option named only by an environment variable.

#### F9 — MCP stdout constraints require structured reporting, not direct global printing

Recall redirects `os.Stdout` to `os.Stderr` for the entire Cobra execution to protect MCP JSON-RPC
(`cmd/mcp-server-recall/root.go:33-48`). MagicTools hijacks stdout only when `serve` starts
(`cmd/mcp-server-magictools/serve.go:17-43`). Magic Remote and `prepare-commit-msg` use ordinary
CLI streams.

A shared updater must never mutate global streams and must not make service or MCP assumptions.
It should emit structured progress through an injected reporter and request confirmation through
an injected confirmer. A supplied plain-text implementation accepts `io.Reader`/`io.Writer`
values; each command decides whether human output belongs on stdout or stderr. This follows the
renderer-agnostic pattern already adopted by `mcplib/wizard` rather than importing a UI toolkit.

### Current Strengths to Preserve

Canonicalization must retain the strongest behavior already present:

* exact target versions and multi-platform coverage from `prepare-commit-msg`;
* check-available exit 10, confirmation by default, local-build protection, service healing,
  definition refresh, and rollback from Magic Remote;
* exact release asset names and strict tag checks from Recall, MagicTools, and
  `prepare-commit-msg`;
* injected HTTP clients/URLs and filesystem seams used by both existing test suites;
* same-directory temporary files, streaming hash verification, sync-before-apply, and cleanup
  from `prepare-commit-msg`;
* independent product lifecycle handling from Magic Remote;
* output injection so MCP stdout remains protocol-clean.

## Decision Drivers

* One implementation of discovery, selection, download, verification, staging, locking,
  replacement, rollback, result semantics, and text reporting.
* One release identity: a strict, immutable SemVer GitHub tag.
* One exact raw-binary asset naming convention across repositories.
* Go 1.26.5 minimum compatibility and idiomatic use of stdlib plus narrowly scoped Go project
  modules.
* Safe behavior under interruption, simultaneous invocations, partial filesystem failure,
  checksum mismatch, service-start failure, and unsupported platform.
* No silent replacement of a local development build, package-manager-owned install, or
  unexpected symlink target.
* Integrity checks that fail closed and a verifier interface that can grow to authenticated
  metadata without redesign.
* Scriptable CLI behavior with stable typed results and check-mode exit status.
* Interactive safety by default without making tests depend on a real TTY.
* Consumer-owned lifecycle, rendering, CLI framework, and platform matrix.
* Additive-first migration so `mcplib` can release before consumers remove their internal code.
* A public API small enough to support across the 13 current consumers.

## Considered Options

* Canonical release contract plus a policy-driven, renderer-agnostic `mcplib/selfupdate` core,
  with standalone and managed-service installers
* Move Magic Remote's current updater into `mcplib` and parameterize product/repository names
* Adopt `creativeprojects/go-selfupdate` and wrap product lifecycle around it
* Share only low-level download and replacement primitives; retain each CLI coordinator
* Keep repository-local implementations and copy the preferred one into new projects

## Decision Outcome

Chosen option: "Canonical release contract plus a policy-driven, renderer-agnostic
`mcplib/selfupdate` core, with standalone and managed-service installers", because it removes the
duplicated failure-prone mechanism while retaining real product differences at explicit seams,
and because standardizing immutable SemVer releases and exact assets avoids enshrining Magic
Remote's historical `BASE.N` exception in every future project.

### Canonical Release Contract

Every migrated product release satisfies all of the following:

* Tag: strict `vMAJOR.MINOR.PATCH`, no leading zeroes, immutable after publication.
* Embedded release version: the tag version, normalized consistently for display; development
  identity is recorded separately as release or local, not inferred from arbitrary version text.
* Executable asset: `<product>-<goos>-<goarch>[.exe]`, selected by exact equality.
* Manifest asset: exactly `SHA256SUMS`, with exactly one valid SHA-256 entry per executable asset.
* GitHub release asset metadata: state `uploaded`, positive bounded size, and SHA-256 digest when
  GitHub supplies it.
* Repository release immutability is enabled before the first canonical release. Each release is
  assembled as a draft and published only when complete; rerunning an already-published tag fails
  rather than mutating its assets.
* A rebuilt fix gets a new patch tag. Magic Remote no longer allocates a new `N` beneath an old
  release tag.
* Release workflows generate artifact attestations. Runtime enforcement remains a verifier-policy
  extension, not a false claim made by a same-origin checksum.
* Release workflow actions and cross-repository reusable-workflow calls are pinned to verified
  full commit SHAs. The workflow commit is the same commit referenced by the mcplib module tag.

Magic Remote needs one explicit bridge exception. Its installed legacy updater selects only
`<product>-<goos>-<goarch>-<BASE.N>[.exe]` and therefore cannot consume a release containing only
canonical exact names (`internal/update/github.go:81-111`). The first immutable canonical-tagged
release must consequently publish both sets from the same verified bytes: the canonical exact
binary plus `SHA256SUMS`, and version-suffixed compatibility copies plus
`SHA256SUMS-<tag-version>`. The binary is stamped with the three-part tag in both copies; no new
`BASE.N` is allocated. This bridge release must use a higher three-part tag than every currently
installed base so the transition is unambiguous. After the declared legacy support window, later
releases emit only canonical exact names. The bridge exception is consumer-local release
compatibility and does not enter `mcplib`'s canonical selector.

GitHub documents that enabling immutable releases does not retroactively protect existing
releases. The shared client therefore requires selected release metadata to report an immutable
release. It will not install or roll back to an older mutable Magic Remote release. Legacy
compatibility is deliberately one-way: an old client can consume the immutable bridge release;
after crossing the bridge, the shared client consumes only canonical immutable releases.

### Canonical CLI Contract

Every product exposes:

```text
<product> update [--check] [--force] [--version vX.Y.Z] [--yes|-y]
```

Semantics:

* No flags: discover the latest stable release; if newer, ask for confirmation and apply it.
* `--check`: perform discovery and policy evaluation only; download and apply nothing.
* `--version`: select that exact release. A lower exact target is an explicit rollback request and
  is reported as such before confirmation.
* `--force`: allow replacing a local/development build or reinstalling the selected version. It
  never bypasses version syntax, asset selection, HTTPS/source restrictions, size limits,
  verification, target ownership, or lifecycle health checks.
* `--yes`/`-y`: approve the already-selected operation without prompting. It has no effect on
  discovery or policy.
* Non-interactive apply without `--yes` fails with an actionable error instead of hanging,
  silently aborting, or silently replacing the binary.
* Positional arguments are rejected.
* `--check` with an operation-changing flag such as `--yes` is rejected as contradictory;
  `--check --force` is also rejected instead of silently ignoring force.
* The command derives all work from the caller's context. Consumers may add a bounded timeout but
  must not replace `cmd.Context()` with `context.Background()`.

Exit behavior is standardized by a library helper but `os.Exit` remains in each `main` package:

| Outcome | Exit code |
|---|---:|
| Current version already equals the selected release | 0 |
| `--check` finds a different actionable target (upgrade or explicit rollback) | 10 |
| User declines an interactive apply | 0 |
| Invalid input, unsupported platform, network, verification, filesystem, lifecycle, or rollback failure | 1 |

The library returns typed `Result` and wrapped errors. It never exits the process. A rollback that
restores the binary but cannot restore service health remains an error and says both facts.

### Package Boundary

The new public package is `github.com/maccavelli/mcplib/selfupdate`. It does not import Cobra,
PTerm, Bubble Tea, or a service manager. The following conceptual API is the design constraint;
exact identifiers may be refined in the implementation plan without changing ownership:

```go
type Request struct {
    Product        string
    CurrentVersion string
    CurrentBuild   BuildKind // ReleaseBuild or LocalBuild
    TargetVersion  string    // empty means latest stable
    Platform       Platform  // zero means runtime GOOS/GOARCH
    CheckOnly      bool
    Force          bool
}

type Updater struct {
    Source        ReleaseSource
    Versions      VersionPolicy
    Assets        AssetSelector
    Verifier      Verifier
    Installer     Installer
    Reporter      Reporter
    Confirmer     Confirmer
    Limits        Limits
}

func (u *Updater) Run(ctx context.Context, request Request) (Result, error)
func ExitCode(result Result, err error) int
```

The standard constructor supplies the canonical policy:

* strict SemVer backed by `golang.org/x/mod/semver` with full-tag validation;
* exact raw-binary platform asset selection;
* a GitHub Releases source using one injected, reusable `http.Client`;
* GitHub digest plus strict `SHA256SUMS` verification;
* bounded same-directory exclusive staging;
* standalone executable replacement with lock, backup, rollback, and durability handling;
* plain-text reporting and confirmation over injected streams.

Interfaces exist only where the four products prove variation:

* `ReleaseSource`: GitHub now; another source later without changing the coordinator.
* `AssetSelector`: the one canonical exact-name selector; the Magic Remote bridge is a publication
  concern for old clients, not a second shared-client selection policy.
* `VersionPolicy`: one strict exported SemVer policy; legacy normalization stays consumer-local.
* `Verifier`: composable integrity/authenticity policies.
* `Installer`: standalone apply or managed-service apply.
* `Reporter` and `Confirmer`: protocol-safe streams, styling, and deterministic tests.

HTTP clients, filesystem operations, clocks, runtime platform, and executable location are
injected through concrete configuration or narrow unexported test seams where possible; they do
not each become a public interface.

### Core State Machine

The coordinator owns one ordered state machine:

```text
validate request
  → resolve target policy
  → fetch latest or exact release
  → require immutable release and validate strict version
  → select exactly one platform asset and exact manifest
  → compare desired/current build identity
  → return check result OR confirm
  → acquire target lock, re-resolve target policy, and reject a changed target
  → stream both assets within limits
  → verify manifest, GitHub digest, size, and verifier chain
  → apply authorized platform transform, if any
  → snapshot lifecycle and stop only when managed
  → replace executable and retain rollback material
  → reconcile managed definition, start, and verify health
  → commit by removing backup and releasing the stable lock
```

Every failure after staging removes the staging file. Once a managed service has been stopped,
every later failure enters one recovery path: retain or restore the old binary as appropriate,
restore any changed definition, and restart the previously installed service. Every failure after
replacement enters the same binary rollback path. An error from recovery is joined with the
original error so neither is hidden. No service is installed implicitly by `update`.

### Standalone and Managed Installation

`StandaloneInstaller` owns target resolution and binary replacement only. It is used by
`prepare-commit-msg`, Recall, and any MCP server without a managed background service.

`ManagedInstaller` composes the same replacer with two consumer implementations:

```go
type Lifecycle interface {
    Installed(context.Context, string) (bool, error)
    Running(context.Context, string) (bool, error)
    Stop(context.Context, string) error
    Start(context.Context, string) error
    WaitHealthy(context.Context, string) error
}

type Reconciler interface {
    Reconcile(ctx context.Context, product, executable string) (ReconcileResult, error)
    Restore(ctx context.Context, product string, receipt ReconcileResult) error
}
```

Magic Remote adapts its current `internal/cli/service` functions and `ExecRefresher` to these
interfaces. MagicTools may adapt its service command when its update command is added. Products
without a service pass neither interface and cannot accidentally start a daemon.

The managed policy preserves Magic Remote's matrix:

| Product service definition | Pre-update process | Apply behavior |
|---|---|---|
| Absent | Irrelevant | Replace binary only; no stop, reconcile, start, or implicit install |
| Present | Running | Stop, replace, reconcile, start, wait healthy; rollback on failure |
| Present | Down | Replace, reconcile, start, wait healthy; rollback on failure |

Whether a failed definition reconcile is fatal is an explicit product policy. It is not hidden as
a logged warning in the generic replacer.

### Version and Build Identity

Release comparison and local-build protection are separate concerns:

* `VersionPolicy` validates and compares release versions only.
* `BuildKind` states whether the running binary is a release or local build.
* Release workflows stamp both facts explicitly through build variables.
* A fallback may use `runtime/debug.ReadBuildInfo` and its `vcs.modified` setting for local builds,
  but a parse failure never silently becomes an ordered release version.
* `--force` is required to replace `LocalBuild`; it does not make invalid remote release metadata
  valid.

This removes both existing antipatterns: arbitrary invalid strings being lexically ordered in
`prepare-commit-msg`, and Magic Remote inferring publication from the number of dotted fields.

### Download and Verification Rules

The GitHub source and downloader must:

* pin `X-GitHub-Api-Version: 2026-03-10` and send the documented JSON accept header;
* set a product/version `User-Agent`;
* resolve tokens by explicit option, then `GH_TOKEN`, then `GITHUB_TOKEN`, without logging them;
* attach authorization only to the configured API origin and use the asset API URL for private
  asset download;
* use one reusable injected client with context deadlines and a redirect policy that never
  forwards credentials to an untrusted host;
* retain bounded response diagnostics and typed rate-limit metadata;
* hard-limit release JSON, manifest, error, and executable bodies;
* reject invalid basename, duplicate asset, duplicate checksum entry, malformed hex digest,
  non-uploaded state, non-positive/oversized asset, size mismatch, and digest mismatch;
* create the staging file with `os.CreateTemp` in the resolved target directory;
* hash while streaming, sync, close, verify, and clean up on every failure;
* use `os.Root` or equivalently anchored operations after resolving the trusted target directory,
  so metadata-derived names cannot traverse outside it.

The verifier chain names its guarantee. The baseline may say "release asset integrity verified";
it may not say "publisher signature verified" unless an independent signature or attestation
verifier actually ran.

### CLI Integration

The library owns operation semantics, prompts, progress events, results, and exit mapping. Each
repository owns only:

* binding its CLI framework's flags into `selfupdate.Request`;
* providing product/repository/version/build metadata and supported platforms;
* selecting stdout or stderr for its injected text reporter;
* adapting service lifecycle where applicable;
* mapping `selfupdate.ExitCode` in `main`.

This deliberately does not add Cobra to `mcplib`. Ten sibling modules currently use Cobra, but a
public shared library is also used by plain-stdlib CLIs and headless packages. Approximately ten
lines of framework-specific flag binding per command are cheaper and more stable than making a
Cobra command tree part of `mcplib`'s public API.

### Migration Order Constraint

Migration is additive-first:

* release `mcplib/selfupdate` with the canonical contract and exhaustive tests;
* cut canonical release-workflow support before pointing a consumer at it;
* migrate one existing updater and run native end-to-end update fixtures;
* migrate the second existing updater, publish its one dual-name bridge release, and delete its
  internal implementation;
* add commands to Recall and MagicTools using the already-released API;
* remove Magic Remote's installed-version normalizer after the supported migration window.

This is sequencing rationale, not an executable implementation plan. Exact phases, files,
commands, acceptance criteria, and commits belong in the associated plan requested after this
MADR is reviewed.

### Consequences

* Good, because 1,823 lines of production updater code converge on one maintained mechanism
  before more repositories copy either implementation.
* Good, because all projects gain exact-version install, default confirmation, scriptable check
  status, bounded downloads, target locking, rollback, and typed outcomes.
* Good, because strict immutable SemVer releases eliminate `BASE.N` parsing and same-tag mutation
  rather than making that exception a permanent cross-project abstraction.
* Good, because three repositories' exact build-output convention becomes the one publication
  convention; one immutable Magic Remote bridge release keeps legacy clients on an upgrade path.
* Good, because the most sophisticated service behavior remains reusable without forcing service
  concepts into standalone CLIs.
* Good, because MCP stdout safety and product styling remain consumer decisions behind injected
  reporting.
* Good, because Go 1.26.5's safe primitives and official `x/mod/semver` replace handwritten
  parsers and predictable staging paths.
* Good, because verification guarantees become explicit and extensible instead of conflating a
  same-origin checksum with publisher authenticity.
* Good, because the shared state machine can be tested against every failure boundary once and
  native update smoke tests can exercise the same code every consumer imports.
* Neutral, because the library grows while two repositories shrink and two gain commands; net LOC
  reduction is secondary to eliminating behavioral arity.
* Neutral, because GitHub remains the initial source implementation and TLS/GitHub account control
  remains part of the baseline trust boundary.
* Neutral, because existing package-manager installs remain outside self-update ownership; their
  command must direct users back to the manager.
* Bad, because immutable releases remove Magic Remote's ability to republish an old tag with a new
  build serial. A fix must receive a new patch release, which is intentional standards compliance
  but changes current operations.
* Bad, because this is a coordinated multi-repository migration requiring `mcplib` releases and
  downstream version bumps.
* Bad, because Windows replacement remains OS-specific and cannot honestly promise POSIX rename
  atomicity. Native tests and rollback reduce the risk; comments cannot erase it.
* Bad, because baseline checksum plus GitHub digest verification still trusts GitHub and the
  release publisher. Independent runtime signature/attestation verification remains additional
  work.
* Bad, because a public `selfupdate` API becomes a compatibility obligation. The decision limits
  interfaces to variations demonstrated by the assessed products.

### Confirmation

Compliance is confirmed by all of the following:

* `mcplib` remains buildable and testable with Go 1.26.5.
* Strict-version tables cover stable, prerelease, build metadata, leading-zero, incomplete,
  illegal-identifier, invalid, local-build, and legacy Magic Remote inputs.
* Release tests reject a tag or asset version mismatch, draft, unexpected prerelease, mutable
  same-tag rebuild, duplicate exact asset, missing platform, unsupported platform, and malformed
  metadata.
* Downloader tests cover oversized JSON/manifest/binary bodies, advertised-size mismatch,
  malformed and duplicate checksum lines, GitHub digest mismatch, manifest mismatch, redirects,
  token-origin policy, timeouts, cancellation, cleanup, and rate-limit errors.
* Filesystem tests cover symlinked invocation, differently named symlink target, staging collision,
  concurrent updates, stale backup, permission denial, full disk/write failure, interrupted apply,
  rollback failure, and package-manager target refusal.
* Native Linux, macOS, and Windows tests update a temporary executable copy while it is running
  and verify old-or-new recoverability at every injected failure point.
* Managed lifecycle tests cover the absent/running/down matrix, reconcile failure policy, failed
  health check, binary rollback, definition rollback, successful healing, and rollback-without-
  service-recovery error reporting.
* CLI contract tests run for every consumer: help, no args, extra args, every flag, contradictory
  flag pairs, TTY confirmation yes/no, non-TTY without `--yes`, exact upgrade, exact rollback,
  local build with/without force, and exit codes 0/10/1.
* Recall's and MagicTools' update commands never start the MCP server or initialize its datastore
  merely to check or apply an update.
* Recall's stdout remains JSON-RPC-clean according to its established redirection contract.
* `prepare-commit-msg --yes` is observably used, and a plain update no longer performs an
  unconfirmed replacement.
* Magic Remote retains independent `mcremote`/`mcrelay` lifecycle behavior and no-unit binary-only
  updates.
* Each release pipeline publishes exact canonical assets, verifies them, generates attestations,
  and refuses to modify a completed release; only Magic Remote's declared bridge release also
  carries byte-identical legacy compatibility names and its separate compatibility manifest.
* Once all consumers migrate, repository searches find no production GitHub release client,
  SemVer parser, checksum parser, staging downloader, or executable replacer outside
  `mcplib/selfupdate`, except Magic Remote's temporary installed-version normalizer and consumer
  lifecycle or platform-transformation adapters.

## Pros and Cons of the Options

### Canonical release contract plus a policy-driven, renderer-agnostic `mcplib/selfupdate` core, with standalone and managed-service installers

* Good, because it deduplicates both mechanics and observable CLI behavior.
* Good, because it treats service lifecycle, reporting, verification, and source as explicit
  composition points proven necessary by the assessed products.
* Good, because it removes the non-standard release exception instead of exporting it forever.
* Good, because it remains CLI-framework- and UI-toolkit-neutral.
* Bad, because it requires coordinated release-pipeline and consumer migrations.
* Bad, because careful API design and native cross-platform fixtures cost more up front than a
  direct code move.

### Move Magic Remote's current updater into `mcplib` and parameterize names

* Good, because it starts from the strongest service rollback and health behavior.
* Good, because two product commands already exercise it.
* Neutral, because its existing test suite provides useful migration fixtures.
* Bad, because it would export Magic Remote's non-SemVer `BASE.N`, version-suffixed asset parsing,
  service refresh assumptions, codesign environment variable, and product-oriented output.
* Bad, because its predictable staging file, missing target lock, permissive checksum selection,
  and Windows atomicity claim should be corrected before reuse.

### Adopt `creativeprojects/go-selfupdate` and wrap product lifecycle around it

* Good, because the upstream project supports GitHub/GitLab/Gitea, platform discovery, archives,
  checksums/signatures, rollback, and Linux/macOS/Windows.
* Good, because it is maintained prior art rather than a novel updater design.
* Neutral, because its validator and source separation validate the architecture chosen here.
* Bad, because these repositories publish raw binaries from one source and do not need archive,
  multi-forge, ARM fallback, or naming-rule breadth.
* Bad, because the local CLI result contract, MCP stream routing, explicit target ownership,
  service reconcile/health transaction, Magic legacy migration, exact asset policy, and release
  immutability rules would still require substantial wrapper code.
* Bad, because delegating the most security-sensitive filesystem behavior does not remove the need
  for native tests and upstream-version review.

### Share only low-level primitives and retain each CLI coordinator

* Good, because download, checksum, and replacement code would stop duplicating.
* Good, because adoption could be incremental with a smaller first API.
* Bad, because confirmation, exact-target, check status, force semantics, local-build policy,
  errors, progress, and lifecycle transaction would continue drifting.
* Bad, because the observed defects are primarily coordination and contract defects, not only
  helper duplication.

### Keep repository-local implementations and copy the preferred one

* Good, because each repository can change independently.
* Good, because no shared public API or coordinated module release is needed.
* Bad, because the two current implementations already disagree and require 1,823 production
  lines.
* Bad, because every future fix and security hardening change must be found and ported repeatedly.
* Bad, because history shows that even one implementation needed several post-release fixes to
  version, service, Windows, and rollback behavior.

## More Information

### Primary Research Sources

* [Go 1.26 release notes](https://go.dev/doc/go1.26)
* [Go 1.26.5 `os` package documentation](https://pkg.go.dev/os?tab=doc)
* [Go 1.26.5 `net/http` package documentation](https://pkg.go.dev/net/http@go1.26.5)
* [Go Modules reference](https://go.dev/ref/mod)
* [`golang.org/x/mod/semver` documentation](https://pkg.go.dev/golang.org/x/mod/semver)
* [Semantic Versioning 2.0.0](https://semver.org/)
* [GitHub REST releases endpoints](https://docs.github.com/en/rest/releases/releases)
* [GitHub REST release-assets endpoints](https://docs.github.com/en/rest/releases/assets)
* [GitHub REST API best practices](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api)
* [GitHub REST API rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api)
* [GitHub immutable releases](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/establish-provenance-and-integrity/prevent-release-changes)
* [GitHub artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations)
* [GitHub Actions secure-use reference](https://docs.github.com/en/actions/reference/security/secure-use)
* [SLSA provenance distribution](https://slsa.dev/spec/draft/distributing-provenance)
* [The Update Framework security model](https://theupdateframework.io/docs/security/)
* [Microsoft `ReplaceFile`](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-replacefilea)
* [Microsoft `MoveFileEx`](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexa)
* [`creativeprojects/go-selfupdate`](https://github.com/creativeprojects/go-selfupdate)
* [`minio/selfupdate`](https://github.com/minio/selfupdate)

### Repository Evidence Index

* `magic-cli-remote/internal/cli/update.go`
* `magic-cli-remote/internal/relay/update.go`
* `magic-cli-remote/internal/update/`
* `magic-cli-remote/internal/cli/service/exec_refresher.go`
* `magic-cli-remote/docs/0065-MADR-update-automation.md`
* `magic-cli-remote/docs/0100-MADR-update-unit-refresh-and-daemon-reload.md`
* `magic-cli-remote/docs/0103-MADR-update-tracks-release-build-and-active-service.md`
* `magic-cli-remote/.github/workflows/ci.yml`
* `prepare-commit-msg/main.go`
* `prepare-commit-msg/internal/selfupdate/`
* `prepare-commit-msg/docs/decisions/0002-MADR-self-update-cli-and-github-releases-integration.md`
* `prepare-commit-msg/scripts/verify-release.sh`
* `prepare-commit-msg/.github/workflows/ci.yml`
* `mcp-server-recall/cmd/mcp-server-recall/root.go`
* `mcp-server-recall/Makefile`
* `mcp-server-recall/.github/workflows/ci.yml`
* `mcp-server-magictools/cmd/mcp-server-magictools/root.go`
* `mcp-server-magictools/cmd/mcp-server-magictools/service.go`
* `mcp-server-magictools/Makefile`
* `mcp-server-magictools/.github/workflows/ci.yml`

### Review Questions

The proposed outcome is complete without answers to these questions, but reviewer decisions can
change its trade-offs before acceptance:

* Should runtime authenticity verification be mandatory in the first shared release, and if so,
  should the trust root be an embedded Ed25519 key, Sigstore identity policy, or TUF metadata?
* Is exact `--version` sufficient authorization for a rollback, or should rollback additionally
  require a distinct `--allow-downgrade` flag?
* Which package-manager-owned paths or install markers must the default target policy refuse in
  the first release?
* Should a managed service-definition reconcile failure abort and roll back every product, or
  remain an explicit per-product policy as proposed?
* What support window is required before Magic Remote can remove its legacy `BASE.N` normalizer
  and version-suffixed asset selector?
