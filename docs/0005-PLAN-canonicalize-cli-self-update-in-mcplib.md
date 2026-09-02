---
status: proposed
date: 2026-09-01
associated-madr: 0005-MADR-canonicalize-cli-self-update-in-mcplib.md
decision-makers: mcplib maintainers
---

# Implementation Plan: Canonicalize CLI Self-Update in mcplib

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
4. End every implementation phase with a green repository and one commit in
   each repository touched by that phase.
5. Do not push, create or move a tag, publish a release, enable release
   immutability, or open a pull request without explicit authorization in the
   same turn. Plan approval alone is not that authorization.
6. In magic-cli-remote, run the repository wrapper
   make pre-add-check with the phase's explicit changed-Go-file list before staging,
   run make race before
   the phase commit, and invoke git commit --no-edit. Do not pass -m.
7. In the other repositories, run gofmt and per-file golint for every changed
   Go file before staging, then the repository checks named by the phase.
8. Record command output, commit IDs, release URLs, deviations, and rollback
   actions in Section 23 during execution. Do not silently widen a phase.

No datastore migration, configuration migration, automatic background polling,
or Android updater change is part of this plan.

## 1. Verified Baseline

### 1.1 Checkout snapshot

The plan was written against the following read-only snapshot:

| Repository | Branch | HEAD | Go directive | mcplib dependency | Working-tree note |
|---|---|---|---|---|---|
| mcplib | main | f5b7771dc8c6 | 1.26.5 | self | accepted MADR/PLAN 0005 committed; scope-extension edit only |
| magic-cli-remote | master | 4ab9d63268df | 1.26.5 | none | clean; newer commit changes only mobile notification files and MADR 0129 |
| prepare-commit-msg | main | 79cdba965289 | 1.26.6 | v1.2.0 | clean |
| mcp-server-recall | main | e22b9adf8c43 | 1.26.5 | v1.2.0 | clean |
| mcp-server-magictools | main | c672a720f8c3 | 1.26.5 | v1.2.0 | owner MADR/PLAN 0002 files present |
| mcp-server-socratic-thinker | ci/harden-release-pipeline | f62221a76ece | 1.26.5 | v1.2.0 | clean; two committed CI-hardening changes not on main |
| mcp-server-duckduckgo | main | 9a312807504f | 1.26.5 | v1.2.0 | clean |

The local toolchain was go1.26.5 darwin/arm64 except that prepare-commit-msg
selected go1.26.6 from its module directive.

Before changing a repository, run:

    git rev-parse --short=12 HEAD
    git status --short
    go version
    go env GOMOD GOOS GOARCH

If an anchor in this plan moved, re-read the affected function and update this
plan before implementing rather than applying a stale mechanical edit.

### 1.2 Existing behavior and measured duplication

The MADR's targeted baseline tests passed:

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

Version selection for the new module dependencies is fixed as of 2026-09-01:
golang.org/x/mod v0.40.0 and golang.org/x/sys v0.47.0. Both declare Go 1.25
and therefore remain compatible with the Go 1.26.5 floor.

## 2. Goal, Scope, and Non-goals

### Goal

Ship one public github.com/maccavelli/mcplib/selfupdate implementation and one
reusable release-publication workflow, migrate the two existing commands, add
commands to Recall, MagicTools, Socratic Thinker, and DuckDuckGo, and make all
six consumers' observable CLI behavior and release contract identical except
for product lifecycle and supported platform data.

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

These values close the review questions in MADR 0005 for this implementation.
Changing one requires editing the MADR and this plan before execution.

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
   add an explicit absolute user-owned root. The standard commands do not add
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
15. Reporter and cleanup failures: an event-reporting error before replacement
    starts aborts the operation. After replacement starts, retain the first
    reporting error but finish reconcile and health or their rollback path. If
    the installation becomes healthy, return Applied true plus joined reporting,
    backup-cleanup, or unlock errors; those post-commit errors do not roll back
    an otherwise healthy installation.

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
    }

    var ErrUpdateAvailable error
    var ErrConfirmationRequired error
    var ErrConcurrentUpdate error
    var ErrManagedInstall error
    var ErrUnsupportedPlatform error
    var ErrIntegrity error
    var ErrMutableRelease error
    var ErrRateLimited error

    func ExitCode(result Result, err error) int

