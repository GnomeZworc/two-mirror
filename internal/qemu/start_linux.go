//go:build linux

package qemu

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func Start(cfg Config) error {
	memory := cfg.Memory
	if memory == 0 {
		memory = 512
	}

	cpus := cfg.CPUs
	if cpus == 0 {
		cpus = 1
	}

	for _, dir := range []string{cfg.SerialDir, cfg.MonitorDir, cfg.QMPDir} {
		if dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("mkdir %s: %w", dir, err)
			}
		}
	}

	serialSock := filepath.Join(cfg.SerialDir, cfg.Name+".sock")
	monitorSock := filepath.Join(cfg.MonitorDir, cfg.Name+".sock")
	qmpSock := filepath.Join(cfg.QMPDir, cfg.Name+".sock")

	args := []string{
		"-enable-kvm",
		"-cpu", "host",
		"-m", fmt.Sprintf("%d", memory),
		"-smp", fmt.Sprintf("%d", cpus),
		"-serial", fmt.Sprintf("unix:%s,server,nowait", serialSock),
		"-monitor", fmt.Sprintf("unix:%s,server,nowait", monitorSock),
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", qmpSock),
		"-display", "none",
	}

	if cfg.UEFICodePath != "" && cfg.UEFIVarsPath != "" {
		args = append(args,
			"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", cfg.UEFICodePath),
			"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", cfg.UEFIVarsPath),
		)
	}

	hasScsi := false
	for _, d := range cfg.Disks {
		if strings.HasPrefix(d.Dev, "sd") {
			hasScsi = true
			break
		}
	}
	if hasScsi {
		args = append(args, "-device", "virtio-scsi-pci,id=scsi0")
	}

	sorted := make([]DiskConfig, len(cfg.Disks))
	copy(sorted, cfg.Disks)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Dev < sorted[j].Dev })

	for _, d := range sorted {
		if strings.HasPrefix(d.Dev, "sd") {
			scsiID := int(d.Dev[2] - 'a')
			args = append(args,
				"-drive", fmt.Sprintf("file=%s,if=none,id=%s", d.Path, d.Dev),
				"-device", fmt.Sprintf("scsi-hd,drive=%s,bus=scsi0.0,scsi-id=%d", d.Dev, scsiID),
			)
		} else {
			args = append(args,
				"-drive", fmt.Sprintf("file=%s,if=virtio,id=%s", d.Path, d.Dev),
			)
		}
	}

	args = append(args,
		"-netdev", fmt.Sprintf("tap,id=net0,ifname=tap%d,script=no,downscript=no", cfg.TapID),
		"-device", fmt.Sprintf("virtio-net-pci,netdev=net0,mac=%s", cfg.Mac),
		"-daemonize",
	)

	cmd := exec.Command("qemu-system-x86_64", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("qemu-system-x86_64: %w", err)
	}
	return nil
}
