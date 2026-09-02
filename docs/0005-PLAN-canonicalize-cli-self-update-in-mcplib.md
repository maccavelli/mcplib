---
status: in-progress
date: 2026-09-02
associated-madr: 0005-MADR-canonicalize-cli-self-update-in-mcplib.md
decision-makers: mcplib maintainers
---

# Implement Canonical CLI Self-Update in mcplib

> Paired with [0005-MADR-canonicalize-cli-self-update-in-mcplib.md](0005-MADR-canonicalize-cli-self-update-in-mcplib.md).
> This plan implements the shared Go package, the canonical release workflow,
> both existing updater migrations, and new update commands for Recall and
> MagicTools, Socratic Thinker, and DuckDuckGo. It does not authorize
> implementation, repository-setting changes, pushes, tags, or releases; each
> external mutation remains an explicit gate.

<!-- markdownlint-disable MD013 MD024 MD029 MD036 -->

## 0. Execution Contract

This is the deterministic execution artifact for MADR 0005. Approval means the
defaults in Section 3 are accepted unless the reviewer edits them first.

Execution rules:

1. Do not change source, tests, dependencies, workflows, or repository settings
   until both this plan and MADR 0005 are accepted.
2. At implementation start, record each repository's HEAD and short status.
   Stop only for an overlapping change; preserve unrelated owner work exactly.
3. Never commit a local replace directive for mcplib. Consumer work begins only
   after the mcplib version in Gate G1 is publicly resolvable.
4. End every mutating implementation phase with a green repository and one
   commit in each repository touched by that phase. Read-only gates and audits
   record their evidence in Section 23 but do not manufacture empty commits.
5. Do not push, create or move a tag, publish a release, enable release
   immutability, or open a pull request without explicit authorization in the
   same turn. Plan approval alone is not that authorization.
6. In magic-cli-remote, run `make pre-add-check` with the phase's explicit
   changed-Go-file list before staging, run `make race` before the phase commit,
   and invoke `git commit --no-edit`. Do not pass `-m`.
7. In repositories that provide a Go pre-commit wrapper, run that wrapper for
   every changed Go file. Otherwise run gofmt and per-file golint for every
   changed Go file before staging. Then run the repository checks named by the
   phase.
8. Record command output, commit IDs, release URLs, deviations, and rollback
   actions in Section 23 during execution. Do not silently widen a phase.

No datastore migration, configuration migration, automatic background polling,
or Android updater change is part of this plan.

## 1. Verified Baseline

### 1.1 Checkout snapshot

The plan was written against the following read-only snapshot:

| Repository | Branch | HEAD | Go directive | mcplib dependency | Working-tree note |
|---|---|---|---|---|---|
| mcplib | main | fc2c94db7817 | 1.26.5 | self | clean at Phase 0 start 2026-09-02; HEAD moved from planning snapshot dc1221873b17 by docs-only hardening commit fc2c94d |
| magic-cli-remote | master | 1b7abccc0747 | 1.26.5 | none | one owner-modified Dart diagnostics file; commits since the prior snapshot affect mobile code and MADRs 0128-0130, not Go updater code |
| prepare-commit-msg | main | 79cdba965289 | 1.26.6 | v1.2.0 | clean |
| mcp-server-recall | main | e22b9adf8c43 | 1.26.5 | v1.2.0 | clean |
| mcp-server-magictools | main | c672a720f8c3 | 1.26.5 | v1.2.0 | owner MADR/PLAN 0002 files present |
| mcp-server-socratic-thinker | ci/harden-release-pipeline | f62221a76ece | 1.26.5 | v1.2.0 | clean; two committed CI-hardening changes not on main |
| mcp-server-duckduckgo | main | 9a312807504f | 1.26.5 | v1.2.0 | clean |

The Go-directive column records each repository's floor as of the planning
snapshot. Superseded 2026-09-02 by MADR/PLAN 0006, which raises every module to
`go 1.26.6`; the snapshot values are left as written.

The local toolchain was go1.26.5 darwin/arm64 except that prepare-commit-msg
selected go1.26.6 from its module directive. The targeted tests in Section 1.2
were rerun on 2026-09-02 and passed at these heads; Recall completed in 51.916s.

Before changing a repository, run:

    git rev-parse --short=12 HEAD
    git status --short
    go version
    go env GOMOD GOOS GOARCH

If an anchor in this plan moved, re-read the affected function and update this
plan before implementing rather than applying a stale mechanical edit.

### 1.2 Existing behavior and measured duplication

The MADR's targeted baseline tests passed:

    (mcplib) go test ./...
    (magic-cli-remote) go test ./internal/update ./internal/cli ./internal/relay
    (prepare-commit-msg) go test ./internal/selfupdate .
    (mcp-server-recall) go test ./cmd/mcp-server-recall
    (mcp-server-magictools) go test ./cmd/mcp-server-magictools
    (mcp-server-socratic-thinker) go test ./cmd/mcp-server-socratic-thinker
    (mcp-server-duckduckgo) go test ./cmd/mcp-server-duckduckgo

Production updater code is currently 1,823 lines:

