package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// BuildKind states whether the running binary is a published release or a
// local development build. Release comparison never infers this from version
// text.
type BuildKind uint8

const (
	// BuildUnknown is the zero value and is not a valid Request.CurrentBuild.
	BuildUnknown BuildKind = iota
	// ReleaseBuild is a binary stamped from a strict vMAJOR.MINOR.PATCH tag.
	ReleaseBuild
	// LocalBuild is any non-release identity, including dirty VCS trees.
	LocalBuild
)

// String implements fmt.Stringer.
func (k BuildKind) String() string {
	switch k {
	case BuildUnknown:
		return "unknown"
	case ReleaseBuild:
		return "release"
	case LocalBuild:
		return "local"
	default:
		return "buildkind(" + itoa(uint64(k)) + ")"
	}
}

// Platform is a GOOS/GOARCH pair. The zero value means the runtime platform.
type Platform struct {
	// OS is a GOOS value such as "linux". Empty only when Arch is also empty.
	OS string
	// Arch is a GOARCH value such as "amd64". Empty only when OS is also empty.
	Arch string
}

// Request is one self-update invocation. The library never reads global flag
// state or process arguments.
type Request struct {
	// Product is the executable basename used in exact asset names.
	Product string
	// CurrentVersion is the running identity. For ReleaseBuild it must be a
	// strict vMAJOR.MINOR.PATCH tag. For LocalBuild it is not ordered.
	CurrentVersion string
	// CurrentBuild distinguishes release and local binaries.
	CurrentBuild BuildKind
	// TargetVersion selects an exact stable tag. Empty means the latest
	// stable release.
	TargetVersion string
	// Platform selects the asset matrix entry. Zero means runtime GOOS/GOARCH.
	Platform Platform
	// CheckOnly performs discovery and policy evaluation without download.
	CheckOnly bool
	// Force permits replacing a local build or reinstalling the selected
	// version. It never bypasses validation, integrity, or target policy.
	Force bool
	// Yes approves the already-selected operation without prompting.
	Yes bool
}

// Operation is the classified action for a request and selected release.
type Operation uint8

const (
	// OperationNone means the running identity already equals the selection.
	OperationNone Operation = iota
	// OperationUpgrade installs a higher stable release.
	OperationUpgrade
	// OperationReinstall replaces the same release version under --force.
	OperationReinstall
	// OperationRollback installs an exact lower stable tag.
	OperationRollback
	// OperationReplaceLocal replaces a local build with a stable release.
	OperationReplaceLocal
)

// String implements fmt.Stringer.
func (o Operation) String() string {
	switch o {
	case OperationNone:
		return "none"
	case OperationUpgrade:
		return "upgrade"
	case OperationReinstall:
		return "reinstall"
	case OperationRollback:
		return "rollback"
	case OperationReplaceLocal:
		return "replace-local"
	default:
		return "operation(" + itoa(uint64(o)) + ")"
	}
}

// Result is the typed outcome of Updater.Run. The library never calls os.Exit.
type Result struct {
	// Product is the requested product name.
	Product string
	// CurrentVersion is the running identity supplied in the request.
	CurrentVersion string
	// TargetVersion is the selected release tag.
	TargetVersion string
	// ReleaseURL is the selected GitHub release HTML URL when known.
	ReleaseURL string
	// AssetName is the exact selected executable asset name.
	AssetName string
	// Operation is the classified action.
	Operation Operation
	// Checked is true when the run was check-only.
	Checked bool
	// Applied is true when a healthy installation committed.
	Applied bool
	// Declined is true when the user declined an interactive apply.
	Declined bool
	// ReleaseDigest is the verified SHA-256 hex of the release bytes.
	ReleaseDigest string
	// InstalledDigest is the SHA-256 hex of the bytes after any transform.
	InstalledDigest string
	// ServiceInstalled reports whether a managed definition existed.
	ServiceInstalled bool
	// ServiceWasRunning reports whether that definition's process was active.
	ServiceWasRunning bool
	// PendingBackup is the Windows running-image backup basename when the
	// active image kept the backup open after commit.
	PendingBackup string
}

