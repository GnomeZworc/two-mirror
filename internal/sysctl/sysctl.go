package sysctl

import (
	"fmt"
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
