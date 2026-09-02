package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type installSession struct {
	target   Target
	policy   TargetPolicy
	root     *os.Root
	lock     lockHandle
	staging  map[string]struct{}
	closed   bool
	mu       sync.Mutex
	closeErr error
}

func (s *installSession) Target() Target {
	return s.target
}

func (s *installSession) CreateStaging(ctx context.Context) (*os.File, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, "", fmt.Errorf("selfupdate: session is closed")
	}
	f, err := os.CreateTemp(s.target.Dir, "."+s.target.Base+".selfupdate-")
	if err != nil {
		return nil, "", fmt.Errorf("selfupdate: create staging: %w", err)
	}
	name := f.Name()
	base := filepath.Base(name)
	if _, err := s.root.Lstat(base); err != nil {
		err = fmt.Errorf("selfupdate: staging escaped target directory: %w", err)
		err = joinClose(err, f)
		return nil, "", joinRemove(err, name)
	}
	if s.staging == nil {
		s.staging = make(map[string]struct{})
	}
	s.staging[name] = struct{}{}
	return f, name, nil
}

func (s *installSession) owns(path string) bool {
	_, ok := s.staging[path]
	return ok
}

func (s *installSession) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return InstallResult{}, fmt.Errorf("selfupdate: session is closed")
	}
	if !s.owns(req.Artifact.Path) {
		return InstallResult{}, fmt.Errorf("selfupdate: artifact is not owned by this session")
	}
	applied, err := replaceTarget(s.target, req.Artifact.Path)
	delete(s.staging, req.Artifact.Path)
	if err != nil {
		return InstallResult{}, err
	}
	pending, err := commitReplacement(s.target, applied)
	if err != nil {
		return InstallResult{
			Target:        s.target.Path,
			Backup:        applied.backup,
			Applied:       true,
			PendingBackup: pending,
		}, err
	}
	return InstallResult{
		Target:        s.target.Path,
		Backup:        applied.backup,
		Applied:       true,
		PendingBackup: pending,
	}, nil
}

func (s *installSession) apply(ctx context.Context, req InstallRequest) (applyResult, error) {
	if err := ctx.Err(); err != nil {
		return applyResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return applyResult{}, fmt.Errorf("selfupdate: session is closed")
	}
	if !s.owns(req.Artifact.Path) {
		return applyResult{}, fmt.Errorf("selfupdate: artifact is not owned by this session")
	}
	applied, err := replaceTarget(s.target, req.Artifact.Path)
	delete(s.staging, req.Artifact.Path)
	return applied, err
}

func (s *installSession) commit(applied applyResult) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return commitReplacement(s.target, applied)
}

func (s *installSession) rollback(applied applyResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return rollbackReplacement(s.target, applied)
}

func (s *installSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	var errs []error
	for name := range s.staging {
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	s.staging = nil
	if s.root != nil {
		if err := s.root.Close(); err != nil {
			errs = append(errs, err)
		}
		s.root = nil
	}
	if err := s.lock.release(); err != nil {
		errs = append(errs, err)
	}
	s.closeErr = errors.Join(errs...)
	return s.closeErr
}

func beginSession(ctx context.Context, policy TargetPolicy, original Target, timeout time.Duration) (*installSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(original.Dir)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: open target directory: %w", err)
	}
	lock, err := acquireLock(ctx, root, original.Base, timeout)
	if err != nil {
		err = joinClose(err, root)
		if errors.Is(err, ErrConcurrentUpdate) {
			return nil, err
		}
		return nil, fmt.Errorf("selfupdate: acquire lock: %w", err)
	}
	if err := processCleanupReceipt(original, root); err != nil {
		return nil, errors.Join(err, lock.release(), root.Close())
	}
	if err := revalidateTarget(original, policy); err != nil {
		return nil, errors.Join(err, lock.release(), root.Close())
	}
	return &installSession{
		target:  original,
		policy:  policy,
		root:    root,
		lock:    lock,
		staging: make(map[string]struct{}),
	}, nil
}
