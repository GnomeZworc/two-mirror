//go:build linux

package qemu

import (
	"fmt"
	"os/exec"
)

type Config struct {
	Name       string
	TapID      int
	Mac        string
	VolumePath string
	Memory     int
	CPUs       int
}

func Start(cfg Config) error {
	memory := cfg.Memory
	if memory == 0 {
		memory = 512
	}

	cpus := cfg.CPUs
	if cpus == 0 {
		cpus = 1
	}

	cmd := exec.Command("qemu-system-x86_64",
		"-enable-kvm",
		"-cpu", "host",
		"-m", fmt.Sprintf("%d", memory),
		"-smp", fmt.Sprintf("%d", cpus),
		"-serial", fmt.Sprintf("unix:/tmp/%s.sock,server,nowait", cfg.Name),
		"-monitor", fmt.Sprintf("unix:/tmp/%s.mon-sock,server,nowait", cfg.Name),
		"-qmp", fmt.Sprintf("unix:/tmp/%s.qmp-sock,server,nowait", cfg.Name),
		"-display", "none",
		"-drive", fmt.Sprintf("file=%s,if=virtio", cfg.VolumePath),
		"-netdev", fmt.Sprintf("tap,id=net0,ifname=tap%d,script=no,downscript=no", cfg.TapID),
		"-device", fmt.Sprintf("virtio-net-pci,netdev=net0,mac=%s", cfg.Mac),
		"-daemonize",
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("qemu-system-x86_64: %w", err)
	}
	return nil
}