| Repository | Files | Production lines | Material behavior |
|---|---|---:|---|
| magic-cli-remote | internal/update/*.go excluding tests, internal/cli/update.go, internal/relay/update.go | 992 | confirmation, exit 10, service healing, definition refresh, rollback; no exact version |
| prepare-commit-msg | internal/selfupdate/*.go excluding tests; main.go runUpdate binding is ~29 additional lines | 831 (selfupdate subpackage) | exact version and six targets; yes is unused, check-available exits 0 |
| Recall | none | 0 | no update command |
| MagicTools | none | 0 | no update command |
| Socratic Thinker | none | 0 | no update command |
| DuckDuckGo | none | 0 | no update command or effective stamped version variable |

The plan preserves the strongest behavior, deletes both general-purpose local
implementations after migration, and permits only consumer lifecycle adapters
and Magic Remote's time-bounded installed-version normalizer and lifecycle
adapters.

### 1.3 Code anchors that constrain the work

| Fact | Evidence |
|---|---|
| Magic Remote check mode already maps update-available to exit 10 | cmd/mcremote/main.go:18-27, cmd/mcrelay/main.go:18-25 |
| Magic Remote replaces the caller context with a background context | internal/cli/update.go:34-57, internal/relay/update.go:23-46 |
| Magic Remote service behavior distinguishes installed from running | internal/update/run.go:133-159, internal/update/service.go:3-74 |
| Magic Remote's legacy updater selects only version-suffixed binaries | internal/update/github.go:81-111 |
| Prepare parses yes but never consumes it | main.go:109-137 |
| Prepare check mode returns nil whether an update exists | internal/selfupdate/updater.go:162-173 |
| Prepare's handwritten parser accepts incomplete and otherwise invalid SemVer | internal/selfupdate/semver.go:27-80 |
| Recall globally initializes Viper and fsnotify for every executed command | cmd/mcp-server-recall/root.go:51-58, internal/config/config.go:73-199 |
| Recall redirects Cobra output to stderr to protect MCP stdout | cmd/mcp-server-recall/root.go:33-48 |
| MagicTools initializes the database only inside ServeFunc, not root setup | cmd/mcp-server-magictools/main.go:98-145 |
| MagicTools already has Linux, macOS, and Windows service commands | cmd/mcp-server-magictools/service.go and service_windows.go |
| Prepare builds canonical exact assets unchanged | scripts/verify-release.sh and .github/workflows/ci.yml:53-70 |
| Recall and MagicTools build exact assets, then publish extra suffixed copies | Recall ci.yml:307-350; MagicTools ci.yml:305-342 |
| The original four current publish jobs permit replacement with --clobber | each repository's release job |
| Magic Remote allocates and pushes build/BASE.N tags | Makefile:1-8, scripts/next-build-version.sh, ci.yml:144-200 |
| Socratic Thinker globally initializes Viper/fsnotify before every command | cmd/mcp-server-socratic-thinker/root.go:41-48; internal/config/config.go:32-69 |
| Socratic Thinker builds three exact binaries, then publishes versioned copies, aliases, two manifests, and installers with --clobber | Makefile:19-31; ci.yml:115-164,250-342 |
| Socratic Thinker's RawVersion defaults to v4.4.4 although its latest observed tag is v1.0.2 | cmd/mcp-server-socratic-thinker/version.go:9-11; local tag list |
| Socratic Thinker's CI hardening is committed on ci/harden-release-pipeline, not main | commits b220dc8 and f62221a; main at d69cfe9 |
| DuckDuckGo's global config initializer creates a cache directory and config file before every command | cmd/mcp-server-duckduckgo/root.go:35-37; internal/config/config.go:32-60 |
| DuckDuckGo opens its BuntDB scraping store only in the serve path | cmd/mcp-server-duckduckgo/serve.go:91-114 |
| DuckDuckGo builds and publishes three canonical exact binaries plus SHA256SUMS, but uses floating action tags and softprops/action-gh-release | Makefile:19-31; ci.yml:12-70 |
| DuckDuckGo's Makefile stamps main.RawVersion, but no such variable or Cobra Version assignment exists | Makefile:15-31; cmd/mcp-server-duckduckgo/root.go:14-37 |

### 1.4 Research constraints incorporated by the plan

The implementation shall follow these primary sources:

* [Go 1.26 release notes](https://go.dev/doc/go1.26) and
  [Go 1.26.5 os documentation](https://pkg.go.dev/os?tab=doc): use
  os.CreateTemp, File.Sync, os.Executable, filepath.EvalSymlinks, and os.Root;
  do not claim os.Rename is atomic on every operating system.
* [golang.org/x/mod/semver](https://pkg.go.dev/golang.org/x/mod/semver) and
  [SemVer 2.0.0](https://semver.org/): use x/mod precedence, with a stricter
  wrapper requiring a complete vMAJOR.MINOR.PATCH tag.
* [GitHub release assets REST API](https://docs.github.com/en/rest/releases/assets)
  and [REST best practices](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api):
  pin API version 2026-03-10, use asset IDs and application/octet-stream,
  bound bodies, reuse a client, and retain rate-limit metadata.
* [GitHub immutable releases](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/establish-provenance-and-integrity/prevent-release-changes):
  enable the repository setting before the first protected release, because it
  is not retroactive; assemble a draft completely, then publish so its tag and
  assets become immutable together.
* [GitHub repository immutable-release REST endpoints](https://docs.github.com/en/rest/repos/repos#check-if-immutable-releases-are-enabled-for-a-repository):
  checking requires repository Administration read and enabling requires
  Administration write, so these operations are an administrator gate rather
  than a GITHUB_TOKEN workflow step.
* [GitHub artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations):
  grant id-token: write and attestations: write and attest the exact published
  files.
* [GitHub Actions secure use](https://docs.github.com/en/actions/reference/security/secure-use):
  pin action dependencies and reusable workflows to verified full-length commit
  SHAs; a movable major-version tag is not an immutable dependency.
* [GitHub CLI release upload](https://cli.github.com/manual/gh_release_upload):
  --clobber deletes an existing asset before uploading its replacement and is
  therefore forbidden for published artifacts.
* [Microsoft MoveFileEx](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw)
  and x/sys/windows: use MOVEFILE_REPLACE_EXISTING and
  MOVEFILE_WRITE_THROUGH with an explicit backup and rollback; make no POSIX
  atomicity claim for Windows.

Version selection for the new module dependencies is fixed as of 2026-09-02:
golang.org/x/mod v0.40.0 and golang.org/x/sys v0.47.0. Both declare Go 1.25
and therefore remain compatible with the Go 1.26.5 floor.

The package also imports golang.org/x/term v0.43.0 in its terminal confirmer.
That module was already a direct mcplib requirement at the Section 1.1 baseline
`fc2c94db7817`, added by `wizard`, so no new module enters the graph. Section 22's
go.sum rule is read as the pinned mcplib, x/mod, x/sys, and pre-existing x/term
graph.

## 2. Goal, Scope, and Non-goals

### Goal

Ship one public github.com/maccavelli/mcplib/selfupdate implementation and one
reusable release-publication workflow, migrate the two existing commands, add
commands to Recall, MagicTools, Socratic Thinker, and DuckDuckGo, and make all
six consumers' observable CLI behavior and release contract identical except
for product lifecycle, supported platform data, and the documented stdout versus
stderr routing needed to keep MCP protocol streams clean.

### In scope

* latest stable checks, exact stable targets, reinstall, and explicit rollback;
* confirmation, non-TTY refusal, structured progress, typed results and errors;
* GitHub Releases discovery and authenticated asset download;
* strict exact asset selection and SHA256SUMS verification;
* GitHub digest, advertised-size, state, and body-limit validation;
* safe staging, target ownership policy, per-target locking, backup and rollback;
* standalone and managed-service installation;
* Magic Remote codesign transformation and legacy bridge;
* release immutability, draft assembly, attestations, and no-clobber publication;
* standalone adoption by Socratic Thinker and DuckDuckGo without config,
  datastore, telemetry, dashboard, browser, or MCP-server startup;
* CLI tests and native Linux/macOS/Windows replacement tests.

### Out of scope

* prerelease installation in v1 of the package;
* mandatory runtime Sigstore, Ed25519, or TUF verification;
* background update polling;
* package-manager updates or privilege escalation;
* archive extraction or multi-file product updates;
* Android update behavior;
* deleting historical MADRs that document the old behavior;
* changing the products' supported platform matrices.

## 3. Defaults Resolved for Deterministic Execution

These values implement the resolved follow-on decisions in MADR 0005. Changing
one requires editing the MADR and this plan before execution.

1. Runtime trust baseline: verify HTTPS GitHub metadata, uploaded asset state,
   positive bounded size, GitHub's sha256 digest when present, and the exact
   SHA256SUMS entry. Generate build provenance attestations, but do not require
   runtime attestation verification in v1.
2. Rollback authorization: an exact lower --version is sufficient intent. It
   still requires interactive confirmation or --yes. No allow-downgrade flag is
   added.
3. Versions: latest and exact targets must be stable complete tags matching
   `^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`. Prerelease and build
   metadata tags are rejected by the canonical constructor.
4. Force: permits only a local-build replacement or same-version reinstall. It
   never bypasses input, source, asset, size, integrity, target, or health rules.
5. Target ownership: the default self-update policy allows only a regular file
   beneath the current user's home directory, rejects a symlink invocation, and
   rejects a target outside the allowed roots before downloading. Consumers may
   add an explicit canonical absolute operator-authorized root. The standard commands do not add
   /usr, /opt, /nix/store, Snap, Flatpak, Homebrew Cellar, Chocolatey, Scoop, or
   WindowsApps roots.
6. Managed reconcile: any stop, definition reconcile, start, or health failure
   is fatal and enters binary plus definition rollback. A rollback failure is
   joined with the initiating error.
7. Magic Remote bridge: v0.16.0 is the proposed first canonical-tagged release.
   It contains exact canonical assets and byte-identical version-suffixed
   compatibility assets, without BASE.N allocation. It remains the latest
   release for at least 90 days. At or after that deadline, v0.17.0 is the
   proposed first canonical-only release; remove the installed-version normalizer
   only in a later patch after v0.17.0 passes native update smoke.
8. Default hard limits: release JSON 2 MiB, error diagnostics 64 KiB, checksum
   manifest 1 MiB, executable 512 MiB, and one 15-minute caller operation
   deadline. An advertised executable size must be positive and no greater than
   the executable limit.
9. Token order: explicit option, GH_TOKEN, then GITHUB_TOKEN. Tokens are sent
   only to the configured GitHub API origin and never logged.
10. Check mode: no confirmation, asset-body download, staging, target lock, or
    service call. It resolves and validates metadata, selection, target policy,
    and version policy, then exits 0 or 10.
11. Stream policy: update human output goes to the consumer-provided error
    stream for MCP servers and to the normal CLI output stream for the other
    products. The shared package never reads or writes global standard streams.
12. Release workflow ownership: mcplib supplies a pinned reusable workflow.
    Consumers retain only build, native smoke, product-specific staging, and
    the expected asset matrix.
13. Release provenance boundary: the GitHub source requires the selected
    release's immutable field to be true. Repository immutability is enabled
    before the first canonical product release because GitHub does not apply it
    retroactively. Existing mutable releases are never install or rollback
    targets for the shared client.
14. Stamped identity: ReleaseBuild CurrentVersion is the raw strict v-tag.
    Consumers may trim the leading v only for presentation. Magic Remote alone
    normalizes an already-installed legacy BASE.N or BASE.N.gHASH identity to
    vBASE before constructing the Request.
15. Reporter and cleanup failures: all events through EventInstalling are
    emitted before Installer.Install and a reporting error aborts before
    replacement. Installers do not call Reporter. EventComplete is emitted only
    after Install returns a healthy committed result and Close has been
    attempted. A terminal reporting, backup-cleanup, or unlock error returns
    Applied true plus the joined error and does not roll back an otherwise
    healthy installation. The narrowly classified Windows running-image
    sharing violation instead returns PendingBackup and the synced cleanup
    receipt defined in Section 4.4; it is not reported as a failed update.
16. Administrative immutability check: the repository setting endpoint requires
    repository Administration permission, which a normal workflow GITHUB_TOKEN
    cannot request. Gate G2 therefore enables and verifies the setting with an
    explicitly authorized administrator token before a release tag is pushed.
    The reusable workflow verifies the published release itself is immutable;
    it does not claim that its contents:write token can inspect repository
    administration settings.

### 3.1 Frozen consumer asset matrices

The workflow inputs and release acceptance checks use these exact matrices. A
platform addition or removal is outside this plan and requires review before a
consumer phase changes it.

| Consumer/product | Exact platform pairs | Declared non-manifest extras |
|---|---|---|
| prepare-commit-msg | linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64 | none |
| mcremote and mcrelay | linux/amd64, linux/arm64, darwin/arm64, windows/amd64 | `install.sh`, `install.ps1`, and `magic-cli-remote-<tag>-arm64.apk` |
| mcp-server-recall | linux/amd64, linux/arm64, darwin/arm64, windows/amd64 | `install.sh`, `install.ps1` |
| mcp-server-magictools | linux/amd64, darwin/arm64, windows/amd64 | `install.sh`, `install.ps1` |
| mcp-server-socratic-thinker | linux/amd64, darwin/arm64, windows/amd64 | `install.sh`, `install.ps1` |
| mcp-server-duckduckgo | linux/amd64, darwin/arm64, windows/amd64 | none |

For the Magic Remote v0.16.0 bridge only, the release file set also contains
one byte-identical compatibility copy per product/platform and
`SHA256SUMS-0.16.0`; the bridge validator derives those names instead of taking
them through extra-assets-json. The APK's exact bridge basename is
`magic-cli-remote-v0.16.0-arm64.apk`. `SHA256SUMS` is mandatory manifest data,
not an entry in extra-assets-json.

## 4. Exact Public Package Contract

Phase 1 must freeze the following API before downstream work starts. Identifier
changes after mcplib v1.3.0 require a new reviewed plan.

### 4.1 Request, result, and exit behavior

    package selfupdate

    type BuildKind uint8

    const (
        BuildUnknown BuildKind = iota
        ReleaseBuild
        LocalBuild
    )

    type Platform struct {
        OS   string
        Arch string
    }

    type Request struct {
        Product        string
        CurrentVersion string
        CurrentBuild   BuildKind
        TargetVersion  string
        Platform       Platform
        CheckOnly      bool
        Force          bool
        Yes            bool
    }

    type Operation uint8

    const (
        OperationNone Operation = iota
        OperationUpgrade
        OperationReinstall
        OperationRollback
        OperationReplaceLocal
    )

    type Result struct {
        Product          string
        CurrentVersion   string
        TargetVersion    string
        ReleaseURL       string
        AssetName        string
        Operation        Operation
        Checked          bool
        Applied          bool
        Declined         bool
        ReleaseDigest    string
        InstalledDigest  string
        ServiceInstalled bool
        ServiceWasRunning bool
        PendingBackup    string
    }

    var ErrUpdateAvailable error
    var ErrConfirmationRequired error
    var ErrConcurrentUpdate error
    var ErrManagedInstall error
    var ErrUnsupportedPlatform error
    var ErrIntegrity error
    var ErrMutableRelease error
    var ErrRateLimited error

    type RateLimitError struct {
        StatusCode int
        RetryAfter time.Duration
        Reset      time.Time
        Remaining int64
    }

    func (e *RateLimitError) Error() string
    func (e *RateLimitError) Unwrap() error

    func ExitCode(result Result, err error) int

ExitCode returns 10 only when ErrUpdateAvailable is in the error chain, 1 for
all other errors, and 0 otherwise. Run returns Result plus
ErrUpdateAvailable when check mode finds any different explicitly actionable
target. The library never calls os.Exit.

RateLimitError unwraps to ErrRateLimited. Missing numeric headers use zero
values, and malformed optional rate-limit headers also produce zero values
rather than guessed guidance. Retry-After supports both delta-seconds and
HTTP-date using an unexported injected clock in tests.

Request validation requires product to match
`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$` and rejects an unknown build kind,
partial platform values, positional arguments at the consumer boundary, check
plus yes, and check plus force. A non-zero platform must exactly match an entry
supplied to the selector; OS and architecture strings are never interpolated
without that allow-list match. NewExactAssetSelector rejects duplicate platform
pairs and requires each OS and architecture field to match
`^[a-z0-9][a-z0-9_]*$`.

Operation classification is fixed by this table:

| Current identity | Selected target | Flags | Outcome |
|---|---|---|---|
| ReleaseBuild | higher | any valid set | OperationUpgrade |
| ReleaseBuild | equal | no force | OperationNone, exit 0 |
| ReleaseBuild | equal | force apply | OperationReinstall |
| ReleaseBuild | lower exact version | apply or check | OperationRollback |
| ReleaseBuild | lower latest release | any | error; latest cannot silently downgrade |
| LocalBuild | any valid stable target | check | OperationReplaceLocal, exit 10, report that apply requires force |
| LocalBuild | any valid stable target | apply without force | error before download |
| LocalBuild | any valid stable target | force apply | OperationReplaceLocal |

For every non-none check outcome, Run returns the populated Result with
ErrUpdateAvailable. `--force` on an ordinary higher or exact lower target does
not change its operation classification.

### 4.2 Release and selection interfaces

    type Repository struct {
        Owner string
        Name  string
    }

    type Release struct {
        ID         int64
        Tag        string
        URL        string
        Draft      bool
        Prerelease bool
        Immutable  bool
        Assets     []Asset
    }

    type Asset struct {
        ID     int64
        Name   string
        State  string
        Size   int64
        Digest string
    }

    type ReleaseSource interface {
        Latest(context.Context) (Release, error)
        ByTag(context.Context, string) (Release, error)
        OpenAsset(context.Context, Release, Asset) (io.ReadCloser, error)
    }

    type Selection struct {
        Binary       Asset
        Manifest     Asset
        ManifestName string
    }

    type AssetSelector interface {
        Select(Release, string, Platform) (Selection, error)
    }

    func NewExactAssetSelector([]Platform) (AssetSelector, error)

The canonical selector requires exactly one
product-goos-goarch plus .exe on Windows, exactly one SHA256SUMS, and uses the
binary's exact name as ManifestName. Duplicate exact names fail.

GitHubSource is a concrete exported implementation constructed from:

    type GitHubOptions struct {
        Repository Repository
        Client     *http.Client
        APIBaseURL *url.URL
        UserAgent  string
        Token      string
        Limits     Limits
    }

    func NewGitHubSource(GitHubOptions) (*GitHubSource, error)

NewGitHubSource requires a non-nil Client, clones its value, and never mutates
the caller's client. APIBaseURL nil means `https://api.github.com`; any supplied
base is normalized once and must have no user information, query, or fragment.
Repository owner/name and UserAgent are validated against control characters
and path separators before any URL is constructed. An empty Token triggers the
documented GH_TOKEN/GITHUB_TOKEN lookup; a non-empty explicit token wins.
Each canonical consumer passes one `http.Client{Timeout: 15 * time.Minute}` and
the same DefaultLimits value to GitHubOptions and Config; its 15-minute child
context is the outer deadline for discovery, download, installation, and
managed recovery.

HTTP test servers may use loopback HTTP. Any non-loopback source must be HTTPS.
Asset bodies are fetched from the asset API path derived from owner, repository,
and asset ID, not from a browser URL supplied by metadata.

    type VersionPolicy interface {
        Validate(string) error
        Compare(string, string) (int, error)
    }

    func NewStrictVersionPolicy() VersionPolicy

### 4.3 Verification and transformation

    type Verification struct {
        Product       string
        Release       Release
        Selection     Selection
        Size          int64
        SHA256        string
        ManifestSHA256 string
        GitHubSHA256  string
        Open          func() (io.ReadCloser, error)
    }

    type Verifier interface {
        Verify(context.Context, Verification) error
    }

    type TransformRequest struct {
        Product       string
        Platform      Platform
        Path          string
        ReleaseDigest string
    }

    type Transformer interface {
        Transform(context.Context, TransformRequest) error
    }

    type StagedArtifact struct {
        Path            string
        Size            int64
        ReleaseDigest   string
        InstalledDigest string
    }

The built-in verifier strictly parses SHA256SUMS: exactly two fields, optional
binary marker only, 64 lowercase-normalized hex characters, basename equal to
the selected ManifestName, no duplicate filename, and no path traversal.
Additional verifiers run after built-in integrity succeeds and receive only a
factory for a new read-only descriptor, not a writable staging path. The
default transformer is a no-op. Magic Remote's Darwin-only codesign transformer
is consumer code and runs codesign plus codesign --verify --strict. After every
transform, the coordinator requires the path still belongs to the locked
session, reopens it, rechecks the executable size limit, and computes
InstalledDigest itself; a transformer cannot assert its own digest.

### 4.4 Target and installer contracts

    type TargetPolicy struct {
        ExecutablePath string
        AllowedRoots   []string
    }

    type Target struct {
        Path string
        Dir  string
        Base string
    }

    type InstallRequest struct {
        Product  string
        Artifact StagedArtifact
    }

    type InstallResult struct {
        Target             string
        Backup             string
        Applied            bool
        ServiceInstalled   bool
        ServiceWasRunning  bool
        PendingBackup      string
    }

    type Installer interface {
        ResolveTarget(context.Context) (Target, error)
        Begin(context.Context, Target) (InstallSession, error)
    }

    type InstallSession interface {
        Target() Target
        CreateStaging(context.Context) (*os.File, string, error)
        Install(context.Context, InstallRequest) (InstallResult, error)
        Close() error
    }

    type Lifecycle interface {
        Installed(context.Context, string) (bool, error)
        Running(context.Context, string) (bool, error)
        Stop(context.Context, string) error
        Start(context.Context, string) error
        WaitHealthy(context.Context, string) error
    }

    type ReconcileResult struct {
        Changed bool
        Detail  string
        State   any
    }

    type Reconciler interface {
        Reconcile(ctx context.Context, product, executable string) (ReconcileResult, error)
        Restore(ctx context.Context, product string, receipt ReconcileResult) error
    }

    type InstallOptions struct {
        TargetPolicy TargetPolicy
        LockTimeout  time.Duration
    }

    const DefaultLockTimeout time.Duration = 5 * time.Second

    func NewStandaloneInstaller(InstallOptions) (*StandaloneInstaller, error)
    func NewManagedInstaller(*StandaloneInstaller, Lifecycle, Reconciler) (*ManagedInstaller, error)

Target resolution occurs before network work. Apply mode passes that exact
Target value to Begin after confirmation. Begin acquires the lock, re-resolves
and revalidates the path, compares the original and locked file identities, and
returns a session only if they match. Target carries an unexported built-in
identity snapshot; callers treat values as opaque and do not construct or
modify them. The session owns the stable lock, its anchored os.Root, every
staging/backup basename, and cleanup through Close. Run always defers Close and
joins its error without hiding an earlier failure.

ExecutablePath empty selects os.Executable. AllowedRoots are additive to the
mandatory canonical user-home root; an empty slice adds nothing. LockTimeout
zero selects DefaultLockTimeout and a negative duration is invalid. The six
canonical integrations do not set ExecutablePath or add a root.

CreateStaging uses os.CreateTemp in the locked target directory, returns the
open 0600 file plus its absolute path, and registers the basename for cleanup.
The downloader writes, hashes, syncs, and closes that file. Install accepts only
an artifact created by the same session. InstallResult.Applied distinguishes a
healthy committed installation from a pre-commit failure when backup cleanup,
reporting, or session close also returns an error.

The stable lock file is a dedicated basename beside the executable. Unix opens
it relative to the anchored directory with no-follow semantics and uses an
advisory x/sys/unix lock; Windows rejects reparse points and uses LockFileEx. A
second process times out with ErrConcurrentUpdate and never touches staging,
backup, or service state.

Windows has one explicit post-commit cleanup exception. If deleting the renamed
old executable fails with a native sharing violation attributable to the still-
running updater image, Install writes an exclusively created, synced cleanup
receipt with a user-only Windows DACL containing only the random backup basename
and its pre-update digest, returns that basename as PendingBackup, and treats
the installation as successful. Any other backup
cleanup failure remains an Applied true error. The next Begin under the same
target lock validates the receipt, regular-file/reparse status, basename, and
digest, removes that backup and receipt before network work, and fails closed if
it cannot do so. No glob-based stale-backup deletion is permitted.

Unix apply creates a random same-directory backup link or synced copy while
the old target remains live, renames the synced staging file over the target,
syncs the directory where supported, and retains the backup until lifecycle
health succeeds. Windows uses unique same-directory backup and staging names,
MoveFileEx with REPLACE_EXISTING and WRITE_THROUGH, retries only documented
transient sharing failures within the caller deadline, and restores the backup
if installing the new name fails.

### 4.5 Reporting, confirmation, construction, and limits

    type EventKind uint8

    const (
        EventUnknown EventKind = iota
        EventResolvingTarget
        EventFetchingRelease
        EventSelected
        EventDownloadingManifest
        EventDownloadingBinary
        EventVerified
        EventTransforming
        EventInstalling
        EventComplete
    )

    type Event struct {
        Kind       EventKind
        Product    string
        Current    string
        Target     string
        Asset      string
        Bytes      int64
        Detail     string
    }

    type Reporter interface {
        Report(context.Context, Event) error
    }

    type Prompt struct {
        Product   string
        Current   string
        Target    string
        Operation Operation
    }

    type Confirmer interface {
        Confirm(context.Context, Prompt) (bool, error)
    }

    type Limits struct {
        ReleaseJSON int64
        ErrorBody   int64
        Manifest    int64
        Executable  int64
    }

    func DefaultLimits() Limits
    func NewTextReporter(io.Writer) Reporter
    func NewTerminalConfirmer(*os.File, io.Writer) Confirmer

    type Config struct {
        Source      ReleaseSource
        Versions    VersionPolicy
        Assets      AssetSelector
        Verifiers   []Verifier
        Transformer Transformer
        Installer   Installer
        Reporter    Reporter
        Confirmer   Confirmer
        Limits      Limits
    }

    type Updater struct { /* unexported fields */ }

    func New(Config) (*Updater, error)
    func (u *Updater) Run(context.Context, Request) (Result, error)

