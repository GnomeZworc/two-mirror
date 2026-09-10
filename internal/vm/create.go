package vm

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/dhcpbackend"
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

	backend, err := dhcpbackend.New(cfg)
	if err != nil {
		return err
	}

	for _, n := range d.nics {
		if err := netif.CreateTap(n.tapID, n.bridge, n.vpcName); err != nil {
			return fmt.Errorf("create tap of interface %d: %w", n.index, err)
		}
	}

	// La redirection est posée pour chaque IP de la VM vers le serveur de
	// métadonnées de l'interface primaire. Toutes les interfaces étant dans le
	// même VPC, donc le même netns, il est joignable depuis n'importe laquelle.
	if err := netns.Call(nic.vpcName, func() error {
		for _, n := range d.nics {
			if err := iptables.AddMetadataRedirect(n.ip, nic.interfaceIP, d.metadataPort); err != nil {
				return fmt.Errorf("interface %d: %w", n.index, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("add metadata redirect: %w", err)
	}

	if err := writeDHCPFiles(d, name, backend); err != nil {
		return err
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

	qNICs := make([]qemu.NICConfig, len(d.nics))
	for i, n := range d.nics {
		qNICs[i] = qemu.NICConfig{TapID: n.tapID, Mac: n.mac}
	}

	qcfg := qemu.Config{
		Name:       name,
		NICs:       qNICs,
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

func dhcpReservations(d vmData) map[string]*subnetReservations {
	bySubnet := make(map[string]*subnetReservations)

	for _, n := range d.nics {
		key := n.vpcName + "_" + n.bridge
		if bySubnet[key] == nil {
			bySubnet[key] = &subnetReservations{subnet: dhcpbackend.Subnet{
				Name:        n.subnetName,
				VPC:         n.vpcName,
				Bridge:      n.bridge,
				InterfaceIP: net.ParseIP(n.interfaceIP),
				VPCRoute:    n.vpcCIDR,
			}}
		}
		f := bySubnet[key]
		f.reservations = append(f.reservations, dhcpbackend.Reservation{
			Index: n.index, MAC: n.mac, IP: n.ip, DefaultRoute: n.primary,
		})
	}
	return bySubnet
}

func writeDHCPFiles(d vmData, name string, backend dhcpbackend.Backend) error {
	for _, f := range dhcpReservations(d) {
		if err := backend.SetVM(f.subnet, name, f.reservations); err != nil {
			return err
		}
	}
	return nil
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
