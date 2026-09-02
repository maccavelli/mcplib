package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// New constructs an Updater. Source, Versions, Assets, Installer, Reporter,
// and Confirmer are required. An empty Verifiers slice is valid. A nil
// Transformer is a no-op.
func New(cfg Config) (*Updater, error) {
	if cfg.Source == nil {
		return nil, fmt.Errorf("selfupdate: source is required")
	}
	if cfg.Versions == nil {
		return nil, fmt.Errorf("selfupdate: version policy is required")
	}
	if cfg.Assets == nil {
		return nil, fmt.Errorf("selfupdate: asset selector is required")
	}
	if cfg.Installer == nil {
		return nil, fmt.Errorf("selfupdate: installer is required")
	}
	if cfg.Reporter == nil {
		return nil, fmt.Errorf("selfupdate: reporter is required")
	}
	if cfg.Confirmer == nil {
		return nil, fmt.Errorf("selfupdate: confirmer is required")
	}
	if err := cfg.Limits.valid(); err != nil {
		return nil, err
	}
	transformer := cfg.Transformer
	if transformer == nil {
		transformer = noopTransformer{}
	}
	verifiers := append([]Verifier(nil), cfg.Verifiers...)
	return &Updater{
		source:      cfg.Source,
		versions:    cfg.Versions,
		assets:      cfg.Assets,
		verifiers:   verifiers,
		transformer: transformer,
		installer:   cfg.Installer,
		reporter:    cfg.Reporter,
		confirmer:   cfg.Confirmer,
		limits:      cfg.Limits,
	}, nil
}

// Run executes one self-update request. The library never calls os.Exit.
func (u *Updater) Run(ctx context.Context, req Request) (Result, error) {
	if !u.running.CompareAndSwap(false, true) {
		return Result{}, ErrConcurrentUpdate
	}
	defer u.running.Store(false)
	return u.execute(ctx, req)
}

func (u *Updater) execute(ctx context.Context, req Request) (Result, error) {
	if err := validateRequest(req); err != nil {
		return Result{}, err
	}
	req.Platform = normalizePlatform(req.Platform)
	if err := u.report(ctx, Event{Kind: EventResolvingTarget, Product: req.Product, Current: req.CurrentVersion}); err != nil {
		return Result{}, err
	}
	target, err := u.installer.ResolveTarget(ctx)
	if err != nil {
		return Result{}, wrapRun(req, err)
	}
	if err := u.report(ctx, Event{Kind: EventFetchingRelease, Product: req.Product, Current: req.CurrentVersion, Target: req.TargetVersion}); err != nil {
		return Result{}, err
	}
	rel, fromLatest, err := u.fetchRelease(ctx, req)
	if err != nil {
		return Result{}, wrapRun(req, err)
	}
	if rel.Draft || rel.Prerelease {
		return Result{}, wrapRun(req, fmt.Errorf("selfupdate: release %s is not a stable published release", rel.Tag))
	}
	if !rel.Immutable {
		return Result{}, wrapRun(req, fmt.Errorf("selfupdate: release %s is not immutable: %w", rel.Tag, ErrMutableRelease))
	}
	if err := u.versions.Validate(rel.Tag); err != nil {
		return Result{}, wrapRun(req, err)
	}
	sel, err := u.assets.Select(rel, req.Product, req.Platform)
	if err != nil {
		return Result{}, wrapRun(req, err)
	}
	op, err := classifyOperation(u.versions, req, rel.Tag, fromLatest)
	if err != nil {
		return Result{}, wrapRun(req, err)
	}
	result := Result{
		Product:        req.Product,
		CurrentVersion: req.CurrentVersion,
		TargetVersion:  rel.Tag,
		ReleaseURL:     rel.URL,
		AssetName:      sel.Binary.Name,
		Operation:      op,
	}
	if err := u.report(ctx, Event{
		Kind: EventSelected, Product: req.Product, Current: req.CurrentVersion,
		Target: rel.Tag, Asset: sel.Binary.Name,
	}); err != nil {
		return Result{}, err
	}
	if req.CheckOnly {
		result.Checked = true
		if op == OperationNone {
			return result, nil
		}
		return result, ErrUpdateAvailable
	}
	if op == OperationNone {
		return result, nil
	}
	if !req.Yes {
		ok, err := u.confirmer.Confirm(ctx, Prompt{
			Product: req.Product, Current: req.CurrentVersion, Target: rel.Tag, Operation: op,
		})
		if err != nil {
			return result, wrapRun(req, err)
		}
		if !ok {
			result.Declined = true
			return result, nil
		}
	}
	return u.apply(ctx, req, result, target, rel, sel)
}

