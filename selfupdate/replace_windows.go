//go:build windows

package selfupdate

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

type applyResult struct {
	backup        string
	pendingBackup string
	oldDigest     string
}

func replaceTarget(target Target, staging string) (applyResult, error) {
	info, err := os.Lstat(target.Path)
	if err != nil {
		return applyResult{}, err
	}
	if err := chmodStaging(staging, info); err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: chmod staging: %w", err)
	}
	oldDigest, err := fileSHA256(target.Path)
	if err != nil {
		return applyResult{}, err
	}
	backup, err := randomSibling(target.Dir, "."+target.Base+".selfupdate-bak-")
	if err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: allocate backup: %w", err)
	}
	if err := backupFile(target.Path, backup); err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: backup target: %w", err)
	}
	if err := moveFileReplace(staging, target.Path); err != nil {
		return applyResult{}, joinRemove(fmt.Errorf("selfupdate: replace target: %w", err), backup)
	}
	if err := syncDirFn(target.Dir); err != nil && !isUnsupportedSync(err) {
		if rerr := moveFileReplace(backup, target.Path); rerr != nil {
			return applyResult{backup: backup, oldDigest: oldDigest}, fmt.Errorf("selfupdate: sync directory: %w", err)
		}
		return applyResult{}, fmt.Errorf("selfupdate: sync directory: %w", err)
	}
	return applyResult{backup: backup, oldDigest: oldDigest}, nil
}

func moveFileReplace(from, to string) error {
	fromW, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	toW, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(DefaultLockTimeout)
	var last error
	for {
		last = windows.MoveFileEx(fromW, toW, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if last == nil {
			return nil
		}
		if !isSharingViolation(last) || time.Now().After(deadline) {
			return last
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func isSharingViolation(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

func commitReplacement(target Target, result applyResult) (pending string, err error) {
	if result.backup == "" {
		return "", nil
	}
	if err := osRemove(result.backup); err != nil {
		if isSharingViolation(err) {
			if werr := writeCleanupReceipt(target, result); werr != nil {
				return "", errors.Join(fmt.Errorf("selfupdate: remove backup: %w", err), werr)
			}
			return result.backup, nil
		}
		return "", fmt.Errorf("selfupdate: remove backup: %w", err)
	}
	return "", syncDirFn(target.Dir)
}

func rollbackReplacement(target Target, result applyResult) error {
	if result.backup == "" {
		return fmt.Errorf("selfupdate: no backup to restore")
	}
	if err := moveFileReplace(result.backup, target.Path); err != nil {
		return fmt.Errorf("selfupdate: restore backup: %w", err)
	}
	return syncDirFn(target.Dir)
}
