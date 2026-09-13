package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FileTokenStore struct {
	Dir string
}

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

func (fs *FileTokenStore) Save(ctx context.Context, provider string, s *OAuthSession) error {
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
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("FileTokenStore write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("FileTokenStore close: %w", err)
	}
	if err := chmod0600(tmpName); err != nil {
		return fmt.Errorf("FileTokenStore chmod: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return fmt.Errorf("FileTokenStore rename: %w", err)
	}
	chmod0600(final)
	return nil
}

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