func (u *Updater) fetchRelease(ctx context.Context, req Request) (Release, bool, error) {
	if req.TargetVersion == "" {
		rel, err := u.source.Latest(ctx)
		return rel, true, err
	}
	rel, err := u.source.ByTag(ctx, req.TargetVersion)
	return rel, false, err
}

func (u *Updater) apply(ctx context.Context, req Request, result Result, target Target, rel Release, sel Selection) (resultOut Result, err error) {
	resultOut = result
	sess, err := u.installer.Begin(ctx, target)
	if err != nil {
		return resultOut, wrapRun(req, err)
	}
	defer func() {
		err = errors.Join(err, sess.Close())
	}()
	if rerr := u.report(ctx, Event{Kind: EventDownloadingManifest, Product: req.Product, Target: rel.Tag, Asset: sel.Manifest.Name}); rerr != nil {
		return resultOut, rerr
	}
	var manifestBuf bytes.Buffer
	if _, err = downloadAsset(ctx, u.source, rel, sel.Manifest, &manifestBuf, u.limits.Manifest); err != nil {
		return resultOut, wrapRun(req, err)
	}
	if rerr := u.report(ctx, Event{Kind: EventDownloadingBinary, Product: req.Product, Target: rel.Tag, Asset: sel.Binary.Name, Bytes: sel.Binary.Size}); rerr != nil {
		return resultOut, rerr
	}
	f, stagedPath, err := sess.CreateStaging(ctx)
	if err != nil {
		return resultOut, wrapRun(req, err)
	}
	releaseDigest, err := downloadAsset(ctx, u.source, rel, sel.Binary, f, u.limits.Executable)
	closeErr := f.Close()
	if err != nil {
		return resultOut, wrapRun(req, errors.Join(err, closeErr))
	}
	if closeErr != nil {
		return resultOut, wrapRun(req, closeErr)
	}
	entries, err := parseSHA256SUMS(manifestBuf.Bytes())
	if err != nil {
		return resultOut, wrapRun(req, err)
	}
	manifestDigest, err := checksumFor(entries, sel.ManifestName)
	if err != nil {
		return resultOut, wrapRun(req, err)
	}
	ghDigest := ""
	if sel.Binary.Digest != "" {
		ghDigest, err = parseGitHubDigest(sel.Binary.Digest)
		if err != nil {
			return resultOut, wrapRun(req, err)
		}
	}
	if err = verifyIntegrity(Verification{
		Product: req.Product, Release: rel, Selection: sel,
		Size: sel.Binary.Size, SHA256: releaseDigest, ManifestSHA256: manifestDigest, GitHubSHA256: ghDigest,
	}); err != nil {
		return resultOut, wrapRun(req, err)
	}
	if err = u.runVerifiers(ctx, req, rel, sel, stagedPath, releaseDigest, ghDigest); err != nil {
		return resultOut, wrapRun(req, err)
	}
	installedDigest := releaseDigest
	if _, isNoop := u.transformer.(noopTransformer); !isNoop {
		if rerr := u.report(ctx, Event{Kind: EventTransforming, Product: req.Product, Asset: sel.Binary.Name}); rerr != nil {
			return resultOut, rerr
		}
		if err = u.transformer.Transform(ctx, TransformRequest{
			Product: req.Product, Platform: req.Platform, Path: stagedPath, ReleaseDigest: releaseDigest,
		}); err != nil {
			return resultOut, wrapRun(req, err)
		}
		installedDigest, err = hashAndValidateStaging(sess, stagedPath, u.limits.Executable)
		if err != nil {
			return resultOut, wrapRun(req, err)
		}
	}
	if rerr := u.report(ctx, Event{Kind: EventVerified, Product: req.Product, Target: rel.Tag, Asset: sel.Binary.Name}); rerr != nil {
		return resultOut, rerr
	}
	if rerr := u.report(ctx, Event{Kind: EventInstalling, Product: req.Product, Target: rel.Tag, Asset: sel.Binary.Name}); rerr != nil {
		return resultOut, rerr
	}
	installed, instErr := sess.Install(ctx, InstallRequest{
		Product: req.Product,
		Artifact: StagedArtifact{
			Path: stagedPath, Size: sel.Binary.Size,
			ReleaseDigest: releaseDigest, InstalledDigest: installedDigest,
		},
	})
	resultOut.ReleaseDigest = releaseDigest
	resultOut.InstalledDigest = installedDigest
	resultOut.ServiceInstalled = installed.ServiceInstalled
	resultOut.ServiceWasRunning = installed.ServiceWasRunning
	resultOut.PendingBackup = installed.PendingBackup
	if instErr != nil && !installed.Applied {
		return resultOut, wrapRun(req, instErr)
	}
	resultOut.Applied = installed.Applied
	if resultOut.Applied {
		detail := "release asset integrity verified"
		if resultOut.PendingBackup != "" {
			detail = "pending backup " + sanitizeText(resultOut.PendingBackup) + " will be removed on the next apply"
		}
		repErr := u.report(ctx, Event{
			Kind: EventComplete, Product: req.Product, Current: req.CurrentVersion,
			Target: rel.Tag, Asset: sel.Binary.Name, Detail: detail,
		})
		return resultOut, errors.Join(instErr, repErr)
	}
	return resultOut, wrapRun(req, instErr)
}