New rejects nil Source, Versions, Assets, Installer, Reporter, or Confirmer and
rejects invalid limits. An empty Verifiers slice is valid, and a nil Transformer
means the built-in no-op transform. Consumers compose NewGitHubSource,
NewStrictVersionPolicy, NewExactAssetSelector, the installer, and the text
adapters explicitly; v1.3.0 does not add a second convenience constructor that
could hide security-relevant defaults.

### 4.6 Coordinator state machine

Run owns this exact order:

1. Validate Request and normalize runtime Platform.
2. Resolve the executable, symlink rule, regular-file rule, and allowed root.
3. Fetch latest stable or exact stable release with the caller context.
4. Require immutable true, then validate release state and tag, select exact
   binary and manifest metadata, and validate asset state, size, basename, and
   digest syntax.
5. Classify none, upgrade, reinstall, or explicit rollback. A latest release
   lower than the running release is an error; only an exact target can roll
   back.
6. In check mode, return current or Result plus ErrUpdateAvailable now. Perform
   no body download or filesystem/service mutation.
7. If not yes, require a TTY and confirm the fully selected operation. A decline
   returns Declined true and nil error.
8. Call Installer.Begin with the original Target. Begin acquires the target
   lock, re-resolves and revalidates beneath the same allowed root, and compares
   the initial and locked file identities with os.SameFile plus size and
   modification time. Return ErrConcurrentUpdate if the target changed while
   metadata or confirmation was in progress. Register an idempotent deferred
   InstallSession.Close as a safety net; the success and error paths also call
   Close explicitly so its error can be joined before returning.
9. Download the exact manifest within its limit and parse the one required
   entry.
10. Ask the locked session to create staging. Stream the binary through a limit
    reader and SHA-256 hasher into that open file; sync and close it.
11. Require actual size to equal API size and each available digest to agree.
12. Run additional verifiers through fresh read-only descriptors, then the
    optional post-verification transformer. Revalidate the registered staging
    path, size, and installed digest after transformation.
13. Snapshot managed lifecycle, stop only an installed service, apply the
    binary while retaining rollback material, reconcile, start, and wait healthy.
14. Once a managed service has been stopped, any later failure enters one
    recovery path. Restore the definition if reconcile began, restore the
    binary if replacement began, start and health-check the prior binary when
    the service was installed, and join every recovery error with the original.
15. On success, Install removes backup and staging names and syncs where
    supported, subject only to the documented Windows running-image receipt.
    Close releases the lock. Emit the terminal event and return Applied true.
    Join other cleanup, Close, or terminal-report errors with the successful
    result without initiating rollback after the health commit point.

Every Reporter call is checked. Events through EventInstalling occur before the
opaque Installer.Install call, so a reporting failure returns before
replacement. Installers never call Reporter. EventComplete occurs only after a
healthy committed Install result and the deferred session cleanup attempt.
Backup-removal, unlock, and terminal-report errors return the successful Result
with Applied true and joined error details; none initiates rollback solely after
the commit point.

Every error wraps its operation and product while preserving errors.Is and
errors.As behavior. Tokens, checksum bodies, release notes, and filesystem
contents are never included in errors.

## 5. Phase 0 — Accept and Commit the Decision Artifacts

### Files

* mcplib/docs/0005-MADR-canonicalize-cli-self-update-in-mcplib.md
* mcplib/docs/0005-PLAN-canonicalize-cli-self-update-in-mcplib.md

### Steps

1. Apply reviewer edits.
2. Confirm the MADR remains accepted. Set the plan status to accepted and set
   its date to the plan-acceptance date. Change the MADR date again only if
   acceptance includes another decision change.
3. Confirm the plan filename exactly mirrors the MADR identifier and slug.
4. Run:

       git diff --check -- docs/0005-MADR-canonicalize-cli-self-update-in-mcplib.md docs/0005-PLAN-canonicalize-cli-self-update-in-mcplib.md
       rg -n "^status: accepted$|^associated-madr:" docs/0005-*

5. Commit the two documentation files. Do not push.

### Acceptance

Both complete artifacts are committed and link to one another; no source file
is changed.

## 6. Phase 1 — Add Core Types, Strict Versions, and Asset Selection to mcplib

### Files

New:

* selfupdate/doc.go
* selfupdate/types.go
* selfupdate/errors.go
* selfupdate/version.go
* selfupdate/version_test.go
* selfupdate/assets.go
* selfupdate/assets_test.go
* selfupdate/checksums.go
* selfupdate/checksums_test.go

Modified:

* go.mod
* go.sum

### Steps

1. Add x/mod v0.40.0 as a direct requirement. x/sys is added when its first
   imported OS-specific implementation lands in Phase 3.
2. Implement the Section 4 data types and sentinels with String methods for all
   public enums and package comments for every exported symbol.
3. Implement strict stable-tag validation as a shape check followed by
   semver.IsValid. Use semver.Compare only after validation.
4. Implement operation classification. Parse CurrentVersion only for
   ReleaseBuild; LocalBuild requires force before any replacement and does not
   receive lexical ordering.
5. Implement NewExactAssetSelector and canonical exact raw-binary naming.
   Reject OS/architecture pairs not present in its caller-supplied platform set
   rather than maintaining a global fleet matrix.
6. Implement the strict manifest parser and digest parser.
7. Add table tests for stable versions, leading zeroes, incomplete versions,
   uppercase V, prerelease, build metadata, illegal identifiers, local builds,
   local check versus local apply, same-version force, exact rollback, and lower
   latest.
8. Add asset tests for Windows extension placement, hyphenated product names,
   duplicate exact assets, prefix-only decoys, missing manifest, duplicate
   manifest, unsupported platform, invalid basename, and digest syntax.
9. Add manifest tests for GNU text/binary forms, CRLF, blank/comment lines,
   duplicate entries, malformed hex, extra fields, absolute paths, traversal,
   and overlong lines.

