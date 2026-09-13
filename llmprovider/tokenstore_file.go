package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
)

// FileTokenStore persists one OAuth session per provider in a directory.
type FileTokenStore struct {
	Dir string
}

// NewFileTokenStore creates the token directory when needed.
func NewFileTokenStore(dir string) (*FileTokenStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("FileTokenStore: empty dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("FileTokenStore mkdir: %w", err)
	}
	return &FileTokenStore{Dir: dir}, nil
}

func (fs *FileTokenStore) path(provider string) string {
	return filepath.Join(fs.Dir, provider+".json")
}

// Load reads a provider session, returning nil when no token file exists.
func (fs *FileTokenStore) Load(ctx context.Context, provider string) (*OAuthSession, error) {
	if err := validateProviderID(provider); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(fs.path(provider))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("FileTokenStore read: %w", err)
	}
	var rec fileRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("FileTokenStore decode: %w", err)
	}
	s := &OAuthSession{
		Provider:  rec.Provider,
		Access:    rec.Access,
		Refresh:   rec.Refresh,
		Expiry:    rec.Expiry,
		Issuer:    rec.Issuer,
		ClientID:  rec.ClientID,
		AccountID: rec.AccountID,
		TokenURL:  rec.TokenURL,
	}
	s.Store = fs
	return s, nil
}

// Save atomically replaces a provider session file.
func (fs *FileTokenStore) Save(ctx context.Context, provider string, s *OAuthSession) (err error) {
	if err := validateProviderID(provider); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("FileTokenStore Save: nil session")
	}
	rec := fileRecord{
		Provider:  provider,
		Access:    s.Access,
		Refresh:   s.Refresh,
		Expiry:    s.Expiry,
		Issuer:    s.Issuer,
		ClientID:  s.ClientID,
		AccountID: s.AccountID,
		TokenURL:  s.TokenURL,
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("FileTokenStore encode: %w", err)
	}
	final := fs.path(provider)
	tmp, err := os.CreateTemp(fs.Dir, ".tok-*.json")
	if err != nil {
		return fmt.Errorf("FileTokenStore temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		cleanupErr := os.Remove(tmpName)
		if cleanupErr != nil && !errors.Is(cleanupErr, iofs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("FileTokenStore cleanup: %w", cleanupErr))
		}
	}()
	if _, writeErr := tmp.Write(data); writeErr != nil {
		closeErr := tmp.Close()
		if closeErr != nil {
			return errors.Join(
				fmt.Errorf("FileTokenStore write: %w", writeErr),
				fmt.Errorf("FileTokenStore close after write: %w", closeErr),
			)
		}
		return fmt.Errorf("FileTokenStore write: %w", writeErr)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("FileTokenStore close: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return fmt.Errorf("FileTokenStore rename: %w", err)
	}
	if err := chmod0600(final); err != nil {
		return fmt.Errorf("FileTokenStore chmod: %w", err)
	}
	return nil
}

// Delete removes a provider session and succeeds when it is already absent.
func (fs *FileTokenStore) Delete(ctx context.Context, provider string) error {
	if err := validateProviderID(provider); err != nil {
		return err
	}
	err := os.Remove(fs.path(provider))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
