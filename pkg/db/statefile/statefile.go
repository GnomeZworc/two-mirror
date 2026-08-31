package statefile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	FileMode = 0o600
	DirMode  = 0o700
)

type File[T any] struct {
	path string
}

func New[T any](path string) *File[T] {
	return &File[T]{path: path}
}

func (f *File[T]) Path() string {
	return f.path
}

func (f *File[T]) Load() (T, error) {
	var value T

	raw, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return value, f.Save(value)
	}
	if err != nil {
		return value, fmt.Errorf("read %s: %w", f.path, err)
	}
	if len(raw) == 0 {
		return value, nil
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		var zero T
		return zero, fmt.Errorf("parse %s: %w", f.path, err)
	}
	return value, nil
}

func (f *File[T]) Save(value T) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode state for %s: %w", f.path, err)
	}

	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, DirMode); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(f.path)+".tmp")
	if err != nil {
		return fmt.Errorf("create temp state in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(FileMode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod %s: %w", tmp.Name(), err)
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}

	if err := os.Rename(tmp.Name(), f.path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmp.Name(), f.path, err)
	}
	return nil
}

func (f *File[T]) Remove() error {
	if err := os.Remove(f.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", f.path, err)
	}
	return nil
}
