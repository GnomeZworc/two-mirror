package subnet

import (
	"fmt"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/internal/dhcpbackend"
	"git.g3e.fr/syonad/two/internal/ebtables"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/state"

	"github.com/dgraph-io/badger/v4"
)

func CreateSubnet(db *badger.DB, subnetName string, cfg *configuration.Config) error {
	current, err := state.Get(db, "subnet/"+subnetName)
	if err != nil {
		return err
	}
	if current != state.Creating {
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

	if err := createSubnet(db, subnetName, d, backend); err != nil {
		return err
	}

	return state.Set(db, "subnet/"+subnetName, state.Running)
}

func createSubnet(db *badger.DB, subnetName string, d subnetData, backend dhcpbackend.Backend) error {
	vethE := "v-" + d.subnetID + "-e"
	vethI := "v-" + d.subnetID + "-i"

	if err := netif.CreateVethToNetns(vethE, vethI, "/var/run/netns/"+d.vpc, 1500); err != nil {
		return fmt.Errorf("create veth: %w", err)
	}

	switch d.mode {
	case ModeVxlan:
		if err := setupVxlanHost(d, vethE); err != nil {
			return err
		}
	case ModePublicIP:
		return fmt.Errorf("subnet mode %q: host network setup is not implemented yet", d.mode)
	case ModeBridge:
		if err := netif.BridgeSetMaster(vethE, d.localIface); err != nil {
			return fmt.Errorf("add veth-e to bridge: %w", err)
		}
		if err := netif.LinkSetUp(vethE); err != nil {
			return fmt.Errorf("set up %s: %w", vethE, err)
		}
	default:
		return fmt.Errorf("unknown subnet mode %q", d.mode)
	}

	if err := netns.Call(d.vpc, func() error {
		if err := netif.CreateBridge(d.bridge, 1500); err != nil {
			return fmt.Errorf("create bridge: %w", err)
		}
		return netif.BridgeSetMaster(vethI, d.bridge)
	}); err != nil {
		return fmt.Errorf("setup bridge in netns: %w", err)
	}

	if err := netns.Call(d.vpc, func() error {
		for _, iface := range []string{vethI, d.bridge} {
			if err := netif.LinkSetUp(iface); err != nil {
				return fmt.Errorf("set up %s: %w", iface, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("set up interfaces in netns: %w", err)
	}

	switch d.mode {
	case "vxlan":
		if err := netns.Call(d.vpc, func() error {
			if err := netif.AddrAdd(d.bridge, d.interfaceIP); err != nil {
				return fmt.Errorf("add addr: %w", err)
			}
			if err := netif.RouteAdd(d.bridge, d.cidr); err != nil {
				return fmt.Errorf("add route: %w", err)
			}
			if err := ebtables.DropARPToGateway(vethI, d.interfaceIP.String()); err != nil {
				return err
			}
			return ebtables.DropDHCP(vethI, d.interfaceIP.String())
		}); err != nil {
			return fmt.Errorf("configure netns: %w", err)
		}
	case "bridge":
		if err := netns.Call(d.vpc, func() error {
			if err := netif.AddrAdd(d.bridge, d.interfaceIP); err != nil {
				return fmt.Errorf("add addr: %w", err)
			}
			if err := netif.RouteAdd(d.bridge, d.cidr); err != nil {
				return fmt.Errorf("add route: %w", err)
			}
			return ebtables.DropDHCP(vethI, d.interfaceIP.String())
		}); err != nil {
			return fmt.Errorf("configure netns: %w", err)
		}
	}

	return startDHCP(db, subnetName, d, backend)
}

func setupVxlanHost(d subnetData, vethE string) error {
	vxlanIface := fmt.Sprintf("vxlan-%d", d.vxlanID)

	if err := netif.CreateBridge(d.bridge, 1500); err != nil {
		return fmt.Errorf("create bridge: %w", err)
	}
	if err := netif.CreateVxlan(vxlanIface, d.vxlanID, d.localIface, 1500); err != nil {
		return fmt.Errorf("create vxlan: %w", err)
	}
	if err := netif.BridgeSetMaster(vethE, d.bridge); err != nil {
		return fmt.Errorf("add veth-e to bridge: %w", err)
	}
	if err := netif.BridgeSetMaster(vxlanIface, d.bridge); err != nil {
		return fmt.Errorf("add vxlan to bridge: %w", err)
	}
	for _, iface := range []string{vethE, vxlanIface, d.bridge} {
		if err := netif.LinkSetUp(iface); err != nil {
			return fmt.Errorf("set up %s: %w", iface, err)
		}
	}
	return nil
}

func startDHCP(db *badger.DB, subnetName string, d subnetData, backend dhcpbackend.Backend) error {
	defaultGateway, vpcRoute, err := dhcpRouting(d, netif.GetDefaultGateway)
	if err != nil {
		return err
	}

	if err := dhcp.StoreDHCPEntries(db, subnetName, dhcp.Entries(d.cidr)); err != nil {
		return fmt.Errorf("store dhcp entries: %w", err)
	}

	return backend.ConfigureSubnet(dhcpbackend.Subnet{
		Name:           subnetName,
		VPC:            d.vpc,
		Bridge:         d.bridge,
		Network:        d.cidr,
		InterfaceIP:    d.interfaceIP,
		VPCRoute:       vpcRoute,
		DefaultGateway: defaultGateway,
	})
}
