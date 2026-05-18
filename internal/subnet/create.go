package subnet

import (
	"fmt"

	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/internal/ebtables"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"git.g3e.fr/syonad/two/pkg/systemd"

	"github.com/dgraph-io/badger/v4"
)

func CreateSubnet(db *badger.DB, subnetName string) error {
	state, err := kv.GetFromDB(db, "subnet/"+subnetName+"/state")
	if err != nil {
		return err
	}
	if state != "creating" {
		return nil
	}

	d, err := loadSubnet(db, subnetName)
	if err != nil {
		return err
	}

	vxlanIface := fmt.Sprintf("vxlan-%d", d.vxlanID)

	if err := netif.CreateVethToNetns("v-"+d.subnetID+"-e", "v-"+d.subnetID+"-i", "/var/run/netns/"+d.vpc, 1500); err != nil {
		return fmt.Errorf("create veth: %w", err)
	}

	if err := netif.CreateBridge(d.bridge, 1500); err != nil {
		return fmt.Errorf("create bridge: %w", err)
	}

	if err := netns.Call(d.vpc, func() error {
		return netif.CreateBridge(d.bridge, 1500)
	}); err != nil {
		return fmt.Errorf("create bridge in netns: %w", err)
	}

	if err := netif.CreateVxlan(vxlanIface, d.vxlanID, d.localIface, 1500); err != nil {
		return fmt.Errorf("create vxlan: %w", err)
	}

	if err := netif.BridgeSetMaster("v-"+d.subnetID+"-e", d.bridge); err != nil {
		return fmt.Errorf("add veth-e to bridge: %w", err)
	}
	if err := netns.Call(d.vpc, func() error {
		return netif.BridgeSetMaster("v-"+d.subnetID+"-i", d.bridge)
	}); err != nil {
		return fmt.Errorf("add veth-i to bridge in netns: %w", err)
	}
	if err := netif.BridgeSetMaster(vxlanIface, d.bridge); err != nil {
		return fmt.Errorf("add vxlan to bridge: %w", err)
	}

	for _, iface := range []string{"v-" + d.subnetID + "-e", vxlanIface, d.bridge} {
		if err := netif.LinkSetUp(iface); err != nil {
			return fmt.Errorf("set up %s: %w", iface, err)
		}
	}

	if err := netns.Call(d.vpc, func() error {
		for _, iface := range []string{"v-" + d.subnetID + "-i", d.bridge} {
			if err := netif.LinkSetUp(iface); err != nil {
				return fmt.Errorf("set up %s: %w", iface, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("set up interfaces in netns: %w", err)
	}

	if err := netns.Call(d.vpc, func() error {
		return netif.AddrAdd(d.bridge, d.gatewayIP)
	}); err != nil {
		return fmt.Errorf("add addr to bridge in netns: %w", err)
	}

	if err := netns.Call(d.vpc, func() error {
		return netif.RouteAdd(d.bridge, d.cidr)
	}); err != nil {
		return fmt.Errorf("add route in netns: %w", err)
	}

	if err := ebtables.DropARPToGateway(d.bridge, d.gatewayIP.String()); err != nil {
		return err
	}
	if err := ebtables.DropDHCP(d.bridge); err != nil {
		return err
	}

	conf := dhcp.Config{
		Network: d.cidr,
		Gateway: d.gatewayIP,
		Name:    d.vpc + "_" + d.bridge,
		ConfDir: "/etc/dnsmasq.d",
	}
	_, entries, err := dhcp.GenerateConfig(conf)
	if err != nil {
		return fmt.Errorf("generate dhcp config: %w", err)
	}
	if err := dhcp.StoreDHCPEntries(db, subnetName, entries); err != nil {
		return fmt.Errorf("store dhcp entries: %w", err)
	}

	svc, err := systemd.New()
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer svc.Close()

	if err := svc.Start("dnsmasq@" + conf.Name + ".service"); err != nil {
		return fmt.Errorf("start dnsmasq: %w", err)
	}

	return kv.AddInDB(db, "subnet/"+subnetName+"/state", "created")
}
