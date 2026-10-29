package sysctl

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var root = "/proc/sys"

func Set(key, value string) error {
	path := filepath.Join(root, strings.ReplaceAll(key, ".", "/"))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return fmt.Errorf("sysctl %s: %w", key, err)
	}
	if _, err := f.WriteString(value); err != nil {
		f.Close()
		return fmt.Errorf("sysctl %s=%s: %w", key, value, err)
	}
	return f.Close()
}

func SetIfPresent(key, value string) (bool, error) {
	path := filepath.Join(root, strings.ReplaceAll(key, ".", "/"))
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("sysctl %s: %w", key, err)
	}
	return true, Set(key, value)
}
