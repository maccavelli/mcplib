//go:build windows

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

type lockHandle struct {
	file *os.File
}

func lockBasename(base string) string {
	return "." + base + ".selfupdate.lock"
}

func acquireLock(ctx context.Context, root *os.Root, base string, timeout time.Duration) (lockHandle, error) {
	name := lockBasename(base)
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return lockHandle{}, err
		}
		h, err := tryAcquireLock(root, name)
		if err == nil {
			return h, nil
		}
		if !isLockBusy(err) {
			return lockHandle{}, err
		}
		if timeout == 0 || time.Now().After(deadline) {
			return lockHandle{}, ErrConcurrentUpdate
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lockHandle{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func tryAcquireLock(root *os.Root, name string) (lockHandle, error) {
	f, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return lockHandle{}, fmt.Errorf("selfupdate: open lock: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		return lockHandle{}, joinClose(err, f)
	}
	if !info.Mode().IsRegular() {
		return lockHandle{}, joinClose(fmt.Errorf("selfupdate: lock is not a regular file"), f)
	}
	if isReparsePoint(f.Name()) {
		return lockHandle{}, joinClose(fmt.Errorf("selfupdate: lock is a reparse point"), f)
	}
	var overlapped windows.Overlapped
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if err != nil {
		return lockHandle{}, joinClose(err, f)
	}
	return lockHandle{file: f}, nil
}

func isLockBusy(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING)
}

func isReparsePoint(path string) bool {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return false
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func (h lockHandle) release() error {
	if h.file == nil {
		return nil
	}
	var overlapped windows.Overlapped
	err := windows.UnlockFileEx(windows.Handle(h.file.Fd()), 0, 1, 0, &overlapped)
	if cerr := h.file.Close(); err == nil {
		err = cerr
	}
	return err
}