ExitCode returns 10 only when ErrUpdateAvailable is in the error chain, 1 for
all other errors, and 0 otherwise. Run returns Result plus
ErrUpdateAvailable when check mode finds any different explicitly actionable
target. The library never calls os.Exit.

Request validation requires product to match
`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$` and rejects an unknown build kind,
partial platform values, positional arguments at the consumer boundary, check
plus yes, and check plus force. A non-zero platform must exactly match an entry
supplied to the selector; OS and architecture strings are never interpolated
without that allow-list match.

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
        StagedPath    string
        Size          int64
        SHA256        string
        ManifestSHA256 string
        GitHubSHA256  string
    }

    type Verifier interface {
        Verify(context.Context, Verification) error
    }

    type StagedArtifact struct {
        Path          string
        Size          int64
        ReleaseDigest string
        InstalledDigest string
    }

    type Transformer interface {
        Transform(context.Context, StagedArtifact) (StagedArtifact, error)
    }

The built-in verifier strictly parses SHA256SUMS: exactly two fields, optional
binary marker only, 64 lowercase-normalized hex characters, basename equal to
the selected ManifestName, no duplicate filename, and no path traversal.
Additional verifiers run after built-in integrity succeeds. The default
transformer is a no-op. Magic Remote's Darwin-only codesign transformer is
consumer code and must recompute InstalledDigest after codesign --verify
--strict succeeds.

