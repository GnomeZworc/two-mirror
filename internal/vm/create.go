package vm

import (
	"fmt"
	"io"
	"net"
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

	if err := writeDHCPFiles(d, name); err != nil {
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

// writeDHCPFiles écrit, pour chaque subnet touché par la VM, les réservations
// de ses interfaces et les options qui suppriment la route par défaut sur les
// interfaces non primaires. Le subnet de l'interface primaire ne reçoit aucune
// option : les options non taggées du subnet portent déjà la route par défaut.
func writeDHCPFiles(d vmData, name string) error {
	type subnetFiles struct {
		nic          nicData
		reservations []dhcp.Reservation
		tags         []string
	}
	bySubnet := make(map[string]*subnetFiles)

	for _, n := range d.nics {
		confName := n.vpcName + "_" + n.bridge
		if bySubnet[confName] == nil {
			bySubnet[confName] = &subnetFiles{nic: n}
		}
		f := bySubnet[confName]
		f.reservations = append(f.reservations, dhcp.Reservation{
			MAC: n.mac, IP: n.ip, Tag: nicTag(name, n.index),
		})
		if !n.primary {
			f.tags = append(f.tags, nicTag(name, n.index))
		}
	}

	for confName, f := range bySubnet {
		if err := dhcp.WriteReservations(dhcp.DefaultConfDir, confName, name, f.reservations); err != nil {
			return fmt.Errorf("write dhcp reservations on %s: %w", confName, err)
		}
		if err := dhcp.WriteVMOptions(dhcp.DefaultConfDir, confName, name, f.tags, dhcp.Config{
			InterfaceIP: net.ParseIP(f.nic.interfaceIP),
			VPCRoute:    f.nic.vpcCIDR,
		}); err != nil {
			return fmt.Errorf("write dhcp options on %s: %w", confName, err)
		}
	}
	return nil
}

// nicTag identifie une interface auprès de dnsmasq. Il est par interface et non
// par VM : deux interfaces d'une même VM peuvent partager un subnet, et n'y
// avoir pas le même rôle.
func nicTag(vmName string, index int) string {
	return fmt.Sprintf("%s-%d", vmName, index)
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