// Repository is a GitHub owner/name pair.
type Repository struct {
	// Owner is the GitHub account or organization.
	Owner string
	// Name is the repository name.
	Name string
}

// GitHubOptions construct a GitHubSource. There is no convenience constructor
// that fills a client, token, or limits.
type GitHubOptions struct {
	// Repository is the GitHub owner/name pair.
	Repository Repository
	// Client is required. NewGitHubSource clones it and never mutates the
	// caller's value.
	Client *http.Client
	// APIBaseURL is the GitHub API origin. Nil means https://api.github.com.
	APIBaseURL *url.URL
	// UserAgent is the required product/version User-Agent.
	UserAgent string
	// Token is an explicit GitHub token. Empty falls back to GH_TOKEN then
	// GITHUB_TOKEN.
	Token string
	// Limits bound JSON, error, manifest, and executable bodies.
	Limits Limits
}

// Release is one GitHub release after JSON decoding and field validation.
type Release struct {
	// ID is the GitHub release identifier.
	ID int64
	// Tag is the release tag name.
	Tag string
	// URL is the HTML URL for the release.
	URL string
	// Draft reports whether the release is still a draft.
	Draft bool
	// Prerelease reports whether GitHub marked the release as a prerelease.
	Prerelease bool
	// Immutable reports whether GitHub marked the release immutable.
	Immutable bool
	// Assets are the release assets as returned by the API.
	Assets []Asset
}

// Asset is one GitHub release asset.
type Asset struct {
	// ID is the GitHub asset identifier used for API downloads.
	ID int64
	// Name is the exact asset filename.
	Name string
	// State is the GitHub asset state; uploaded is required to install.
	State string
	// Size is the advertised byte length.
	Size int64
	// Digest is GitHub's sha256:<hex> field when supplied.
	Digest string
}

// Selection is the exact executable and checksum manifest chosen from a release.
type Selection struct {
	// Binary is the exact platform executable asset.
	Binary Asset
	// Manifest is the exact SHA256SUMS asset.
	Manifest Asset
	// ManifestName is the binary basename looked up in SHA256SUMS.
	ManifestName string
}

// ReleaseSource discovers releases and opens asset bodies.
type ReleaseSource interface {
	Latest(context.Context) (Release, error)
	ByTag(context.Context, string) (Release, error)
	OpenAsset(context.Context, Release, Asset) (io.ReadCloser, error)
}

// AssetSelector chooses exactly one platform binary and the SHA256SUMS asset.
type AssetSelector interface {
	Select(Release, string, Platform) (Selection, error)
}

// VersionPolicy validates and compares strict stable release tags.
type VersionPolicy interface {
	Validate(string) error
	Compare(string, string) (int, error)
}

// Verification is the input to a Verifier. Open returns a fresh read-only
// descriptor of the staged bytes; it is not a writable staging path.
type Verification struct {
	// Product is the requested product name.
	Product string
	// Release is the selected release metadata.
	Release Release
	// Selection is the selected binary and manifest.
	Selection Selection
	// Size is the staged byte length.
	Size int64
	// SHA256 is the staged content digest.
	SHA256 string
	// ManifestSHA256 is the digest from the SHA256SUMS entry.
	ManifestSHA256 string
	// GitHubSHA256 is the digest from GitHub asset metadata when supplied.
	GitHubSHA256 string
	// Open returns a new reader over the staged bytes.
	Open func() (io.ReadCloser, error)
}

// Verifier is an additional integrity or authenticity check.
type Verifier interface {
	Verify(context.Context, Verification) error
}

// TransformRequest describes a consumer-authorized post-verification change
// such as macOS codesigning.
type TransformRequest struct {
	// Product is the requested product name.
	Product string
	// Platform is the selected platform.
	Platform Platform
	// Path is the locked staging path.
	Path string
	// ReleaseDigest is the verified pre-transform digest.
	ReleaseDigest string
}