### Verification and commit

    gofmt -w selfupdate/*.go

Run `golint FILE` separately for every changed Go file, including test files,
then run:

    go mod tidy
    go test ./selfupdate
    go vet ./selfupdate
    make lint
    git diff --check

Commit after all commands pass. Do not push.

### Acceptance

The package compiles on Go 1.26.5; its pure policy tests have no network,
terminal, executable, or service dependency.

## 7. Phase 2 — Add the Hardened GitHub Source and Bounded Downloader

### Files

New:

* selfupdate/github.go
* selfupdate/github_test.go
* selfupdate/download.go
* selfupdate/download_test.go
* selfupdate/verify.go
* selfupdate/verify_test.go
* selfupdate/testdata/SHA256SUMS.valid
* selfupdate/testdata/SHA256SUMS.invalid

Modified:

* selfupdate/types.go
* selfupdate/errors.go

### Steps

1. Implement NewGitHubSource validation, a cloned reusable client, header
   policy, token precedence, and an origin-aware redirect hook.
2. Pin Accept to application/vnd.github+json and X-GitHub-Api-Version to
   2026-03-10. Require a non-empty product/version User-Agent.
3. Implement latest and exact endpoints with url.PathEscape and bounded JSON
   decoding that detects overflow and trailing JSON rather than silently
   truncating or accepting a second value. Close every response body on every
   path.
4. Return RateLimitError for HTTP 429 and for HTTP 403 when
   X-RateLimit-Remaining is zero or Retry-After is present. Preserve status,
   Retry-After, X-RateLimit-Remaining, and X-RateLimit-Reset in its typed
   fields; other non-success statuses return a bounded sanitized HTTP error.
5. Download assets through the GitHub asset API URL derived from the asset ID,
   using Accept application/octet-stream. Never authorize a metadata-provided
   browser URL.
6. Require uploaded state, positive advertised size, maximum size, and valid
   sha256 digest when supplied.
7. Implement bounded manifest fetch and a streaming executable downloader that
   writes to a caller-supplied io.Writer while computing SHA-256 and enforcing
   the advertised and hard sizes. The locked installation session added in
   Phase 3 owns file creation, sync, close, and cleanup; Phase 2 tests the
   downloader with bounded in-memory and temporary-file writers.
8. Test one reusable client, required headers, token order, no token on foreign
   redirect, 200 and 302 asset responses, cancellation, timeout, JSON overflow,
   manifest overflow, binary overflow, short and long body versus advertised
   size, digest mismatch, checksum mismatch, malformed metadata, draft,
   prerelease, mutable release, hostile control characters in bounded error
   diagnostics, and rate-limit fields.
9. Run the tests under httptest only; no live GitHub call belongs in unit tests.
10. Do not retry or sleep inside the source. Return typed status, rate-limit,
    and Retry-After data so the CLI fails promptly and a caller can decide when
    to invoke it again.

### Verification and commit

Run per-file gofmt and golint, then:

    go test ./selfupdate -run "GitHub|Download|Verif"
    go test -race ./selfupdate
    go vet ./...
    make lint
    git diff --check

Commit after all commands pass. Do not push.

### Acceptance

No unbounded remote body read exists, and an authorization header can reach
only the configured API origin.

### Deviation 2026-09-02 — file list narrower than planned

**Found.** Commit `7652cbf` added every planned file but modified only
`selfupdate/types.go`. `selfupdate/errors.go` needed no change because Phase 1
already declared every sentinel this phase returns.

**Decision.** Record the narrower list. No sentinel was added, renamed, or
removed, so the Section 4.1 frozen error set is unaffected.

## 8. Phase 3 — Add Target Policy, Locking, Replacement, and Installers

### Files

New:

* selfupdate/target.go
* selfupdate/target_test.go
* selfupdate/lock_unix.go
* selfupdate/lock_windows.go
* selfupdate/lock_test.go
* selfupdate/replace.go
* selfupdate/replace_unix.go
* selfupdate/replace_windows.go
* selfupdate/replace_test.go
* selfupdate/replace_native_test.go
* selfupdate/session.go
* selfupdate/session_test.go
* selfupdate/cleanup.go
* selfupdate/cleanup_test.go
* selfupdate/cleanup_windows.go
* selfupdate/cleanup_windows_test.go
* selfupdate/cleanup_other.go
* selfupdate/standalone.go
* selfupdate/standalone_test.go
* selfupdate/managed.go
* selfupdate/managed_test.go
* selfupdate/fs_test.go

Modified:

* selfupdate/types.go
* selfupdate/errors.go
* .github/workflows/ci.yml

### Steps

1. Implement home-root defaults with component-aware filepath.Rel checks.
   Canonicalize every allowed root, require it to exist as a directory, reject
   a filesystem root, and deduplicate it with platform-appropriate case rules.
   Reject empty, relative, root-directory, non-regular, detected symlink, or
   outside-root targets before network work. Lstat the raw absolute path from
   os.Executable or the explicit policy before EvalSymlinks; because
   os.Executable may already resolve an invocation symlink on some operating
   systems, document that no portable API can detect every symlink used to
   launch the process. Never combine a resolved directory with an unresolved
   basename.
2. Open an os.Root for the resolved target directory and use validated
   basenames for staging, backup, lock, rename, and cleanup.
3. Implement Installer.Begin and InstallSession. Begin acquires the lock,
   revalidates the exact Target returned before network access, compares its
   identity, and returns a session owning the lock and os.Root. Close is
   idempotent, removes registered staging material, closes the root, and
   releases the lock while joining all cleanup errors.
4. Create random staging through InstallSession.CreateStaging with os.CreateTemp
   in that directory and random backup names; never use .staging, .old, or
   .prev as a shared fixed path. Reject an artifact not registered to the same
   session.
5. Implement the stable advisory lock in OS-specific files and a bounded
   context-aware acquisition loop. Open Unix lock files with O_NOFOLLOW; reject
   a non-regular lock or a Windows reparse-point lock before LockFileEx.
6. Promote x/sys v0.47.0 to a direct requirement when the Unix and Windows lock
   and replacement files import it.
7. Implement Unix backup-without-gap, atomic rename-over-target, permissions,
   file sync, directory sync, rollback, and cleanup. Before replacement, chmod
   staging to the old target's FileMode.Perm value; do not copy setuid, setgid,
   or sticky bits. The random same-directory temp remains owned by the invoking
   user.
8. Implement Windows LockFileEx and MoveFileEx replacement with documented
   flags, unique backup, bounded retry, rollback, and joined errors. Classify
   only the native sharing violation for the renamed currently running image as
   PendingBackup; exclusively create and sync its exact-basename/digest cleanup
   receipt with a user-only Windows DACL.
9. At Begin, process only the exact cleanup receipt while holding the target
   lock. Validate schema, basename, regular-file/reparse state, and digest
   before removal; sync receipt creation/removal and reject malformed,
   mismatched, or undeletable state. Never discover backups with a glob.
10. Implement StandaloneInstaller as the sole owner of target resolution,
   session construction, and replacement.
11. Implement ManagedInstaller's absent/running/down matrix. Never install a
   missing service. Always start and health-check a previously installed service
   after update, including one that began down.
12. Add injected unexported filesystem operation seams for deterministic
   failures after every state transition without adding a public mock
   filesystem interface.
13. Require Reconciler implementations to return their restoration receipt even
    when Reconcile reports a partial-write error. ManagedInstaller calls Restore
    for any receipt indicating changed state, including the error return path.
14. Add native helper-process tests that copy an executing test binary to a
    temporary user-allowed root, replace it while running, and assert old-or-new
    recoverability on Linux, macOS, and Windows.
15. Expand mcplib CI to run go test on ubuntu-24.04, macos-15, and windows-2025.
    Retain Linux lint/vet and add gofmt plus go mod tidy-diff gates. Pin actions
    to reviewed commit SHAs: actions/checkout
    3d3c42e5aac5ba805825da76410c181273ba90b1 (v7.0.1) and
    actions/setup-go b7ad1dad31e06c5925ef5d2fc7ad053ef454303e
    (v7.0.0). Any new action is pinned and recorded before the phase commit.

### Required failure tests

* differently named symlink invocation;
* staging collision and attacker-created symlink;
* attacker-created lock symlink or Windows reparse point;
* two concurrent update attempts;
* target replacement between confirmation and lock acquisition;
* stale but unlocked lock file;
* stale backup;
* valid, malformed, digest-mismatched, symlink/reparse, and undeletable Windows
  pending-backup receipts;
* permission denial;
* injected short write, sync failure, close failure, rename failure, and
  directory-sync failure;
* cancellation before and after staging;
* rollback failure joined with the original;
* absent, running, and down service states;
* reconcile, start, health, definition-restore, binary-restore, and
  rollback-restart failures.

### Verification and commit

Run per-file gofmt and golint, then:

    go test ./selfupdate
    go test -race ./selfupdate
    GOOS=windows GOARCH=amd64 go test -c ./selfupdate
    go vet ./...
    make lint
    go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/ci.yml
    git diff --check

Delete the cross-compiled test binary after inspection. Commit only source,
tests, module files, and workflow changes. Do not push.

### Acceptance

Every apply failure has one tested rollback path; Windows comments describe
native guarantees without calling the two-step operation atomic. A successful
Windows update can retain only the reported, receipted running-image backup,
and the next Begin proves deterministic cleanup.

### Deviation 2026-09-02 — unlisted internal helper file

**Found.** Commit `7e3d803` added `selfupdate/close.go`, which no phase file
list names. It holds three unexported helpers shared by the session, cleanup,
and replacement code: `joinClose`, `joinRemove`, and `openAbsFile`. The same
commit also modified `selfupdate/assets.go` and `go.mod` — the x/sys v0.47.0
requirement anticipated by Section 1.4 step 1 — while `selfupdate/errors.go`
was again unchanged.

**Decision.** Keep the file and record it here. Every symbol is unexported, so
the Section 4 frozen public API is unaffected and no reviewed-plan change is
required. Treat `selfupdate/close.go` as part of this phase's Files list.

## 9. Phase 4 — Add Coordinator, Text UX, and Public Examples

### Files

New:

* selfupdate/updater.go
* selfupdate/updater_test.go
* selfupdate/reporter.go
* selfupdate/reporter_test.go
* selfupdate/confirmer.go
* selfupdate/confirmer_test.go
* selfupdate/transform.go
* selfupdate/example_test.go

Modified:

* selfupdate/doc.go
* selfupdate/types.go
* README.md

### Steps

1. Implement New and Run in the exact Section 4.6 order.
2. Make all collaborators immutable after construction. One Updater is safe for
   sequential reuse and has a non-blocking in-process run guard: overlapping
   Run calls on the same instance return ErrConcurrentUpdate before invoking a
   collaborator. Separate Updater instances and processes converge on the
   per-target filesystem lock.
3. Implement all Event kinds and stable plain-text rendering without ANSI
   escapes. Escape C0/C1 control characters and collapse remote CR/LF so a tag,
   URL, diagnostic, or detail cannot inject terminal lines or workflow commands.
   Progress writes return errors rather than being silently discarded.
4. Implement terminal confirmation through an injected os.File. Non-terminal
   input returns ErrConfirmationRequired with an instruction to pass --yes.
   Reuse mcplib's existing golang.org/x/term dependency and term.IsTerminal;
   do not infer interactivity only from os.ModeCharDevice.
5. Ensure yes bypasses only confirmation and decline is a successful result.
6. Implement no-op, upgrade, reinstall, rollback, local-build, and check
   results. Return ErrUpdateAvailable only in check mode. When PendingBackup is
   non-empty, EventComplete and text output identify the exact retained path and
   state that the next apply will validate and remove it before downloading.
7. Add a failure-boundary state-machine test table that records exact call order
   and asserts no later stage runs after a failure. Add an overlapping-Run test
   proving the in-process guard returns ErrConcurrentUpdate without invoking a
   collaborator.
8. Test partial writes, writer failure before Install, terminal writer failure
   after a healthy Install, verifier descriptor closure, a transformer that exceeds the
   size limit or replaces staging with a symlink/non-regular file,
   terminal-control injection, PendingBackup rendering, and cleanup/unlock
   failure at the post-health commit boundary.
9. Add examples for standalone CLI use, custom reporting, and managed service
   composition. Do not import Cobra in package or examples.
10. Document the trust guarantee as release-asset integrity, not publisher
   signature authenticity.

### Verification and commit

Run per-file gofmt and golint, then:

    go test ./...
    go test -race ./...
    go vet ./...
    make lint
    go mod tidy -diff
    git diff --check

Commit after all commands pass. Do not push.

### Acceptance

The full public API is usable without Cobra, a UI toolkit, a service manager, or
global standard streams. go list -deps for selfupdate contains no such package.

## 10. Phase 5 — Add the Shared Immutable Release Workflow

### Files

New:

* .github/workflows/publish-selfupdate-release.yml
* scripts/verify-selfupdate-release.sh
* scripts/verify-selfupdate-release_test.sh

Modified:

* .github/workflows/ci.yml
* selfupdate/doc.go
* README.md

### Reusable workflow inputs

The workflow_call contract is:

| Input | Type | Meaning |
|---|---|---|
| artifact-name | string | caller artifact containing the complete staged release |
| products-json | string | JSON string array of exact product basenames, for example `["mcremote","mcrelay"]` |
| platforms-json | string | JSON array of exact objects with only `os` and `arch`, for example `[{"os":"linux","arch":"amd64"},{"os":"windows","arch":"amd64"}]`; the validator adds `.exe` only for Windows |
| extra-assets-json | string | JSON string array of exact extra basenames such as installers and APKs; `[]` means none |
| bridge-release | boolean | permits the Magic Remote compatibility set only |

The caller grants contents: write, id-token: write, and attestations: write.
No custom secret is accepted; the caller's GITHUB_TOKEN is used.

### Steps

1. Check out `${{ job.workflow_repository }}` at
   `${{ job.workflow_sha }}` with persist-credentials false into
   `.mcplib-release-tools`. Those job-scoped values identify the exact called
   workflow commit; a normal checkout would instead select the caller
   repository, where mcplib's validator script is absent.
2. Download exactly artifact-name into a separate empty staging directory.
3. Require github.ref_type tag and a strict stable tag. Refuse an existing
   release, including a draft, so a rerun cannot silently alter its contents.
4. Derive the exact binary list from products-json and platforms-json. Require
   each once, require SHA256SUMS once, and reject every undeclared file.
5. Parse SHA256SUMS and require exactly one entry for every canonical binary and
   no entry for installer scripts or other extras.
6. For bridge-release, additionally require the exact Magic Remote compatibility
   filenames with the leading `v` removed from the suffix and exact
   `SHA256SUMS-0.16.0`; prove each compatibility binary is byte-identical to its
   canonical counterpart. Reject bridge-release unless the caller repository is
   `maccavelli/magic-cli-remote` and the tag is exactly `v0.16.0`.
7. Treat the completed G2 administrative immutability gate as a precondition of
   pushing the tag that triggers the caller. Do not query the immutable-releases
   settings endpoint with GITHUB_TOKEN: that endpoint requires repository
   Administration permission, which a normal called workflow cannot request.
8. Create a draft with gh release create --draft --verify-tag and generated
   notes. Upload the explicit file list without --clobber.
9. Generate provenance for the exact staged files with
   actions/attest-build-provenance pinned to commit
   96278af6caaf10aea03fd8d33a09a777ca52d62f.
10. Publish only after validation, upload, and attestation succeed.
11. Poll the published release's isImmutable field and `gh release verify` at
    five-second intervals for at most two minutes; fail unless both prove the
    release is immutable and its automatic release attestation is available.
    This bounded poll handles post-publication propagation only; it never
    uploads or edits. This is defense in depth—the authorized pre-tag G2 setting
    check is what prevents publication while the repository setting is disabled.
12. Put all file-set and manifest validation in
    `.mcplib-release-tools/scripts/verify-selfupdate-release.sh`. Do not
    maintain a second inline validator.
13. Pin actions/checkout to 3d3c42e5aac5ba805825da76410c181273ba90b1
    (v7.0.1), actions/download-artifact to
    3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c (v8.0.1), and
    actions/attest-build-provenance to
    96278af6caaf10aea03fd8d33a09a777ca52d62f (v3.2.0). These tag-to-commit
    mappings were verified with git ls-remote on 2026-09-02. Any other action
    introduced into this workflow must also use a verified full commit SHA.
14. Add scripts/verify-selfupdate-release_test.sh with valid, missing,
    duplicate, extra, malformed, and bridge artifact directories. It performs
    no publication.
15. Before creating a draft, require `gh release verify --help` and
    `gh release view --json isImmutable --help` to succeed. The reviewed local
    baseline is gh 2.98.0; a runner lacking these capabilities fails before it
    creates release state.

### Verification and commit

    go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/*.yml
    shellcheck scripts/verify-selfupdate-release.sh scripts/verify-selfupdate-release_test.sh
    ./scripts/verify-selfupdate-release_test.sh
    go test ./...
    go test -race ./...
    go vet ./...
    make lint
    git diff --check

Commit after all commands pass. Do not push.

### Acceptance

The shared workflow contains no --clobber path, cannot publish a partial
release, and rejects undeclared assets. Paired with the pre-tag G2
administrative check, it publishes only an immutable release. It ships in the
same mcplib release as the Go API, and every consumer call uses the exact commit
behind that tag.

## 11. Gate G1 — Publish mcplib v1.3.0

This gate is external and blocks all consumer dependency commits.

### Preconditions

* Phases 0-5 are committed and mcplib is clean.
* CI is green on Linux, macOS, and Windows.
* Public API documentation and examples match Section 4.
* The latest tag observed during planning was v1.2.0, and v1.3.0 does not
  already exist locally or remotely at execution time.

### Commands, only after same-turn authorization

    git fetch origin --tags
    git tag --list v1.3.0
    git ls-remote --tags origin refs/tags/v1.3.0
    git push origin main

Wait for the exact pushed main commit to pass the Linux, macOS, and Windows CI
jobs. Confirm that the remote main SHA equals the reviewed local HEAD. Only
then, under the same explicit publication authorization, run:

~~First push of `9c076e2` failed CI (run 33628586778).~~ See
[Deviation 2026-09-02 — G1 CI failures](#deviation-2026-09-02--g1-ci-failures).
A follow-up commit is required before tagging.

    test "$(git rev-parse HEAD)" = "$(git ls-remote origin refs/heads/main | awk '{print $1}')"
    git tag -a v1.3.0 -m "mcplib v1.3.0"
    git push origin refs/tags/v1.3.0

After tag CI succeeds:

    GOPROXY=https://proxy.golang.org go list -m github.com/maccavelli/mcplib@v1.3.0
    git rev-list -n 1 v1.3.0

Do not begin a consumer phase until the module command resolves v1.3.0. Record
the second command's exact 40-character commit as the mcplib workflow SHA in
Section 23. Every consumer calls the reusable workflow at that SHA, with a
`# v1.3.0` comment; no consumer calls it through a branch or movable tag.

### Rollback

Do not move or overwrite the tag. If a defect is found after publication,
revert on main and publish v1.3.1 after review.

### Deviation 2026-09-02 — G1 CI failures

**Found.** After pushing main at `9c076e22d1459e80fedc451a28f2f49cdbcec6a5`,
CI run [33628586778](https://github.com/maccavelli/mcplib/actions/runs/33628586778)
failed on ubuntu-24.04, macos-15, and windows-2025. `v1.3.0` was not created.

1. Pre-existing: `wizard.TestConfigureLLM_LocalProviderSkipsKey` failed with
   `unexpected Confirm("Try a different endpoint?")`. The same failure is on
   origin/main run 33268493827 (2026-08-29). The test scripts an Ollama
   endpoint at `http://localhost:11434` and does not answer the Confirm that
   `resolveBaseURL` issues when `ValidateOllamaURL` fails. GitHub runners have
   no Ollama; a developer machine with Ollama running does not take that
   branch, which is why the test passed locally. The sibling
   `TestConfigureLLM_NoModelsAndNoneEnteredErrors` uses the same endpoint.
2. New in Phase 3: Windows `File.Sync` on a directory returns
   `ERROR_ACCESS_DENIED`. `isUnsupportedSync` only treated `EINVAL`/`ENOENT`,
   so standalone and managed installs failed. The native helper was named
   `helper` without `.exe`, so `exec.Command` failed with
   `executable file not found`.
3. Follow-up on run 33629500170 (Linux/macOS green): Windows
   `TestInjectedRenameFailure` still passed because replacement used
   `MoveFileEx`, not the injected `os.Rename` seam. Native backup deletion
   of the still-running image returned `ERROR_ACCESS_DENIED`, not
   `ERROR_SHARING_VIOLATION`, so it was not classified as `PendingBackup`.

**Decision.** Fix all of the above. Do not skip or loosen tests. Wizard change
is test-only: script `Confirm` false so an unreachable endpoint continues.
Windows: treat directory `ERROR_ACCESS_DENIED` as unsupported sync;
`MoveFileEx` `WRITE_THROUGH` remains the durability path. Name the native
helper `helper.exe` on Windows. Inject `replacePath` so both Unix rename and
Windows `MoveFileEx` share one test seam. Treat backup-delete
`ERROR_ACCESS_DENIED` as the same running-image busy condition as a sharing
violation (`PendingBackup` + cleanup receipt).

**Files added to G1 scope.** `wizard/configure_test.go`;
`selfupdate/replace.go`; `selfupdate/replace_unix.go`;
`selfupdate/replace_windows.go`; `selfupdate/replace_test.go`;
`selfupdate/replace_windows_test.go`; `selfupdate/replace_native_test.go`;
`selfupdate/fs_test.go`; this PLAN.

## 12. Phase 6 — Migrate prepare-commit-msg as the Standalone Canary

### Files

New:

* update.go
* update_test.go

Modified:

* main.go
* main_test.go
* Makefile
* go.mod
* go.sum
* .github/workflows/ci.yml
* scripts/verify-release.sh
* README.md

Deleted after parity tests pass:

* internal/selfupdate/client.go
* internal/selfupdate/client_test.go
* internal/selfupdate/semver.go
* internal/selfupdate/semver_test.go
* internal/selfupdate/updater.go
* internal/selfupdate/updater_test.go
* internal/selfupdate/apply_unix.go
* internal/selfupdate/apply_windows.go
* any remaining tests whose only subject is that deleted package

### Steps

1. Pin mcplib v1.3.0 with go get and tidy. Do not use a replace directive.
2. Move stdlib flag binding to update.go. runUpdate accepts a caller context and
   args, rejects fs.NArg greater than zero, and returns selfupdate.Result plus
   error.
3. Bind check, force, version, yes, and y exactly once. Reject check plus yes and
   check plus force before source construction.
4. Add a string RawBuildKind linker variable defaulting to `local`. Local source
   builds are LocalBuild even if Version resembles a release. Tag builds stamp
   RawVersion from the tag and RawBuildKind=`release`; the binding maps only
   that exact value to ReleaseBuild. Go linker -X is not used on a bool.
5. Use the home-root standalone installer, stdout text reporter, terminal
   confirmer, 15-minute signal-aware caller context, and repository
   maccavelli/prepare-commit-msg.
6. In main, call selfupdate.ExitCode. Do not label ErrUpdateAvailable as
   "Update failed"; exit 10 silently after the normal availability report.
7. Verify plain update now prompts, yes is consumed, non-TTY without yes fails,
   exact lower version is reported as rollback, and check available exits 10.
8. Delete internal/selfupdate only after equivalent tests pass against the
   shared package.
9. Stamp RawBuildKind=release in all six Makefile build targets and extend
   verify-release.sh to execute one native binary and assert both version and
   release build-kind identity through an internal diagnostic test seam.
10. Replace the local publish job with the mcplib reusable workflow pinned at
    the exact G1 workflow SHA (`# v1.3.0`). Keep the existing exact six-binary
    build artifact and SHA256SUMS.

### CLI matrix

Test help; no args to update; extra args; each flag; both aliases; contradictory
pairs; TTY yes/no; non-TTY without yes; current; newer; exact older; exact same;
local build with and without force; unsupported platform; cancellation; and
exit 0, 10, and 1.

### Verification and commit

    make verify
    make verify-release VERSION=v1.2.0
    rg -n "internal/selfupdate|api.github.com/repos/.*/releases|func ParseSemver|func ReplaceExecutable" --glob '*.go' .
    git diff --check

