package vm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/internal/iptables"
	"git.g3e.fr/syonad/two/internal/metadata"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/qemu"
	"git.g3e.fr/syonad/two/internal/state"

	"github.com/dgraph-io/badger/v4"
)

func StartVM(db *badger.DB, name string, cfg *configuration.Config) error {
	current, err := state.Get(db, "vm/"+name)
	if err != nil {
		return err
	}
	if current != state.Creating {
		return nil
	}

	d, err := loadVM(db, name)
	if err != nil {
		return err
	}
	nic := d.primary()

	if err := netif.CreateTap(nic.tapID, nic.bridge, nic.vpcName); err != nil {
		return fmt.Errorf("create tap: %w", err)
	}

	if err := netns.Call(nic.vpcName, func() error {
		return iptables.AddMetadataRedirect(nic.ip, nic.interfaceIP, d.metadataPort)
	}); err != nil {
		return fmt.Errorf("add metadata redirect: %w", err)
	}

	if err := dhcp.WriteReservations(dhcp.DefaultConfDir, nic.vpcName+"_"+nic.bridge, name,
		[]dhcp.Reservation{{MAC: nic.mac, IP: nic.ip}}); err != nil {
		return fmt.Errorf("write dhcp reservation: %w", err)
	}

	if err := metadata.StartMetadata(metadata.NoCloudConfig{
		Name:      name,
		VpcName:   nic.vpcName,
		BindIP:    nic.interfaceIP,
		BindPort:  d.metadataPort,
		Password:  d.password,
		SSHKEY:    d.sshkey,
		Documents: d.documents,
	}, cfg, false); err != nil {
		return fmt.Errorf("start metadata: %w", err)
	}

	qDisks := make([]qemu.DiskConfig, len(d.disks))
	for i, disk := range d.disks {
		qDisks[i] = qemu.DiskConfig{Path: disk.path, Dev: disk.dev}
	}

	qcfg := qemu.Config{
		Name:       name,
		TapID:      nic.tapID,
		Mac:        nic.mac,
		Disks:      qDisks,
		Memory:     d.memory,
		CPUs:       d.cpus,
		SerialDir:  cfg.QEMU.SerialDir,
		MonitorDir: cfg.QEMU.MonitorDir,
		QMPDir:     cfg.QEMU.QMPDir,
	}

	if d.uefi {
		varsPath := filepath.Join(cfg.QEMU.UEFIVarsDir, name+"-uefi-vars.fd")
		if err := copyFile(cfg.QEMU.OVMFVarsTemplate, varsPath); err != nil {
			return fmt.Errorf("copy uefi vars: %w", err)
		}
		qcfg.UEFICodePath = cfg.QEMU.OVMFCodePath
		qcfg.UEFIVarsPath = varsPath
	}

	if err := netns.Call(nic.vpcName, func() error {
		return qemu.Start(qcfg)
	}); err != nil {
		return fmt.Errorf("start qemu: %w", err)
	}

	return state.Set(db, "vm/"+name, state.Running)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