// Transformer applies an authorized post-verification change. The coordinator
// recomputes InstalledDigest itself after Transform returns.
type Transformer interface {
	Transform(context.Context, TransformRequest) error
}

// StagedArtifact is a verified staging file owned by one InstallSession.
type StagedArtifact struct {
	// Path is the absolute staging path.
	Path string
	// Size is the staged byte length after any transform.
	Size int64
	// ReleaseDigest is the verified pre-transform digest.
	ReleaseDigest string
	// InstalledDigest is the post-transform digest.
	InstalledDigest string
}

// TargetPolicy controls which executable path may be replaced.
type TargetPolicy struct {
	// ExecutablePath overrides os.Executable when non-empty.
	ExecutablePath string
	// AllowedRoots are additive canonical directories besides the user's
	// home directory. An empty slice adds nothing.
	AllowedRoots []string
}

// Target is a resolved executable path. Callers treat values as opaque and
// must not construct or modify them.
type Target struct {
	// Path is the absolute resolved executable path.
	Path string
	// Dir is the absolute resolved parent directory.
	Dir string
	// Base is the executable basename.
	Base string

	identity fileIdentity
}

type fileIdentity struct {
	info  os.FileInfo
	size  int64
	mtime int64
}

// InstallRequest is the payload for InstallSession.Install.
type InstallRequest struct {
	// Product is the requested product name.
	Product string
	// Artifact is the session-owned staged binary.
	Artifact StagedArtifact
}

// InstallResult is the outcome of InstallSession.Install.
type InstallResult struct {
	// Target is the replaced executable path.
	Target string
	// Backup is the retained rollback path before commit cleanup.
	Backup string
	// Applied is true when replacement committed healthily.
	Applied bool
	// ServiceInstalled reports whether a managed definition existed.
	ServiceInstalled bool
	// ServiceWasRunning reports whether that definition's process was active.
	ServiceWasRunning bool
	// PendingBackup is the Windows running-image backup basename when needed.
	PendingBackup string
}

// Installer resolves the target and begins a locked install session.
type Installer interface {
	ResolveTarget(context.Context) (Target, error)
	Begin(context.Context, Target) (InstallSession, error)
}

// InstallSession owns the per-target lock, anchored directory, staging names,
// and cleanup through Close.
type InstallSession interface {
	Target() Target
	CreateStaging(context.Context) (*os.File, string, error)
	Install(context.Context, InstallRequest) (InstallResult, error)
	Close() error
}

// Lifecycle is the consumer-owned service control seam.
type Lifecycle interface {
	Installed(context.Context, string) (bool, error)
	Running(context.Context, string) (bool, error)
	Stop(context.Context, string) error
	Start(context.Context, string) error
	WaitHealthy(context.Context, string) error
}

// ReconcileResult is the restoration receipt for a definition change.
type ReconcileResult struct {
	// Changed reports whether the definition was rewritten.
	Changed bool
	// Detail is a short human-readable description.
	Detail string
	// State holds consumer restoration data.
	State any
}

// Reconciler rewrites an existing managed definition and can restore it.
type Reconciler interface {
	Reconcile(ctx context.Context, product, executable string) (ReconcileResult, error)
	Restore(ctx context.Context, product string, receipt ReconcileResult) error
}

// InstallOptions configure standalone target resolution and locking.
type InstallOptions struct {
	// TargetPolicy selects the executable and allowed roots.
	TargetPolicy TargetPolicy
	// LockTimeout bounds lock acquisition. Zero selects DefaultLockTimeout.
	LockTimeout time.Duration
}

// DefaultLockTimeout is the lock acquisition bound when InstallOptions leaves
// LockTimeout unset.
const DefaultLockTimeout time.Duration = 5 * time.Second

const goosWindows = "windows"

// EventKind is a structured progress event.
type EventKind uint8

