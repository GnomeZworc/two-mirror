package subnet

import (
	"fmt"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/dhcpbackend"
	"git.g3e.fr/syonad/two/internal/ebtables"
	"git.g3e.fr/syonad/two/internal/l3vni"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/state"

	"github.com/dgraph-io/badger/v4"
)

func DeleteSubnet(db *badger.DB, subnetName string, cfg *configuration.Config) error {
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

	backend, err := dhcpbackend.New(cfg)
	if err != nil {
		return err
	}

	if d.mode == ModeVxlan {
		if err := l3vni.WithdrawRoute(db, d.vpc, d.cidr); err != nil {
			return err
		}
	}

	if err := stopDHCP(subnetName, d, backend); err != nil {
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

func stopDHCP(subnetName string, d subnetData, backend dhcpbackend.Backend) error {
	return backend.TeardownSubnet(dhcpbackend.Subnet{
		Name:   subnetName,
		VPC:    d.vpc,
		Bridge: d.bridge,
	})
}

func deleteSubnetVxlan(d subnetData) error {
	vxlanIface := fmt.Sprintf("vxlan-%d", d.vxlanID)
	vethI := "v-" + d.subnetID + "-i"

	if d.anycast() {
		if err := deleteAnycastGateway(d, vethI); err != nil {
			return fmt.Errorf("delete netns resources: %w", err)
		}
	} else if err := netns.Call(d.vpc, func() error {
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
		if err := ebtables.DeleteConfineGateway(vethI, d.interfaceIP.String()); err != nil {
			return fmt.Errorf("delete ebtables gateway rules: %w", err)
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