The rg command must return no production implementation hit. Commit after all
checks pass. Stage only this phase's files, run `make verify-staged` against the
exact staged snapshot, and commit only if it passes. Do not push.

### Acceptance

The canary has one thin binding, no local release client/parser/downloader/
replacer, and all prior update features now obey the canonical contract.

### Deviation 2026-09-02 — modified test file name

**Found.** Commit `9f887d3b7b47` modified `main_extra2_test.go`, not the
`main_test.go` named in this phase's Modified list. The repository keeps its
main-package coverage in numbered extra test files; `main_test.go` needed no
change once the flag binding moved to `update.go`.

**Decision.** Record the actual file. The phase's CLI matrix is covered by the
new `update_test.go` (409 lines) plus the amended `main_extra2_test.go`; no
matrix case was dropped.

## 13. Phase 7 — Migrate Magic Remote and Create the Bridge

### Files

New:

* internal/updateclient/client.go
* internal/updateclient/client_test.go
* internal/updateclient/legacy.go
* internal/updateclient/legacy_test.go
* internal/updateclient/lifecycle.go
* internal/updateclient/lifecycle_test.go
* internal/updateclient/codesign_darwin.go
* internal/updateclient/codesign_other.go
* internal/updateclient/codesign_test.go

Modified:

* cmd/mcremote/main.go
* cmd/mcrelay/main.go
* internal/cli/update.go
* internal/cli/update_test.go
* internal/relay/update.go
* internal/relay/cli_test.go
* internal/cli/service/exec_refresher.go
* Makefile
* go.mod
* go.sum
* .github/workflows/ci.yml
* scripts/install.sh
* scripts/install.ps1
* scripts/install_test.sh
* scripts/build-apk.sh
* README.md
* docs/config.md

Deleted:

* internal/update/download.go and download_test.go
* internal/update/github.go and github_test.go
* internal/update/run.go and run_test.go
* internal/update/service.go
* internal/update/swap.go, swap_test.go, and swap_rollback_test.go
* internal/update/version.go and version_test.go
* scripts/next-build-version.sh
* scripts/next-build-version_test.sh

### Steps

1. Add mcplib v1.3.0 and no local replace.
2. Add --version to mcremote update and mcrelay update; add y as the shorthand
   for yes. Use cobra.NoArgs and the command's context with a 15-minute child
   timeout, never context.Background.
3. Make both command files call one internal/updateclient constructor. The
   adapter supplies product, repository, stamped identity, lifecycle, unit
   reconciler, streams, and optional Darwin codesign transformation only.
4. Adapt service.IsInstalled, IsActive, Stop, Start, and ExecRefresher to the
   context-bearing shared interfaces. Add bounded health verification instead
   of treating Start returning nil as sufficient.
5. Make reconcile failure fatal. Verify absent means binary-only, installed
   running means stop/reconcile/start/health, and installed down means
   reconcile/start/health.
6. Implement the consumer-local legacy version normalizer:
   BASE.N and BASE.N.gHASH normalize only to vBASE for comparison; a malformed
   value is LocalBuild. Do not export this from mcplib.
7. Do not add a legacy asset selector. The shared client rejects mutable
   pre-v0.16.0 releases even for an exact rollback. Return ErrMutableRelease
   with the release tag and an explanation that self-update cannot install it;
   the old client's only automated path is forward through the immutable
   v0.16.0 bridge.
8. Move ErrUpdateAvailable handling in both mains to selfupdate.ExitCode and
   delete imports of internal/update.
9. Replace the Makefile allocator with VERSION defaulting to a local identity.
   Tag CI passes the strict tag directly and stamps RawBuildKind=release. Local
   build, build-remote, build-relay, run, install, and APK paths create no tags,
   counter file, or remote mutation.
10. Keep Android versionCode on github.run_number. Set release APK versionName
    to the three-part tag. This changes release identity only; Android update
    behavior remains out of scope.
11. Stage v0.16.0 canonical exact binaries and SHA256SUMS plus byte-identical
    `<product>-<goos>-<goarch>-0.16.0[.exe]` compatibility copies and
    `SHA256SUMS-0.16.0`. The compatibility suffix deliberately omits the
    leading `v`: the legacy AssetFor returns that filename suffix and
    SumsAsset then requests the manifest with the identical suffix
    (`internal/update/github.go:81-128`). Include the existing APK and installer
    scripts as declared extra assets. Add a fixture that runs the unmodified
    legacy selector against the complete staged bridge asset list and proves it
    selects the compatibility binary, compatibility manifest, and matching
    checksum entry for every product/platform pair.
12. Update installer checksum lookup to prefer the exact canonical entry.
    Do not make the shared CLI updater read old versioned manifests.
13. Call the reusable release workflow at the exact G1 workflow SHA
    (`# mcplib v1.3.0`) with bridge-release true only when the tag equals
    v0.16.0; fail if true on any other tag.
14. Remove build-tag write permissions and every build/BASE.N fetch, allocation,
    summary, and assertion from CI.
15. Update current README and config docs. Leave historical MADRs as history;
    add a short supersession link where readers would otherwise follow obsolete
    BASE.N operating instructions.

### Verification and commit

    make pre-add-check FILES="cmd/mcremote/main.go cmd/mcrelay/main.go internal/cli/update.go internal/cli/update_test.go internal/relay/update.go internal/relay/cli_test.go internal/cli/service/exec_refresher.go internal/updateclient/client.go internal/updateclient/client_test.go internal/updateclient/legacy.go internal/updateclient/legacy_test.go internal/updateclient/lifecycle.go internal/updateclient/lifecycle_test.go internal/updateclient/codesign_darwin.go internal/updateclient/codesign_other.go internal/updateclient/codesign_test.go"
    go test ./internal/updateclient ./internal/cli ./internal/relay
    make race
    make verify-build-metadata
    ./scripts/install_test.sh
    go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/ci.yml
    rg -n "next-build-version|MCREMOTE_VERSION_PUSH|build/<BASE>|internal/update" Makefile scripts .github/workflows cmd internal README.md docs/config.md
    git diff --check

The rg output must contain only an intentional historical note or no hit; it
must not contain executable production paths. If implementation changes the
planned Go-file set, update this plan before proceeding. Stage only this phase,
rerun make pre-add-check with the same explicit list, then run
git commit --no-edit. Do not push.

### Acceptance

Both products share one adapter, exact version works, context cancels, service
rollback remains transactional, no build tag is created, and a legacy installed
binary can update through the v0.16.0 dual-name fixture.

## 14. Phase 8 — Add Recall Update Without MCP or Config Startup

### Files

New:

* cmd/mcp-server-recall/update.go
* cmd/mcp-server-recall/update_test.go

Modified:

* cmd/mcp-server-recall/root.go
* cmd/mcp-server-recall/root_test.go
* cmd/mcp-server-recall/main.go
* cmd/mcp-server-recall/main_test.go
* cmd/mcp-server-recall/version.go
* Makefile
* go.mod
* go.sum
* .github/workflows/ci.yml
* scripts/install.sh
* scripts/install.ps1
* scripts/install_test.sh
* README.md

### Steps

1. Pin mcplib v1.3.0.
2. Replace cobra.OnInitialize with a root PersistentPreRunE that initializes
   config for ordinary commands but skips commands annotated
   selfupdate.skip-config=true. Preserve any child pre-run by chaining rather
   than overwriting it.
3. Add update with cobra.NoArgs and the four canonical flags. Mark it
   skip-config, use cmd.Context with a 15-minute timeout, and route reporter and
   confirmation output to cmd.ErrOrStderr.
4. Use StandaloneInstaller only. Updating Recall must not create a service,
   initialize Viper, start fsnotify, open the datastore, or invoke serve.
5. Change Execute to return an error rather than exit. Keep the stdout-to-stderr
   protection, then let main print ordinary errors and map
   selfupdate.ExitCode. Check-available exits 10.
6. Default RawVersion to dev and RawBuildKind to `local`. Stamp the strict tag
   and RawBuildKind=release in every release target.