### 4.4 Target and installer contracts

    type TargetPolicy struct {
        ExecutablePath string
        AllowedRoots   []string
        AllowSymlink   bool
    }

    type Target struct {
        Path string
        Dir  string
        Base string
    }

    type InstallRequest struct {
        Product  string
        Target   Target
        Artifact StagedArtifact
    }

    type InstallResult struct {
        Target             string
        Backup             string
        ServiceInstalled   bool
        ServiceWasRunning  bool
    }

    type Installer interface {
        ResolveTarget(context.Context) (Target, error)
        Install(context.Context, InstallRequest) (InstallResult, error)
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

    func NewStandaloneInstaller(InstallOptions) (*StandaloneInstaller, error)
    func NewManagedInstaller(*StandaloneInstaller, Lifecycle, Reconciler) (*ManagedInstaller, error)

Target resolution occurs before network work. Apply mode re-resolves and
revalidates after acquiring the lock. The stable lock file is a dedicated
basename beside the executable. Unix uses an advisory x/sys/unix lock; Windows
uses LockFileEx. A second process times out with ErrConcurrentUpdate and never
touches staging, backup, or service state.

Unix apply creates a random same-directory backup link or synced copy while
the old target remains live, renames the synced staging file over the target,
syncs the directory where supported, and retains the backup until lifecycle
health succeeds. Windows uses unique same-directory backup and staging names,
MoveFileEx with REPLACE_EXISTING and WRITE_THROUGH, retries only documented
transient sharing failures within the caller deadline, and restores the backup
if installing the new name fails.

### 4.5 Reporting, confirmation, construction, and limits

    type EventKind uint8

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

New rejects nil mandatory collaborators and invalid limits. Consumers compose
NewGitHubSource, NewStrictVersionPolicy, NewExactAssetSelector, the installer,
and the text adapters explicitly; v1.3.0 does not add a second convenience
constructor that could hide security-relevant defaults.

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
8. Acquire the target lock; re-resolve and revalidate the target beneath the
   same allowed root. Compare the initial and locked file identities with
   os.SameFile plus size and modification time; return ErrConcurrentUpdate if
   the target changed while metadata or confirmation was in progress.
9. Download the exact manifest within its limit and parse the one required
   entry.
10. Stream the binary through a limit reader and SHA-256 hasher into an
    os.CreateTemp file in the target directory; sync and close it.
11. Require actual size to equal API size and each available digest to agree.
12. Run additional verifiers, then the optional post-verification transformer.
13. Snapshot managed lifecycle, stop only an installed service, apply the
    binary while retaining rollback material, reconcile, start, and wait healthy.
14. Once a managed service has been stopped, any later failure enters one
    recovery path. Restore the definition if reconcile began, restore the
    binary if replacement began, start and health-check the prior binary when
    the service was installed, and join every recovery error with the original.
15. On success, remove backup and staging names, sync where supported, release
    the lock, emit the terminal event, and return Applied true.

Every Reporter call is checked. A reporting failure before replacement starts
returns immediately. After replacement starts, record the first reporting error
and continue the required lifecycle or rollback work. Once the replacement and
lifecycle are healthy, backup-removal, unlock, and reporting errors return the
successful Result with Applied true and joined error details; none initiates
rollback solely after the commit point.

Every error wraps its operation and product while preserving errors.Is and
errors.As behavior. Tokens, checksum bodies, release notes, and filesystem
contents are never included in errors.

## 5. Phase 0 — Accept and Commit the Decision Artifacts

### Files

* mcplib/docs/0005-MADR-canonicalize-cli-self-update-in-mcplib.md
* mcplib/docs/0005-PLAN-canonicalize-cli-self-update-in-mcplib.md

### Steps

1. Apply reviewer edits.
2. Set both frontmatter status values to accepted and retain date 2026-09-01
   unless acceptance occurs on a later date, in which case update both dates.
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

    gofmt -w selfupdate
    golint selfupdate/doc.go
    golint selfupdate/types.go
    golint selfupdate/errors.go
    golint selfupdate/version.go
    golint selfupdate/assets.go
    golint selfupdate/checksums.go
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
4. Preserve status, Retry-After, X-RateLimit-Remaining, and
   X-RateLimit-Reset in a typed RateLimitError.
5. Download assets through the GitHub asset API URL derived from the asset ID,
   using Accept application/octet-stream. Never authorize a metadata-provided
   browser URL.
6. Require uploaded state, positive advertised size, maximum size, and valid
   sha256 digest when supplied.
7. Implement bounded manifest fetch, streaming executable write, SHA-256,
   exact advertised-size validation, File.Sync, Close, and cleanup.
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
   Reject empty, relative, root-directory, non-regular, detected symlink, or
   outside-root targets before network work. Lstat the raw absolute path from
   os.Executable or the explicit policy before EvalSymlinks; because
   os.Executable may already resolve an invocation symlink on some operating
   systems, document that no portable API can detect every symlink used to
   launch the process. Never combine a resolved directory with an unresolved
   basename.
2. Open an os.Root for the resolved target directory and use validated
   basenames for staging, backup, lock, rename, and cleanup.
3. Create random staging with os.CreateTemp in that directory and random backup
   names; never use .staging, .old, or .prev as a shared fixed path.
4. Implement the stable advisory lock in OS-specific files and a bounded
   context-aware acquisition loop.
5. Promote x/sys v0.47.0 to a direct requirement when the Unix and Windows lock
   and replacement files import it.
6. Implement Unix backup-without-gap, atomic rename-over-target, permissions,
   file sync, directory sync, rollback, and cleanup. Before replacement, chmod
   staging to the old target's FileMode.Perm value; do not copy setuid, setgid,
   or sticky bits. The random same-directory temp remains owned by the invoking
   user.
7. Implement Windows LockFileEx and MoveFileEx replacement with documented
   flags, unique backup, bounded retry, rollback, and joined errors.
8. Implement StandaloneInstaller as the sole owner of the replacer and its
   ResolveTarget method.
9. Implement ManagedInstaller's absent/running/down matrix. Never install a
   missing service. Always start and health-check a previously installed service
   after update, including one that began down.
10. Add injected unexported filesystem operation seams for deterministic
   failures after every state transition without adding a public mock
   filesystem interface.
11. Require Reconciler implementations to return their restoration receipt even
    when Reconcile reports a partial-write error. ManagedInstaller calls Restore
    for any receipt indicating changed state, including the error return path.
12. Add native helper-process tests that copy an executing test binary to a
    temporary user-allowed root, replace it while running, and assert old-or-new
    recoverability on Linux, macOS, and Windows.
13. Expand mcplib CI to run go test on ubuntu-24.04, macos-15, and windows-2025.
    Retain Linux lint/vet and add gofmt plus go mod tidy-diff gates. Pin actions
    to reviewed commit SHAs.

### Required failure tests

* differently named symlink invocation;
* staging collision and attacker-created symlink;
* two concurrent update attempts;
* target replacement between confirmation and lock acquisition;
* stale but unlocked lock file;
* stale backup;
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
native guarantees without calling the two-step operation atomic.

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
2. Make all collaborators immutable after construction so one Updater is safe
   for sequential reuse; document whether concurrent Run calls are supported.
   The initial contract rejects concurrent runs on one target through the lock.
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
   results. Return ErrUpdateAvailable only in check mode.
7. Add a failure-boundary state-machine test table that records exact call order
   and asserts no later stage runs after a failure.
8. Test partial writes, writer failure before replacement, writer failure after
   replacement, terminal-control injection, and cleanup/unlock failure at the
   post-health commit boundary.
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
| products-json | string | exact executable basenames; one normally, two for Magic Remote |
| platforms-json | string | exact OS-architecture suffixes, including Windows extension rule |
| extra-assets-json | string | explicit installers, APKs, and temporary bridge files |
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
   filenames and SHA256SUMS-tag-version and prove each compatibility binary is
   byte-identical to its canonical counterpart.
7. Query the caller repository's immutable-releases endpoint and require
   enabled true before creating any release state.
8. Create a draft with gh release create --draft --verify-tag and generated
   notes. Upload the explicit file list without --clobber.
9. Generate provenance for the exact staged files with
   actions/attest-build-provenance pinned to commit
   977bb373ede98d70efdf65b84cb5f73e068dcc2a.
10. Publish only after validation, upload, and attestation succeed.
11. Query the published release's isImmutable field and fail unless true.
12. Put all file-set and manifest validation in
    `.mcplib-release-tools/scripts/verify-selfupdate-release.sh`. Do not
    maintain a second inline validator.
13. Pin actions/checkout to d23441a48e516b6c34aea4fa41551a30e30af803
    (v6), actions/download-artifact to
    37930b1c2abaa49bbe596cd826c3c89aef350131 (v7), and
    actions/attest-build-provenance to the v3 commit above. Any other action
    introduced into this workflow must also use a verified full commit SHA.
14. Add scripts/verify-selfupdate-release_test.sh with valid, missing,
    duplicate, extra, malformed, and bridge artifact directories. It performs
    no publication.

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
release, and rejects undeclared assets. It ships in the same mcplib release as
the Go API, and every consumer call uses the exact commit behind that tag.

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
    git tag -a v1.3.0 -m "mcplib v1.3.0"
    git push origin refs/tags/v1.3.0

After CI succeeds:

    GOPROXY=https://proxy.golang.org go list -m github.com/maccavelli/mcplib@v1.3.0
    git rev-list -n 1 v1.3.0

Do not begin a consumer phase until the module command resolves v1.3.0. Record
the second command's exact 40-character commit as the mcplib workflow SHA in
Section 23. Every consumer calls the reusable workflow at that SHA, with a
`# v1.3.0` comment; no consumer calls it through a branch or movable tag.

### Rollback

Do not move or overwrite the tag. If a defect is found after publication,
revert on main and publish v1.3.1 after review.

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
checks pass. Do not push.

### Acceptance

The canary has one thin binding, no local release client/parser/downloader/
replacer, and all prior update features now obey the canonical contract.

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
    product-platform-v0.16.0 compatibility copies and SHA256SUMS-0.16.0.
    Include the existing APK and installer scripts as declared extra assets.
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

Run gofmt and per-file golint, then:

    make test
    go test -race ./cmd/mcp-server-recall ./internal/config
    make vet
    make lint
    sh -n scripts/install.sh
    sh scripts/install_test.sh
    go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/ci.yml
    git diff --check

Commit after all pass. Do not push.

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

Modified:

* cmd/mcp-server-magictools/root.go
* cmd/mcp-server-magictools/root_test.go
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
in Section 23. The canonical immutable-release decision supersedes only local
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
authorization, enable and verify immutable releases:

    gh api --method PUT -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: 2026-03-10" repos/maccavelli/REPOSITORY/immutable-releases
    gh api -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: 2026-03-10" repos/maccavelli/REPOSITORY/immutable-releases --jq ".enabled"

Expected result is true. Do not tag if the check fails.

### 18.2 Per-repository release gate

For one repository at a time:

1. Fetch and prove the proposed tag is absent locally and remotely.
2. Run that repository's complete verification contract.
3. Push the implementation branch only after explicit authorization.
4. Create an annotated strict tag and push that exact ref only after explicit
   authorization.
5. Wait for build, native, shared publication, and attestation jobs.
6. Verify:

       gh release view TAG --json tagName,isDraft,isPrerelease,isImmutable
       gh release download TAG --dir TEMP_DIRECTORY
       (cd TEMP_DIRECTORY && sha256sum -c SHA256SUMS)
       gh release verify TAG

7. Compare the downloaded file list byte-for-byte with the declared matrix.
   Magic v0.16.0 is the only release allowed declared bridge extras.
8. Never rerun publication against a completed tag. A fix receives a new patch.

### 18.3 Native update smoke

Run on Linux, macOS, and Windows runners:

* Prepare: copy the previous published binary, run update --yes, and require the
  new version.
* Magic Remote bridge: copy v0.15.3's legacy binary, run update --yes with no
  service definition, and require v0.16.0. This is the critical bridge proof.
* Recall and MagicTools: copy the new release binary, run update --check and
  require 0; then run update --force --yes and require a successful verified
  reinstall without service initialization.
* Socratic Thinker and DuckDuckGo: copy the new release binary, run update
  --check and require 0; then run update --force --yes and require a verified
  reinstall while config, datastore, browser, dashboard, telemetry, and MCP
  startup sentinels remain untouched.
* All: corrupt a fixture manifest and require exit 1 with the original
  executable digest unchanged.

Managed service update smoke is manual on one disposable host per supported
service manager because CI must not install persistent user services:

1. install the previous binary and service;
2. capture running state and definition digest;
3. update to the new exact tag;
4. require new version, installed definition, running health, and no stale
   backup;
5. inject a health failure in a fixture build and require binary plus definition
   rollback.

## 19. Phase 12 — Deduplication Audit and Legacy Removal

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

### Deferred Magic cleanup trigger

Do not remove bridge support until both conditions are true:

1. at least 90 days have elapsed since v0.16.0 publication; and
2. v0.17.0, the first canonical-only release, has passed native update smoke.

At that point create a small follow-up phase:

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

1. mcplib/selfupdate builds and tests with Go 1.26.5.
2. Every migrated executable exposes:

       update [--check] [--force] [--version vX.Y.Z] [--yes|-y]

3. Current and declined return 0, actionable check returns 10, and every error
   returns 1.
4. Plain apply prompts; non-TTY apply without yes fails before download.
5. Exact lower versions are labeled rollback and require confirmation.
6. Local builds require force; force bypasses no security validation.
7. All GitHub API/body/token/redirect/rate-limit and limit tests pass.
8. All filesystem, concurrency, native replacement, and rollback tests pass.
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
17. Every phase has one green commit per touched repository and no unapproved
    push or tag occurred.

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

| Phase or gate | Status | Commit/release | Verification evidence | Deviation |
|---|---|---|---|---|
| 0 Decision artifacts | pending | | | |
| 1 Core policy | pending | | | |
| 2 GitHub/download | pending | | | |
| 3 Installers/native | pending | | | |
| 4 Coordinator/UX | pending | | | |
| 5 Shared release workflow | pending | | | |
| G1 mcplib v1.3.0 | pending authorization | | | |
| 6 Prepare canary | pending | | | |
| 7 Magic bridge | pending | | | |
| 8 Recall | pending | | | |
| 9 MagicTools | pending | | | |
| 10 Socratic Thinker | pending | | | |
| 11 DuckDuckGo | pending | | | |
| G2 product releases | pending authorization | | | |
| 12 Audit/legacy cleanup | deferred | | | |
