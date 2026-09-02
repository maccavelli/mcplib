package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	osExecutable = os.Executable
	userHomeDir  = os.UserHomeDir
	evalSymlinks = filepath.EvalSymlinks
)

func resolveTarget(policy TargetPolicy) (Target, error) {
	raw, err := rawExecutablePath(policy.ExecutablePath)
	if err != nil {
		return Target{}, err
	}
	info, err := os.Lstat(raw)
	if err != nil {
		return Target{}, fmt.Errorf("selfupdate: stat target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Target{}, fmt.Errorf("selfupdate: refusing to update through a symlink")
	}
	if !info.Mode().IsRegular() {
		return Target{}, fmt.Errorf("selfupdate: target is not a regular file")
	}
	resolved, err := evalSymlinks(raw)
	if err != nil {
		return Target{}, fmt.Errorf("selfupdate: resolve target: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return Target{}, err
	}
	resolvedInfo, err := os.Lstat(resolved)
	if err != nil {
		return Target{}, fmt.Errorf("selfupdate: stat resolved target: %w", err)
	}
	if resolvedInfo.Mode()&os.ModeSymlink != 0 || !resolvedInfo.Mode().IsRegular() {
		return Target{}, fmt.Errorf("selfupdate: resolved target is not a regular file")
	}
	dir := filepath.Dir(resolved)
	base := filepath.Base(resolved)
	if dir == resolved || base == "" || base == "." || base == ".." {
		return Target{}, fmt.Errorf("selfupdate: target path is not a file")
	}
	roots, err := allowedRoots(policy.AllowedRoots)
	if err != nil {
		return Target{}, err
	}
	if !underAnyRoot(resolved, roots) {
		return Target{}, fmt.Errorf("selfupdate: target %s is outside allowed roots", resolved)
	}
	return Target{
		Path: resolved,
		Dir:  dir,
		Base: base,
		identity: fileIdentity{
			info:  resolvedInfo,
			size:  resolvedInfo.Size(),
			mtime: resolvedInfo.ModTime().UnixNano(),
		},
	}, nil
}

func rawExecutablePath(explicit string) (string, error) {
	raw := explicit
	if raw == "" {
		var err error
		raw, err = osExecutable()
		if err != nil {
			return "", fmt.Errorf("selfupdate: locate executable: %w", err)
		}
	}
	if raw == "" {
		return "", fmt.Errorf("selfupdate: executable path is empty")
	}
	if !filepath.IsAbs(raw) {
		abs, err := filepath.Abs(raw)
		if err != nil {
			return "", err
		}
		raw = abs
	}
	return filepath.Clean(raw), nil
}

func allowedRoots(extra []string) ([]string, error) {
	home, err := userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("selfupdate: locate home directory: %w", err)
	}
	roots := make([]string, 0, 1+len(extra))
	homeRoot, err := canonicalizeRoot(home)
	if err != nil {
		return nil, err
	}
	roots = append(roots, homeRoot)
	for _, r := range extra {
		canon, err := canonicalizeRoot(r)
		if err != nil {
			return nil, err
		}
		if !containsRoot(roots, canon) {
			roots = append(roots, canon)
		}
	}
	return roots, nil
}

func canonicalizeRoot(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("selfupdate: allowed root is empty")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("selfupdate: allowed root must be absolute")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("selfupdate: stat allowed root: %w", err)
	}
	if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("selfupdate: allowed root is not a directory")
	}
	resolved, err := evalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("selfupdate: resolve allowed root: %w", err)
	}
	dirInfo, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !dirInfo.IsDir() {
		return "", fmt.Errorf("selfupdate: allowed root is not a directory")
	}
	if isFilesystemRoot(resolved) {
		return "", fmt.Errorf("selfupdate: filesystem root is not an allowed self-update root")
	}
	return resolved, nil
}

func isFilesystemRoot(path string) bool {
	cleaned := filepath.Clean(path)
	return filepath.Dir(cleaned) == cleaned
}

func containsRoot(roots []string, candidate string) bool {
	for _, r := range roots {
		if sameDir(r, candidate) {
			return true
		}
	}
	return false
}

func sameDir(a, b string) bool {
	if runtime.GOOS == goosWindows {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func underAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if underRoot(path, root) {
			return true
		}
	}
	return false
}

func underRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	if runtime.GOOS == goosWindows {
		relFold, err := filepath.Rel(strings.ToLower(root), strings.ToLower(path))
		if err != nil {
			return false
		}
		if relFold == ".." || strings.HasPrefix(relFold, ".."+string(os.PathSeparator)) {
			return false
		}
	}
	return true
}

func sameTargetIdentity(original Target, info os.FileInfo) bool {
	if original.identity.info == nil || info == nil {
		return false
	}
	if !os.SameFile(original.identity.info, info) {
		return false
	}
	if original.identity.size != info.Size() {
		return false
	}
	return original.identity.mtime == info.ModTime().UnixNano()
}

func revalidateTarget(original Target, policy TargetPolicy) error {
	fresh, err := resolveTarget(policy)
	if err != nil {
		return err
	}
	info, err := os.Lstat(fresh.Path)
	if err != nil {
		return err
	}
	if fresh.Path != original.Path || fresh.Dir != original.Dir || fresh.Base != original.Base {
		return fmt.Errorf("selfupdate: target changed during confirmation: %w", ErrConcurrentUpdate)
	}
	if !sameTargetIdentity(original, info) {
		return fmt.Errorf("selfupdate: target changed during confirmation: %w", ErrConcurrentUpdate)
	}
	return nil
}
