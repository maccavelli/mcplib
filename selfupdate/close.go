package selfupdate

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

func joinClose(err error, c io.Closer) error {
	if c == nil {
		return err
	}
	return errors.Join(err, c.Close())
}

func joinRemove(err error, path string) error {
	if path == "" {
		return err
	}
	rerr := os.Remove(path)
	if rerr != nil && errors.Is(rerr, os.ErrNotExist) {
		rerr = nil
	}
	return errors.Join(err, rerr)
}

func openAbsFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	dir := filepath.Dir(path)
	name := filepath.Base(path)
	if name == "" || name == "." || name == ".." {
		return nil, errors.New("selfupdate: path is not a file")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	f, err := root.OpenFile(name, flag, perm)
	cerr := root.Close()
	if err != nil {
		return nil, errors.Join(err, cerr)
	}
	if cerr != nil {
		return nil, joinClose(cerr, f)
	}
	return f, nil
}
