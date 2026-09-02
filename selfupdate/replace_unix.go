//go:build unix

package selfupdate

import (
	"fmt"
	"os"
)

type applyResult struct {
	backup string
}

func isUnsupportedDirSync(error) bool {
	return false
}

func replacePathOS(oldpath, newpath string) error {
	return osRename(oldpath, newpath)
}

func replaceTarget(target Target, staging string) (applyResult, error) {
	info, err := os.Lstat(target.Path)
	if err != nil {
		return applyResult{}, err
	}
	if err := chmodStaging(staging, info); err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: chmod staging: %w", err)
	}
	backup, err := randomSibling(target.Dir, "."+target.Base+".selfupdate-bak-")
	if err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: allocate backup: %w", err)
	}
	if err := backupFile(target.Path, backup); err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: backup target: %w", err)
	}
	if err := replacePath(staging, target.Path); err != nil {
		return applyResult{}, joinRemove(fmt.Errorf("selfupdate: rename staging over target: %w", err), backup)
	}
	if err := syncDirFn(target.Dir); err != nil {
		if rerr := replacePath(backup, target.Path); rerr != nil {
			return applyResult{backup: backup}, fmt.Errorf("selfupdate: sync directory: %w", err)
		}
		return applyResult{}, fmt.Errorf("selfupdate: sync directory: %w", err)
	}
	return applyResult{backup: backup}, nil
}

func commitReplacement(target Target, result applyResult) (pending string, err error) {
	if result.backup == "" {
		return "", nil
	}
	if err := osRemove(result.backup); err != nil {
		return "", fmt.Errorf("selfupdate: remove backup: %w", err)
	}
	if err := syncDirFn(target.Dir); err != nil {
		return "", err
	}
	return "", nil
}

func rollbackReplacement(target Target, result applyResult) error {
	if result.backup == "" {
		return fmt.Errorf("selfupdate: no backup to restore")
	}
	if err := replacePath(result.backup, target.Path); err != nil {
		return fmt.Errorf("selfupdate: restore backup: %w", err)
	}
	return syncDirFn(target.Dir)
}
