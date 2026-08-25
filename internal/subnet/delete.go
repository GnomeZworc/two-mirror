package subnet

import (
	"fmt"
	"os"
	"path/filepath"

	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/internal/ebtables"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"git.g3e.fr/syonad/two/pkg/systemd"

	"github.com/dgraph-io/badger/v4"
)

func DeleteSubnet(db *badger.DB, subnetName string) error {
	current, err := state.Get(db, "subnet/"+subnetName)
	if err != nil {
		return err
	}
	if current != state.Deleting {
		return nil
	}

	d, err := loadSubnet(db, subnetName)
	if err != nil {
		return err
	}

	if err := stopDHCP(db, subnetName, d); err != nil {
		return err
	}

	switch d.mode {
	case "vxlan":
		if err := deleteSubnetVxlan(d); err != nil {
			return err
		}
	case "bridge":
		if err := deleteSubnetBridge(d); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown subnet mode %q", d.mode)
	}

	return state.Set(db, "subnet/"+subnetName, state.Deleted)
}

func stopDHCP(db *badger.DB, subnetName string, d subnetData) error {
	svc, err := systemd.New()
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer svc.Close()

	svcName := "dnsmasq@" + d.vpc + "_" + d.bridge + ".service"
	if status, err := svc.Status(svcName); err == nil && status.ActiveState == "active" {
		if err := svc.Stop(svcName); err != nil {
			return fmt.Errorf("stop dnsmasq: %w", err)
		}
	}

	if err := os.Remove(filepath.Join(dhcp.DefaultConfDir, d.vpc+"_"+d.bridge+".conf")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove dnsmasq config: %w", err)
	}

	if err := dhcp.RemoveSubnetDirs(dhcp.DefaultConfDir, d.vpc+"_"+d.bridge); err != nil {
		return fmt.Errorf("remove dnsmasq dirs: %w", err)
	}

	if err := kv.DeleteInDB(db, "subnet/"+subnetName+"/dhcp"); err != nil {
		return fmt.Errorf("delete dhcp entries: %w", err)
	}
	return nil
}

func deleteSubnetVxlan(d subnetData) error {
	vxlanIface := fmt.Sprintf("vxlan-%d", d.vxlanID)
	vethI := "v-" + d.subnetID + "-i"

	if err := netns.Call(d.vpc, func() error {
		if err := ebtables.DeleteARPToGateway(vethI, d.interfaceIP.String()); err != nil {
			return fmt.Errorf("delete ebtables arp rule: %w", err)
		}
		if err := ebtables.DeleteDHCP(vethI, d.interfaceIP.String()); err != nil {
			return fmt.Errorf("delete ebtables dhcp rule: %w", err)
		}
		return netif.DeleteLink(d.bridge)
	}); err != nil {
		return fmt.Errorf("delete netns resources: %w", err)
	}

	if err := netif.DeleteLink(vxlanIface); err != nil {
		return fmt.Errorf("delete vxlan: %w", err)
	}

	if err := netif.DeleteLink("v-" + d.subnetID + "-e"); err != nil {
		return fmt.Errorf("delete veth: %w", err)
	}

	if err := netif.DeleteLink(d.bridge); err != nil {
		return fmt.Errorf("delete bridge: %w", err)
	}
	return nil
}

func deleteSubnetBridge(d subnetData) error {
	vethI := "v-" + d.subnetID + "-i"

	if err := netns.Call(d.vpc, func() error {
		if err := ebtables.DeleteDHCP(vethI, d.interfaceIP.String()); err != nil {
			return fmt.Errorf("delete ebtables dhcp rule: %w", err)
		}
		return netif.DeleteLink(d.bridge)
	}); err != nil {
		return fmt.Errorf("delete netns resources: %w", err)
	}

	if err := netif.DeleteLink("v-" + d.subnetID + "-e"); err != nil {
		return fmt.Errorf("delete veth: %w", err)
	}
	return nil
}