7. Add tests with an initConfig counter and ServeFunc sentinel proving update
   check/apply touches neither. Assert RealStdout remains unused by updater
   output and captured stdout is protocol-clean.
8. Update installers to verify the exact unversioned filename entry instead of
   discovering a version-suffixed checksum line. Report the installed binary's
   own version after replacement.
9. Stop creating suffixed publication copies and the second manifest. Add both
   installer scripts as declared extras to the build artifact and call the
   reusable release workflow at the exact G1 workflow SHA
   (`# mcplib v1.3.0`).

### Verification and commit

Run the repository's pre-commit wrapper against the exact changed Go list:

    scripts/go-precheck.sh cmd/mcp-server-recall/update.go cmd/mcp-server-recall/update_test.go cmd/mcp-server-recall/root.go cmd/mcp-server-recall/root_test.go cmd/mcp-server-recall/main.go cmd/mcp-server-recall/main_test.go cmd/mcp-server-recall/version.go

Then run:

    make test
    go test -race ./cmd/mcp-server-recall ./internal/config
    make vet
    make lint
    sh -n scripts/install.sh
    sh scripts/install_test.sh
    go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/ci.yml
    git diff --check

Stage only this phase's files, rerun `scripts/go-precheck.sh` with the same Go
list against the staged content through the repository hook, and commit after
all checks pass. Do not push.

### Acceptance

Recall exposes the canonical CLI, check available exits 10, stdout remains
JSON-RPC-clean, and config/datastore/server initialization counters remain zero.

## 15. Phase 9 — Add MagicTools Update With Managed Lifecycle

### Files

New:

* cmd/mcp-server-magictools/update.go
* cmd/mcp-server-magictools/update_test.go
* cmd/mcp-server-magictools/update_service.go
* cmd/mcp-server-magictools/update_service_test.go
* cmd/mcp-server-magictools/service_refresh.go
* cmd/mcp-server-magictools/service_refresh_test.go
* cmd/mcp-server-magictools/root_test.go

Modified:

* cmd/mcp-server-magictools/root.go
* cmd/mcp-server-magictools/main.go
* cmd/mcp-server-magictools/main_test.go
* cmd/mcp-server-magictools/version.go
* cmd/mcp-server-magictools/service.go
* cmd/mcp-server-magictools/service_windows.go
* cmd/mcp-server-magictools/service_other.go
* Makefile
* go.mod
* go.sum
* .github/workflows/ci.yml
* scripts/install.sh
* scripts/install.ps1
* scripts/install_test.sh
* README.md

### Steps

1. Pin mcplib v1.3.0 and add the canonical Cobra update command.
2. Make Execute return an error; use the existing exitFunc seam in main to map
   selfupdate.ExitCode. Do not call os.Exit in root.go.
3. Default RawVersion to dev and RawBuildKind to `local`, then stamp the strict
   tag plus RawBuildKind=release in release targets.
4. Construct a lifecycle adapter bound to the resolved target. Installed is
   true only when the OS service definition exists and targets this executable;
   a definition for another binary is treated as absent and never stopped.
5. Implement context-aware installed/running/stop/start operations using
   systemctl --user, launchctl's user domain, or x/sys/windows service manager.
   Reuse these primitives from existing service commands rather than keeping a
   second command-execution path.
6. Add an internal hidden service refresh operation that the newly installed
   binary can execute without starting the server. It rewrites only an existing
   definition, returns a typed JSON receipt, and never installs/enables a
   missing service.
7. Linux refresh writes and syncs the unit then daemon-reloads. macOS refresh
   validates and writes the plist without bootstrapping. Windows refresh
   snapshots mgr.Config plus the service environment registry value and updates
   the existing service configuration.
8. Reconciler invokes the new target's hidden refresh operation, captures the
   receipt in ReconcileResult.State, and restores from it during rollback.
9. WaitHealthy polls the service manager plus service.state PID for at most 30
   seconds with caller cancellation. Health timeout is fatal.
10. Prove update never calls ServeFunc, NewOrchestratorApp, db.NewStore, config
    loading, or stdout hijacking. Route all update human output to stderr.
11. Update installer checksum parsing and publication exactly as Recall does.
    Retain the existing three-platform matrix.
12. Replace the publish job with the reusable workflow at the exact G1
    workflow SHA (`# mcplib v1.3.0`).

### Verification and commit

Run gofmt and per-file golint, then:

    make test
    go test -race ./cmd/mcp-server-magictools
    make vet
    make lint
    sh -n scripts/install.sh
    sh scripts/install_test.sh
    go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/ci.yml
    git diff --check

Commit after all pass. Do not touch the owner's docs/0002 files and do not push.

### Acceptance

Standalone installs update without service calls. A matching installed service
obeys absent/running/down behavior, definition and binary rollback are tested,
and no update test initializes MCP or datastore state.

## 16. Phase 10 — Add Socratic Thinker Update Without Config or Runtime Startup

### Branch precondition

This phase is based on clean branch `ci/harden-release-pipeline` at
`f62221a76ece`, whose commits `b220dc8` and `f62221a` add the installer and
hardened workflow documented by local MADR/PLAN 0001. Do not overwrite or
recreate that work. At execution time, do exactly one of the following:

1. continue on that branch if it remains the maintainer's integration branch;
   or
2. if those commits have merged, switch to the clean intended base and re-read
   every file below before editing.

If neither condition is true, stop this phase and record the branch divergence
in Section 23.

**Resolved 2026-09-02.** Condition 2 now applies. `ci/harden-release-pipeline`
was merged to `origin/main` as a fast-forward at `3e4499b`, so this phase begins
from a clean `main` that already contains the hardened workflow, the
binary-only installers, MADR/PLAN 0001, and `go 1.26.6`. The repository has a
second remote (`gitlab`) on a divergent lineage using the internal mcplib; the
owner confirmed **GitHub is canonical**, so this phase targets `origin` only and
must not reintroduce the GitLab module path. The canonical immutable-release decision supersedes only local
MADR 0001's version-suffixed, dual-manifest, and `--clobber` publication
details; its native testing, installer, cgo, timeout, and action-pinning work is
retained.

### Files

New:

* cmd/mcp-server-socratic-thinker/update.go
* cmd/mcp-server-socratic-thinker/update_test.go
* cmd/mcp-server-socratic-thinker/main_test.go

Modified:

* cmd/mcp-server-socratic-thinker/root.go
* cmd/mcp-server-socratic-thinker/cmd_extra_test.go
* cmd/mcp-server-socratic-thinker/main.go
* cmd/mcp-server-socratic-thinker/version.go
* cmd/mcp-server-socratic-thinker/serve_extra_test.go
* Makefile
* go.mod
* go.sum
* .github/workflows/ci.yml
* scripts/install.sh
* scripts/install.ps1
* scripts/install_test.sh
* README.md
* docs/0001-MADR-port-magictools-ci-cd-pipeline.md
* docs/0001-PLAN-port-magictools-ci-cd-pipeline.md

### Steps

1. Pin mcplib v1.3.0 with no replace directive. Add a standalone canonical
   Cobra update command for repository `maccavelli/mcp-server-socratic-thinker`
   and the exact linux/amd64, darwin/arm64, and windows/amd64 platform set.
2. Replace cobra.OnInitialize with a root PersistentPreRunE that calls
   initConfig for ordinary commands but skips a command annotated
   `selfupdate.skip-config=true`. Preserve any command pre-run hook by explicit
   chaining. The update command must not call config.New, start fsnotify, create
   a Recall client, open the metrics BuntDB, start telemetry, or launch the
   dashboard or MCP server.
3. Change Execute to return rootCmd.Execute errors while preserving the existing
   default serve delegate. Keep stdout redirected to stderr for protocol safety,
   but let main map selfupdate.ExitCode so check-available exits 10. No package
   outside main may call os.Exit for update failures.
4. Default RawVersion to `dev`, retain Version only as the trimmed display
   value, and add string RawBuildKind=`local`. Every release Makefile target
   stamps the raw strict tag and RawBuildKind=release; local build/run/install
   paths remain LocalBuild even when `git describe` resembles a tag. Do not use
   linker -X on a bool.
5. Use cmd.Context with a 15-minute child timeout, StandaloneInstaller, stderr
   reporting/confirmation, Cobra NoArgs, and the four canonical flags. Use an
   injected updater-construction seam in tests; make no live GitHub call.
6. Add tests for the complete CLI matrix, exit 0/10/1, local-build force,
   context cancellation, config-init count zero, RealStdout unused, and
   sentinels proving serve/dashboard/Recall/metrics/telemetry paths are not
   invoked by check or apply.
7. Keep the hardening branch's exact three binaries and build-time SHA256SUMS.
   Remove release-time version-suffixed copies and the second manifest. Stage
   install.sh and install.ps1 as declared extras and invoke the reusable
   workflow at the exact G1 workflow SHA (`# mcplib v1.3.0`).
8. Change both installers to select the exact canonical platform filename in
   SHA256SUMS. Preserve their existing binary-only behavior, explicit version
   option, offline fixtures, install roots, and PowerShell WhatIf behavior.
9. Append dated supersession notes to local MADR/PLAN 0001 and set their status
   to superseded. Preserve their original rationale and execution history;
   identify mcplib MADR 0005 as superseding only release naming and mutable
   publication behavior.
10. Update README command, release, installer, exit-code, local-build, and
    package-manager guidance.

### Verification and commit

Run gofmt and per-file golint, then:

    make test
    go test -race ./cmd/mcp-server-socratic-thinker ./internal/config
    make vet
    make lint
    sh -n scripts/install.sh
    shellcheck -s sh scripts/install.sh scripts/install_test.sh
    sh scripts/install_test.sh
    go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/ci.yml
    rg -n -- "--clobber|SHA256SUMS-[${]?[A-Za-z_]|softprops/action-gh-release" .github scripts Makefile
    git diff --check

The rg command may match only quoted historical text in docs, not an executable
workflow or installer path. Stage only the files listed by this phase, rerun
the checks against the staged Go list, and commit. Do not push.

### Acceptance

Socratic Thinker exposes the canonical CLI and exact assets; update starts no
config watcher or runtime component; the hardened native tests and binary-only
installers remain; and no mutable or version-suffixed publication path remains.

## 17. Phase 11 — Add DuckDuckGo Update and Canonical Release Identity

### Files

New:

* cmd/mcp-server-duckduckgo/update.go
* cmd/mcp-server-duckduckgo/update_test.go
* cmd/mcp-server-duckduckgo/version.go
* cmd/mcp-server-duckduckgo/version_test.go
* cmd/mcp-server-duckduckgo/main_test.go

Modified:

* cmd/mcp-server-duckduckgo/root.go
* cmd/mcp-server-duckduckgo/root_test.go
* cmd/mcp-server-duckduckgo/main.go
* cmd/mcp-server-duckduckgo/serve_test.go
* Makefile
* go.mod
* go.sum
* .github/workflows/ci.yml
* README.md

### Steps

1. Pin mcplib v1.3.0 with no replace directive. Add a standalone canonical
   Cobra update command for repository `maccavelli/mcp-server-duckduckgo` and
   the exact linux/amd64, darwin/arm64, and windows/amd64 platform set.
2. Replace cobra.OnInitialize(config.Load) with a root PersistentPreRunE that
   calls a narrow loadConfig seam for ordinary commands and skips commands
   annotated `selfupdate.skip-config=true`. Update check/apply must not create
   the cache directory or config.yaml currently written by config.Load.
3. Add RawVersion=`dev`, Version display normalization, and string
   RawBuildKind=`local` in version.go. Assign RootCmd.Version. Change every tag
   build to pass the strict raw tag and RawBuildKind=release; local build/run/
   install paths remain LocalBuild. Remove the ineffective linker assumption
   that main.RawVersion exists without defining it, and do not use linker -X on
   a bool.
4. Make Execute return an error and let main map selfupdate.ExitCode. Preserve
   the existing stdout-to-stderr protection, route update output to stderr, use
   cmd.Context with a 15-minute child timeout, and reject positional arguments.
5. Use StandaloneInstaller only. Update must not acquire the serve singleton
   lock, create its data directory, open or seed BuntDB, construct the browser/
   search engine, register MCP tools, or start stdio transport.
6. Add the full CLI matrix and injected updater tests. Set a temporary user
   cache directory and assert check/apply leave it absent; replace serve RunE
   with a restored test sentinel and assert it remains uncalled; assert
   OriginalStdout receives no updater output.
7. Preserve the already-canonical exact three-binary plus SHA256SUMS artifact
   set. Make tag builds explicitly use VERSION="$GITHUB_REF_NAME", verify the
   native binary reports the trimmed tag and release build-kind identity, and reject
   every undeclared artifact.
8. Expand CI to the shared native Linux/macOS/Windows quality and update-smoke
   gates. Pin every action reference modified or added by this phase to a
   verified full commit SHA. Remove softprops/action-gh-release and call the
   reusable workflow at the exact G1 workflow SHA (`# mcplib v1.3.0`) with an
   empty extras list.
9. Update README with the update command, version behavior, exit codes,
   supported platforms, self-update ownership limits, and immutable release
   contract. Do not add installer scripts or service behavior in this phase.

### Verification and commit

Run gofmt and per-file golint, then:

    make test
    go test -race ./cmd/mcp-server-duckduckgo ./internal/config ./internal/store
    make vet
    make lint
    go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/ci.yml
    rg -n "softprops/action-gh-release|uses: .*@v[0-9]|api.github.com/repos/.*/releases" .github cmd internal
    git diff --check

The rg command must return no production or workflow hit. Stage only the files
listed by this phase, rerun checks against the staged Go list, and commit. Do
not push.

### Acceptance

DuckDuckGo gains a real stamped release identity and the canonical CLI; update
creates no config, datastore, browser, search, registry, transport, or server
state; and its exact three-platform assets publish only through the immutable
shared workflow.

## 18. Gate G2 — Enable Immutability and Roll Out Releases