func (u *Updater) runVerifiers(ctx context.Context, req Request, rel Release, sel Selection, path, digest, ghDigest string) error {
	for _, v := range u.verifiers {
		err := v.Verify(ctx, Verification{
			Product: req.Product, Release: rel, Selection: sel,
			Size: sel.Binary.Size, SHA256: digest, ManifestSHA256: digest, GitHubSHA256: ghDigest,
			Open: func() (io.ReadCloser, error) {
				return openAbsFile(path, os.O_RDONLY, 0)
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (u *Updater) report(ctx context.Context, ev Event) error {
	return u.reporter.Report(ctx, ev)
}

func hashAndValidateStaging(sess InstallSession, path string, limit int64) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("selfupdate: transformed staging is not a regular file")
	}
	if info.Size() > limit {
		return "", fmt.Errorf("selfupdate: transformed staging exceeds executable limit")
	}
	if filepath.Base(path) != filepath.Base(sess.Target().Path) && !sessOwns(sess, path) {
		return "", fmt.Errorf("selfupdate: transformed staging is not owned by the session")
	}
	sum, err := hashFile(path)
	if err != nil {
		return "", err
	}
	return sum, nil
}

func sessOwns(sess InstallSession, path string) bool {
	inner, ok := sess.(*installSession)
	if !ok {
		if m, ok := sess.(*managedSession); ok {
			inner = m.inner
		}
	}
	if inner == nil {
		return true
	}
	inner.mu.Lock()
	defer inner.mu.Unlock()
	return inner.owns(path)
}

func hashFile(path string) (digest string, err error) {
	f, err := openAbsFile(path, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	defer func() {
		err = joinClose(err, f)
	}()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func normalizePlatform(p Platform) Platform {
	if p.OS == "" && p.Arch == "" {
		return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	}
	return p
}

func wrapRun(req Request, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("selfupdate: %s: %w", req.Product, err)
}
