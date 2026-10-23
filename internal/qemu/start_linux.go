//go:build linux

package qemu

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Start(cfg Config) error {
	for _, dir := range []string{cfg.SerialDir, cfg.MonitorDir, cfg.QMPDir} {
		if dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("mkdir %s: %w", dir, err)
			}
		}
	}

	args, err := buildArgs(cfg)
	if err != nil {
		return err
	}

	scopeArgs := append([]string{
		"--scope",
		"--unit=" + ScopeName(cfg.Name),
		"--collect",
		"qemu-system-x86_64",
	}, args...)

	cmd := exec.Command("systemd-run", scopeArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemd-run qemu-system-x86_64: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