Each repository is a separate stop-and-authorize gate. The latest local tags in
the Section 1 snapshot and proposed feature-release tags are:

| Repository | Latest observed local tag | Proposed release |
|---|---|---|
| prepare-commit-msg | v1.1.3 | v1.2.0 |
| magic-cli-remote | v0.15.3 | v0.16.0 bridge |
| mcp-server-recall | v2.0.0 | v2.1.0 |
| mcp-server-magictools | v1.0.3 | v1.1.0 |
| mcp-server-socratic-thinker | v1.0.2 | v1.1.0 |
| mcp-server-duckduckgo | v1.0.2 | v1.1.0 |

The gate fetches remote tags again before acting; any newly occupied proposed
tag stops execution and requires a reviewed replacement version.

### 18.1 Repository setting gate

Before pushing the first tag for each repository, and only with same-turn
authorization, use `gh auth status` to prove the active operator credential is
a user or GitHub App credential with repository Administration write access;
the Actions GITHUB_TOKEN is insufficient for this endpoint. Then enable and
verify immutable releases:

    gh api --method PUT -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: 2026-03-10" repos/maccavelli/REPOSITORY/immutable-releases
    gh api -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: 2026-03-10" repos/maccavelli/REPOSITORY/immutable-releases --jq ".enabled"

Expected result is true. Do not tag if the check fails.

### 18.2 Per-repository release gate

For one repository at a time:

1. Fetch and prove the proposed tag is absent locally and remotely.
2. Run that repository's complete verification contract.
3. Push the implementation branch only after explicit authorization.
4. Re-run Section 18.1's GET with the administrator credential immediately
   before tagging and require `.enabled == true`.
5. Create an annotated strict tag and push that exact ref only after explicit
   authorization.
6. Wait for build, native, shared publication, and attestation jobs.
7. Verify:

       gh release view TAG --json tagName,isDraft,isPrerelease,isImmutable
       gh release download TAG --dir TEMP_DIRECTORY
       (cd TEMP_DIRECTORY && sha256sum -c SHA256SUMS)
       if [ -f TEMP_DIRECTORY/SHA256SUMS-0.16.0 ]; then (cd TEMP_DIRECTORY && sha256sum -c SHA256SUMS-0.16.0); fi
       gh release verify TAG
       for file in TEMP_DIRECTORY/*; do gh release verify-asset TAG "$file"; done

8. Compare the downloaded file list byte-for-byte with the declared matrix.
   Magic v0.16.0 is the only release allowed declared bridge extras.
9. Never rerun publication against a completed tag. A fix receives a new patch.

### 18.3 Native update smoke

Run on Linux, macOS, and Windows runners. Create each executable fixture in an
exclusive random directory beneath the runner user's canonical home directory,
not the default system temporary directory, so the smoke exercises the same
default TargetPolicy shipped to users. Remove the fixture directory after the
process exits and all pending-backup assertions finish.

* Prepare: copy the previous published binary, run update --yes, and require the
  new version; then run the new binary with update --force --yes and require a
  successful verified reinstall.
* Magic Remote bridge: copy v0.15.3's legacy binary, run update --yes with no
  service definition, and require v0.16.0. Then use the v0.16.0 shared updater
  for a verified forced reinstall from the canonical exact asset. These are the
  critical old-selector and new-selector bridge proofs.
* Recall and MagicTools: copy the new release binary, run update --check and
  require 0; then run update --force --yes and require a successful verified
  reinstall without service initialization.
* Socratic Thinker and DuckDuckGo: copy the new release binary, run update
  --check and require 0; then run update --force --yes and require a verified
  reinstall while config, datastore, browser, dashboard, telemetry, and MCP
  startup sentinels remain untouched.
* All: use each repository's native smoke harness and injected loopback release
  source to serve a corrupt fixture manifest; require exit 1 with the original
  executable digest unchanged. Do not attempt to alter a live immutable
  release or add a production CLI flag for the test source.

Managed service update smoke is manual on one disposable host per supported
service manager because CI must not install persistent user services:

1. install the previous binary and service;
2. capture running state and definition digest;
3. update to the new exact tag;
4. require new version, installed definition, running health, and no stale
   backup; on Windows, an explicitly reported PendingBackup is allowed only
   while the updater process is still alive, and the next update invocation
   must consume its validated receipt before network access;
5. inject a health failure in a fixture build and require binary plus definition
   rollback.

## 19. Phase 12 — Deduplication and Completion Audit

### Immediate audit after all six repository migrations

Run from the common parent directory:

    rg -n "api.github.com/repos/.*/releases|SHA256SUMS|ParseSemver|ParseVersion|ReplaceExecutable|DownloadAndVerify|MoveFileEx" magic-cli-remote prepare-commit-msg mcp-server-recall mcp-server-magictools mcp-server-socratic-thinker mcp-server-duckduckgo --glob '*.go'

Allowed production hits:

* thin repository/product configuration;
* Magic Remote's installed-version normalizer and lifecycle/codesign adapters;
* Magic Remote and MagicTools service adapters;
* no GitHub client, general SemVer parser, checksum parser, downloader, lock, or
  executable replacer outside mcplib/selfupdate.

Also run:

    go list -m -f "{{.Path}} {{.Version}}" github.com/maccavelli/mcplib

in every consumer and require v1.3.0 or a reviewed later patch.

Record the exact search output, module versions, six consumer release URLs,
native-smoke results, and any allowed adapter hits in Section 23. Update this
plan's status to complete only when every Section 20 criterion is satisfied,
then run markdown and diff checks and commit that execution-record update in
mcplib. This documentation commit is the phase's only mutation.

### Deferred Magic cleanup is a separate follow-up plan

The initial shared-self-update plan does not remain open for the bridge support
window. Create and approve a new linked implementation plan before removing
bridge code. That plan may begin only when both conditions are true:

1. at least 90 days have elapsed since v0.16.0 publication; and
2. v0.17.0, the first canonical-only release, has passed native update smoke.

The follow-up plan must:

* delete internal/updateclient/legacy.go and its tests;
* remove the installed-version normalization path;
* publish only exact binaries and SHA256SUMS;
* retain generic extra-asset support in the shared workflow for installers/APK;
* update README and docs/config.md;
* run Magic's pre-add wrapper, race suite, installer tests, actionlint, and
  canonical-only release fixture;
* commit with git commit --no-edit and publish only under a new strict tag after
  separate authorization.

Do not delete old immutable release assets or tags.

## 20. Global Acceptance Criteria

The plan is complete only when all statements are true:

1. ~~mcplib/selfupdate builds and tests with Go 1.26.5.~~ Superseded
   2026-09-02 by MADR/PLAN 0006: builds and tests with Go 1.26.6.
2. Every migrated executable exposes:

       update [--check] [--force] [--version vX.Y.Z] [--yes|-y]

3. Current and declined return 0, actionable check returns 10, and every error
   returns 1.
4. Plain apply prompts; non-TTY apply without yes fails before download.
5. Exact lower versions are labeled rollback and require confirmation.
6. Local builds require force; force bypasses no security validation.
7. All GitHub API/body/token/redirect/rate-limit and limit tests pass.
8. All filesystem, concurrency, native replacement, and rollback tests pass.
   Windows tests additionally prove the pending running-image backup receipt is
   narrow, reported, digest-consistent, and consumed before the next download.
9. All managed lifecycle matrix and rollback tests pass.
10. Recall, MagicTools, Socratic Thinker, and DuckDuckGo each keep their
    stdout JSON-RPC-clean for `update` and initialize nothing more than the
    sentinels required to bind flags. The minimum proven set is:

    * Recall: no Viper, no fsnotify, no datastore open, no `serve.RunE`.
    * MagicTools: no `db.NewStore`, no `NewOrchestratorApp`, no stdout
      hijack, no `ServeFunc`.
    * Socratic Thinker: no `config.New`, no fsnotify, no Recall client, no
      metrics BuntDB, no telemetry, no dashboard, no `serve.RunE`.
    * DuckDuckGo: no `config.Load` (no cache directory, no `config.yaml`
      write), no serve singleton lock, no BuntDB open or seed, no browser
      or search engine, no MCP tool registration, no stdio transport.

    Each consumer's check and apply paths assert the relevant counters and
    sentinels remain at their zero or sentinel values.
11. Prepare yes is observably used; Magic commands propagate caller context.
12. No general updater implementation remains outside mcplib; consumer code is
    limited to version normalization, product configuration, lifecycle,
    reconciliation, and platform transformation adapters.
13. Consumer go.mod files pin a released mcplib version with no replace.
14. Every new release is strict SemVer, complete before publish, immutable,
    attested, and never uploaded with --clobber.
15. Magic v0.16.0 proves a legacy binary can cross the bridge.
16. The release file list matches each product's existing platform matrix.
17. Every mutating phase has one green commit per touched repository, every
    read-only gate/audit has recorded evidence, and no unapproved push or tag
    occurred.

## 21. Rollback and Recovery

### Before publication

Revert only the phase commit in the affected repository, rerun its complete
checks, and keep previously green mcplib phases. Never reset or discard owner
work.

### After mcplib v1.3.0

Do not move the module tag. Publish a compatible v1.3.1 fix. Consumers may pin
the fixed patch in a dedicated dependency-only commit after tests.

### After an immutable consumer release

Do not replace assets, disable immutability, or move the tag. Revert source,
publish a new patch release, and direct users to the new version. The runtime
installer's retained backup handles an individual failed apply; it is not a
substitute for a new release.

### Partial draft publication

The shared workflow leaves a failed release as a draft. It refuses to reuse it.
An operator must inspect the draft and its attestations, explicitly delete the
incomplete draft if appropriate, and rerun from the same immutable source tag.
No workflow automatically deletes release state.

### Post-publication verification timeout

Never rerun the publication job after a draft has been published. If the
workflow's bounded immutable-field or release-attestation poll times out, use
the G2 administrator credential to inspect the existing release and run
`gh release verify TAG` plus `gh release verify-asset TAG FILE` for every
downloaded declared asset. If the release is immutable and every asset verifies,
record the delayed propagation as a deviation and continue without mutation. If
the published release is mutable or any asset fails, stop rollout, preserve the
evidence, and prepare a reviewed new patch release; do not upload to, delete, or
reuse the affected tag automatically.

## 22. File Summary

| Repository | Added | Modified | Deleted |
|---|---|---|---|
| mcplib | selfupdate package/tests; reusable release workflow | go.mod, go.sum, CI, README, MADR/PLAN | none |
| prepare-commit-msg | thin update binding/tests | main, build/release files, module files, README | internal/selfupdate |
| magic-cli-remote | one shared product adapter and tests | both bindings/mains, service adapter, build/release/install/docs | internal/update; build allocator scripts |
| mcp-server-recall | update binding/tests | root/main/version, build/release/install/docs, module files | none |
| mcp-server-magictools | update and managed service adapters/tests | root/main/version/service, build/release/install/docs, module files | none |
| mcp-server-socratic-thinker | standalone update binding/tests | root/main/version, build/release/install/docs, module files | none |
| mcp-server-duckduckgo | standalone update/version bindings and tests | root/main/serve tests, build/release/docs, module files | none |

Exact generated go.sum changes are accepted only when explained by the pinned
mcplib, x/mod, and x/sys graph. No unrelated dependency upgrade belongs in
these commits.

## 23. Execution Record

Populate during approved implementation.

Rows 0 through 6 were backfilled on 2026-09-02, after the fact. They were left
at `pending` while the work landed, which breached Execution Contract rule 8;
the correction and its limits are described in the retrospective note below the
table. Rows from Phase 7 onward are populated as each phase executes.

