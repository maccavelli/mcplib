package selfupdate

import (
	"errors"
	"io"
	"os"
	"syscall"
)

var (
	osRename  = os.Rename
	osChmod   = os.Chmod
	osRemove  = os.Remove
	osLink    = os.Link
	syncDirFn = syncDirectory
)

func randomSibling(dir, prefix string) (string, error) {
	f, err := os.CreateTemp(dir, prefix)
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		return "", joinRemove(err, name)
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func copyFile(src, dst string) (err error) {
	in, err := openAbsFile(src, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer func() {
		err = joinClose(err, in)
	}()
	out, err := openAbsFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, copyErr := io.Copy(out, in); copyErr != nil {
		return joinRemove(joinClose(copyErr, out), dst)
	}
	if syncErr := out.Sync(); syncErr != nil {
		return joinRemove(joinClose(syncErr, out), dst)
	}
	if closeErr := out.Close(); closeErr != nil {
		return joinRemove(closeErr, dst)
	}
	return nil
}

func backupFile(target, backup string) error {
	if err := osLink(target, backup); err == nil {
		return nil
	}
	return copyFile(target, backup)
}

func chmodStaging(staging string, old os.FileInfo) error {
	return osChmod(staging, old.Mode().Perm())
}

func syncDirectory(dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	f, err := root.Open(".")
	cerr := root.Close()
	if err != nil {
		if isUnsupportedSync(err) {
			return cerr
		}
		return errors.Join(err, cerr)
	}
	syncErr := f.Sync()
	syncErr = joinClose(syncErr, f)
	if cerr != nil {
		return errors.Join(syncErr, cerr)
	}
	if syncErr == nil || isUnsupportedSync(syncErr) {
		return nil
	}
	return syncErr
}

func isUnsupportedSync(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOENT)
}