const (
	// EventUnknown is the zero value and is never emitted.
	EventUnknown EventKind = iota
	// EventResolvingTarget is emitted before network work.
	EventResolvingTarget
	// EventFetchingRelease is emitted before release discovery.
	EventFetchingRelease
	// EventSelected is emitted after exact asset selection.
	EventSelected
	// EventDownloadingManifest is emitted before the SHA256SUMS body.
	EventDownloadingManifest
	// EventDownloadingBinary is emitted before the executable body.
	EventDownloadingBinary
	// EventVerified is emitted after integrity checks succeed.
	EventVerified
	// EventTransforming is emitted before an authorized transform.
	EventTransforming
	// EventInstalling is emitted immediately before Installer.Install.
	EventInstalling
	// EventComplete is emitted after a healthy committed installation.
	EventComplete
)

// String implements fmt.Stringer.
func (k EventKind) String() string {
	switch k {
	case EventUnknown:
		return "unknown"
	case EventResolvingTarget:
		return "resolving-target"
	case EventFetchingRelease:
		return "fetching-release"
	case EventSelected:
		return "selected"
	case EventDownloadingManifest:
		return "downloading-manifest"
	case EventDownloadingBinary:
		return "downloading-binary"
	case EventVerified:
		return "verified"
	case EventTransforming:
		return "transforming"
	case EventInstalling:
		return "installing"
	case EventComplete:
		return "complete"
	default:
		return "eventkind(" + itoa(uint64(k)) + ")"
	}
}

// Event is one structured progress report.
type Event struct {
	// Kind identifies the stage.
	Kind EventKind
	// Product is the requested product name.
	Product string
	// Current is the running identity.
	Current string
	// Target is the selected release tag.
	Target string
	// Asset is the selected executable name when known.
	Asset string
	// Bytes is a size or progress count when meaningful.
	Bytes int64
	// Detail is additional sanitized text.
	Detail string
}

// Reporter receives structured progress. Implementations must not read or
// write global standard streams unless the consumer injected them.
type Reporter interface {
	Report(context.Context, Event) error
}

// Prompt is the confirmation request for an already-selected operation.
type Prompt struct {
	// Product is the requested product name.
	Product string
	// Current is the running identity.
	Current string
	// Target is the selected release tag.
	Target string
	// Operation is the classified action.
	Operation Operation
}

// Confirmer approves or declines an already-selected apply.
type Confirmer interface {
	Confirm(context.Context, Prompt) (bool, error)
}

// Limits bound remote bodies. Zero values are invalid at Updater construction.
type Limits struct {
	// ReleaseJSON is the maximum GitHub release JSON body in bytes.
	ReleaseJSON int64
	// ErrorBody is the maximum error diagnostic body in bytes.
	ErrorBody int64
	// Manifest is the maximum SHA256SUMS body in bytes.
	Manifest int64
	// Executable is the maximum executable body in bytes.
	Executable int64
}

// DefaultLimits returns the canonical v1 body limits.
func DefaultLimits() Limits {
	return Limits{
		ReleaseJSON: 2 << 20,
		ErrorBody:   64 << 10,
		Manifest:    1 << 20,
		Executable:  512 << 20,
	}
}

func (l Limits) valid() error {
	if l.ReleaseJSON <= 0 || l.ErrorBody <= 0 || l.Manifest <= 0 || l.Executable <= 0 {
		return fmt.Errorf("selfupdate: limits must be positive")
	}
	return nil
}

// Config is the explicit Updater composition. There is no convenience
// constructor that fills security-relevant defaults.
type Config struct {
	// Source discovers releases.
	Source ReleaseSource
	// Versions validates and compares tags.
	Versions VersionPolicy
	// Assets selects exact raw-binary names.
	Assets AssetSelector
	// Verifiers run after built-in integrity checks. An empty slice is valid.
	Verifiers []Verifier
	// Transformer is an optional post-verification change. Nil is a no-op.
	Transformer Transformer
	// Installer owns target resolution and replacement.
	Installer Installer
	// Reporter receives structured progress.
	Reporter Reporter
	// Confirmer approves interactive applies.
	Confirmer Confirmer
	// Limits bound remote bodies.
	Limits Limits
}

// Updater is the coordinator. Unexported collaborator fields are populated
// by New and are immutable afterwards.
type Updater struct{}