| Phase or gate | Status | Commit/release | Verification evidence | Deviation |
|---|---|---|---|---|
| 0 Decision artifacts | complete | f5b7771, dc12218, fc2c94d, 5afe26c | PLAN status set to accepted at 5afe26c; MADR accepted; filenames mirror; baseline HEAD recorded as fc2c94db7817 | planning snapshot HEAD dc1221873b17 superseded by docs-only fc2c94d; no source anchors moved |
| 1 Core policy | complete | e21b997 | Added files match Section 6 exactly: 9 selfupdate files plus go.mod and go.sum; x/mod v0.40.0 added as a direct requirement | none |
| 2 GitHub/download | complete | 7652cbf | Added files match Section 7 exactly, including both SHA256SUMS fixtures; only types.go was modified | Deviation 2026-09-02 in Section 7 — errors.go needed no change |
| 3 Installers/native | complete | 7e3d803 | Added files match Section 8 plus the unlisted selfupdate/close.go; ci.yml moves validate to the ubuntu-24.04/macos-15/windows-2025 matrix and pins its actions to full SHAs; go.mod adds x/sys v0.47.0 | Deviation 2026-09-02 in Section 8 — unlisted internal helper file; assets.go and go.mod also modified |
| 4 Coordinator/UX | complete | 1fbf36a | Added and modified files match Section 9 exactly, including example_test.go and the README entry | none |
| 5 Shared release workflow | complete | 9c076e2 | Added files match Section 10 exactly: publish-selfupdate-release.yml, verify-selfupdate-release.sh, and its test harness; ci.yml, doc.go, and README modified | none |
| G1 mcplib v1.3.0 | complete | tag v1.3.0 at 3e64e3078c875a3dc3ffe235952be9f76c1ac787; https://github.com/maccavelli/mcplib/releases/tag/v1.3.0 | main CI 33629927085 green (ubuntu-24.04, macos-15, windows-2025); tag CI 33630084419 green; `GOPROXY=https://proxy.golang.org go list -m github.com/maccavelli/mcplib@v1.3.0` resolved. Reusable workflow SHA is 3e64e3078c875a3dc3ffe235952be9f76c1ac787 (`# v1.3.0`). | 2026-09-02: first push 9c076e2 failed (33628586778); wizard Ollama Confirm (pre-existing 33268493827); Windows dir sync ACCESS_DENIED, helper.exe, replacePath seam, backup-delete ACCESS_DENIED as PendingBackup. Fixes in 758209c and 3e64e30. |
| 6 Prepare canary | complete | prepare-commit-msg 9f887d3b7b47, authored 2026-09-02 07:48:42 -0500 | internal/selfupdate deleted (9 files, 1,363 lines); update.go and update_test.go added; go.mod pins github.com/maccavelli/mcplib v1.3.0 with no replace directive; ci.yml calls maccavelli/mcplib/.github/workflows/publish-selfupdate-release.yml@3e64e3078c875a3dc3ffe235952be9f76c1ac787 (`# v1.3.0`), the exact G1 workflow SHA. `GOTOOLCHAIN=auto go test ./...` green on 2026-09-02 across 5 packages, with internal/selfupdate absent from the package list | Deviation 2026-09-02 in Section 12 — main_extra2_test.go modified instead of main_test.go |
| 7 Magic bridge | complete | magic-cli-remote 45f8203 (migration) and 32510a6 (bridge, steps 11-13) | `internal/update` (2,225 lines) and the BASE.N allocator scripts deleted; `internal/updateclient` added with legacy normalizer, lifecycle, reconciler and codesign adapters plus tests; both products share one command body with `--version` and `-y`; mains map exit codes via `selfupdate.ExitCode`; Makefile and CI allocator removed, release builds stamp `BUILD_KIND=release`. Verified: `make pre-add-check` 17 files clean, `make race` green, `make verify-build-metadata` OK, `./scripts/install_test.sh` 135 passed / 0 failed, actionlint at parity with HEAD (two pre-existing shellcheck infos, unchanged), `go build`/`go vet`/`go test ./...` clean. `WaitHealthy` assertions proven to fail against a deliberately broken implementation via `go test -overlay`, tree untouched | the partial-phase deviation below is now closed; one contract change recorded: an ambiguous manifest fails closed |
| 8 Recall | complete | mcp-server-recall d80a55d | Standalone update on mcplib v1.4.0; scoped `PersistentPreRunE` replaces `cobra.OnInitialize`; `Execute` returns and main maps `ExitCode`; publication moved to the reusable workflow at `3389f793`. Verified: build/vet/test, `make lint` 0 issues, `go-precheck.sh` clean, `install_test.sh` 52 passed / 0 failed (up from 48), actionlint clean | `RawVersion` defaulted to `v2.0.0` — the current latest tag — so every local build claimed to be the newest release and self-update would never have fired; now `dev` + `RawBuildKind`. The installer shadowing test was tightened to fail closed on an ambiguous manifest, as in Phase 7 |
| 9 MagicTools | complete | mcp-server-magictools 91b5466 | Managed update on mcplib v1.4.0: lifecycle adapter bound to the resolved target, hidden `service refresh --json` with a typed receipt, fatal reconcile and fatal health timeout, exit mapping through the existing `exitFunc` seam, publication via the reusable workflow at `3389f793`. Verified: `make test`, `go test -race`, `make vet`, `make lint` 0 issues, gofmt/golint clean, `install_test.sh` 34 passed / 0 failed (up from 31), actionlint clean, linux/darwin/windows cross-build. The target-binding rule was proven to fail against a deliberately broken check via `go test -overlay`, tree untouched | `RawVersion` defaulted to `v4.3.2`, outranking every real tag; now `dev` + `RawBuildKind`. Steps 6-7's Windows refresh is deliberately a no-op on disk: the definition lives in the service control manager, so it is left untouched rather than guessed — recorded below |
| 10 Socratic Thinker | complete | mcp-server-socratic-thinker 067a438 (14 files, +570 / -67) | Standalone update command on mcplib v1.4.0; scoped `PersistentPreRunE` replaces `cobra.OnInitialize`; `Execute` returns and main maps `selfupdate.ExitCode`; `RawVersion` defaults to `dev` with `RawBuildKind`; publication moved to the reusable workflow at `3389f793` with exact assets only. Verified: `make test`, `go test -race`, `make vet`, `make lint` 0 issues, per-file `golint` clean, `install_test.sh` 35 passed / 0 failed (up from 31), actionlint clean, no `--clobber` or `softprops/action-gh-release` path remains. The config sentinel was proven to fail against the pre-0005 behaviour with `go test -overlay`, tree untouched | plan said pin v1.3.0; pinned v1.4.0 per the 0006 deviation. Two lint fixes in scope: extracted `skipConfigValue` (my second `"true"` literal tripped goconst) and documented the pre-existing `Cfg`/`RealStdout` vars in a file this phase modified |
| 11 DuckDuckGo | complete | mcp-server-duckduckgo 240ca3a | Standalone update on mcplib v1.4.0; `loadConfig` seam behind a scoped pre-run; `Execute` returns and main maps `ExitCode`; `softprops/action-gh-release` replaced by the reusable workflow at `3389f793`; every action pinned to a full SHA; tag build verifies its stamped identity. Verified: build/vet/test, gofmt and per-file golint clean, actionlint clean, no floating action tag or softprops path remains | `version.go` did not exist, so the Makefile's `-X main.RawVersion` had silently done nothing and the server had no version identity at all. Two floating action tags in the untouched test job were also pinned — a small consistency fix beyond the phase's literal file list |
| G2 product releases | 18.1 complete; 18.2 in progress (2 of 6 released) | no commit (repository settings) | Immutable releases enabled and verified on all six product repositories on 2026-09-02 via `PUT` then `GET repos/maccavelli/<repo>/immutable-releases` with `X-GitHub-Api-Version: 2026-03-10`, using the operator's own credential (`gh auth status`: account maccavelli, classic token, `repo` scope). Every repository read `enabled=false` before and `enabled=true` after, re-verified on a second independent read: prepare-commit-msg, magic-cli-remote, mcp-server-recall, mcp-server-magictools, mcp-server-socratic-thinker, mcp-server-duckduckgo. All six proposed tags confirmed absent on the remote at that moment. mcplib is deliberately NOT in scope: it is consumed as a Go module through the proxy, publishes no release assets, and its workflow integrity comes from the full-SHA pin | the endpoint and API version the MADR cited were confirmed to exist and respond as documented; the earlier caveat that they were unverified is resolved |
| 12 Deduplication/completion audit | pending | | | |

### Retrospective note on rows 0 through 6

Execution Contract rule 8 requires command output, commit IDs, and deviations to
be recorded in this section *during* execution. That did not happen for Phases 0
through 6: the rows stayed at `pending` while every one of those phases was
committed, mcplib v1.3.0 was tagged, and prepare-commit-msg was migrated. The
table above and the two Section 7/8 and one Section 12 deviation entries are a
2026-09-02 reconstruction from the repositories themselves.

What that reconstruction can and cannot establish:

* It establishes the commits, their exact file lists, the published tag and its
  SHA, the consumer's pinned module version and reusable-workflow SHA, and the
  three file-list variances now recorded as deviations.
* It does **not** recover the per-phase `go test`, `go vet`, `make lint`, or
  `git diff --check` output those phases were required to capture, nor
  prepare-commit-msg's `make verify`, `make verify-release`, and `rg` audit
  output. That evidence was never written down and cannot be reconstructed after
  the fact.
* What replaces it is weaker but real: the G1 CI runs 33629927085 and
  33630084419 exercised the cumulative Phase 1-5 tree green on ubuntu-24.04,
  macos-15, and windows-2025, and a 2026-09-02 retrospective run of
  `go build ./...`, `go vet ./selfupdate`, and `go test ./selfupdate` at
  a9e8ad4 passed (`ok github.com/maccavelli/mcplib/selfupdate 1.556s`), as did
  prepare-commit-msg's full suite at 9f887d3b7b47.

Section 20 criterion 17 is therefore satisfied for the *commit* requirement but
only partially for the *evidence* requirement on Phases 0 through 6. The Phase
12 audit must state that limitation rather than claim per-phase evidence exists.
Phases 7 onward record evidence as they run.

### Deviation 2026-09-02 — Windows service refresh is a documented no-op

**Found.** Phase 9 step 7 asks the Windows refresh to snapshot `mgr.Config`
plus the service environment registry value and update the existing service
configuration. Unlike Linux and macOS, Windows keeps no definition file on
disk, so there is nothing to render, back up and restore with the same
mechanism the other two use.

**Decision.** `refreshServiceDefinition` returns an explicit unchanged receipt
on Windows rather than a guessed `mgr.Config` mutation. An update there
replaces the binary and restarts the service; the service configuration is left
exactly as the operator installed it.

**Not done, and why.** The `mgr.Config` snapshot and restore path is not
implemented. Implementing it blind would create a rollback path that has never
run against a real Windows service control manager, and the plan's own managed
smoke tests are manual on a disposable host per Section 18.3. A wrong restore
there would corrupt a service definition rather than leave it alone. This is
recorded as outstanding for the Windows managed smoke rather than shipped
untested.

### Deviation 2026-09-02 — installer now fails closed on an ambiguous manifest

**Found.** Step 12 asked the installer to prefer the exact canonical checksum
entry. Making it simply prefer canonical broke
`scripts/install_test.sh`'s "alias line does not shadow the versioned line",
which appends a bogus canonical line to a legacy manifest and required the
install to still succeed. That test was guarding a real shadowing vector: once
canonical entries are legitimate, a preference in **either** direction lets an
appended line authorize a substituted binary.

**Decision.** Neither preference is safe, so the installer looks up both shapes
and refuses to choose: a manifest carrying a canonical *and* a versioned entry
for the same product/platform now exits 2 without installing. A conforming
release never contains both — `SHA256SUMS` lists canonical names only and the
bridge's `SHA256SUMS-0.16.0` lists compatibility names only.

**Test updated, not loosened.** The assertion changed from exit 0 to exit 2 and
was renamed to state the stronger guarantee, with a new case proving a
canonical-only manifest installs and an added check that an ambiguous manifest
leaves an existing install untouched. Suite: 139 passed / 0 failed, up from 135.

### Deviation 2026-09-02 — Phase 7 committed partially; steps 11-13 outstanding — CLOSED 2026-09-02

**Found.** Phase 7 bundles two separable bodies of work: the Go migration onto
`mcplib/selfupdate`, and the v0.16.0 release-pipeline bridge. The migration is
complete and the repository is green. The release-pipeline steps are not
started.

**Done.** Steps 1-10, 14 and 15: dependency pin, flags and context, the shared
adapter, lifecycle and reconcile adaptation with bounded health verification,
fatal reconcile policy, the consumer-local legacy normalizer, no legacy asset
selector, exit-code mapping, the Makefile allocator replacement, CI allocator
and write-permission removal, and the README/config documentation.

**Not done, and why.** Steps 11, 12 and 13 — staging the v0.16.0 dual-name
bridge assets with the legacy-selector fixture, preferring the canonical
checksum entry in the installers, and calling the mcplib reusable workflow with
`bridge-release` — are release-pipeline work whose only real proof is a
published release, which Gate G2 has not authorized. They were left whole
rather than half-built: a bridge fixture that has never run against a real
asset list is exactly the kind of check this plan's own rule says not to trust.

**Closed 2026-09-02 by commit 32510a6.** Steps 11, 12 and 13 are complete and
the consequence below no longer applies. Retained for the record.

**Consequence of stopping here — corrected 2026-09-02.** The original wording
here said a release would strand pre-v0.16.0 clients. Reading the release job
shows the opposite. It still renames every asset to
`<product>-<goos>-<goarch>-<VER>` and writes `SHA256SUMS-<VER>`, and `VER` is
now the bare tag base, so tagging v0.16.0 would publish exactly the
`mcremote-linux-amd64-0.16.0` and `SHA256SUMS-0.16.0` names the legacy selector
asks for: **old clients would work**.

The canonical half is what is broken. `SHA256SUMS` is a copy of
`SHA256SUMS-<VER>`, so it lists version-suffixed names, and the unversioned
aliases are deliberately excluded from it (`ci.yml:847`). The shared selector
picks `mcremote-linux-amd64`, looks for that exact basename in `SHA256SUMS`,
and fails with `ErrIntegrity`. **Every migrated client would fail.** Gate G2 for
this repository must not run until steps 11-13 are complete, in either
direction.

### Deviation 2026-09-02 — Go floor raised to 1.26.6 and consumers repin v1.4.0

**Found.** After Phase 6, `govulncheck` reported four reachable Go 1.26.5
standard-library advisories in mcplib itself, all fixed in go1.26.6 and all in
packages `selfupdate` depends on (`net/url`, `crypto/tls`, `encoding/asn1`,
`net/http`). The condition is pre-existing and fleet-wide: `make vuln` failed on
mcplib's unmodified tree, and fourteen of the workspace's fifteen modules
declared `go 1.26.5`.

**Decision.** Raise the floor rather than filter the check. Recorded as
[MADR 0006](0006-MADR-raise-go-toolchain-floor-to-1-26-6.md) and
[PLAN 0006](0006-PLAN-raise-go-toolchain-floor-to-1-26-6.md).

**Effect on this plan.**

* Global Acceptance criterion 1 becomes Go 1.26.6, struck through above.
* Section 1.1's Go-directive column is a superseded snapshot, annotated above.
* Remaining consumer phases 7-11 pin **mcplib v1.4.0**, not v1.3.0. Every
  occurrence of "Pin mcplib v1.3.0" in Sections 13 through 17 is read as
  v1.4.0; the no-replace-directive rule and the reusable-workflow SHA pin are
  unchanged, and the workflow SHA remains the v1.3.0 commit
  `3e64e3078c875a3dc3ffe235952be9f76c1ac787` unless PLAN 0006's Gate G1 moves
  it.
* Phase 6's prepare-commit-msg pin of v1.3.0 is bumped to v1.4.0 by PLAN 0006
  Phase 6, not by re-opening this plan's Phase 6.
* mcplib v1.3.0 is not rebuilt or replaced; it stays published and immutable.

Deferred Magic bridge cleanup is intentionally not an execution-record row; it
requires the separately approved follow-up plan described in Section 19.
