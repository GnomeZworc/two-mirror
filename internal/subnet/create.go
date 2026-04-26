package subnet

import (
	"fmt"
	"net"
	"strconv"
	"strings"

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

	// lecture des paramètres depuis la DB
	vpcName, err := kv.GetFromDB(db, "subnet/"+subnetName+"/vpc")
	if err != nil {
		return fmt.Errorf("get vpc: %w", err)
	}

	vxlanIDStr, err := kv.GetFromDB(db, "subnet/"+subnetName+"/vxlan_id")
	if err != nil {
		return fmt.Errorf("get vxlan_id: %w", err)
	}
	vxlanID, err := strconv.Atoi(vxlanIDStr)
	if err != nil {
		return fmt.Errorf("parse vxlan_id: %w", err)
	}

	localIface, err := kv.GetFromDB(db, "subnet/"+subnetName+"/local_iface")
	if err != nil {
		return fmt.Errorf("get local_iface: %w", err)
	}

	gatewayIPStr, err := kv.GetFromDB(db, "subnet/"+subnetName+"/gateway_ip")
	if err != nil {
		return fmt.Errorf("get gateway_ip: %w", err)
	}
	gatewayIP := net.ParseIP(gatewayIPStr)
	if gatewayIP == nil {
		return fmt.Errorf("invalid gateway_ip: %s", gatewayIPStr)
	}

	cidr, err := kv.GetFromDB(db, "subnet/"+subnetName+"/cidr")
	if err != nil {
		return fmt.Errorf("get cidr: %w", err)
	}
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr: %w", err)
	}

	// subnet_id = partie après le premier '-' (ex: "sn-00001" -> "00001")
	subnetID := strings.SplitN(subnetName, "-", 2)[1]
	bridge := "br-" + subnetID
	vxlanIface := fmt.Sprintf("vxlan-%d", vxlanID)

	// veth pair
	if err := netif.CreateVethToNetns("v-"+subnetID+"-e", "v-"+subnetID+"-i", "/var/run/netns/"+vpcName, 1500); err != nil {
		return fmt.Errorf("create veth: %w", err)
	}

	// bridge dans le root netns
	if err := netif.CreateBridge(bridge, 1500); err != nil {
		return fmt.Errorf("create bridge: %w", err)
	}

	// bridge dans le netns VPC
	if err := netns.Call(vpcName, func() error {
		return netif.CreateBridge(bridge, 1500)
	}); err != nil {
		return fmt.Errorf("create bridge in netns: %w", err)
	}

	// vxlan
	if err := netif.CreateVxlan(vxlanIface, vxlanID, localIface, 1500); err != nil {
		return fmt.Errorf("create vxlan: %w", err)
	}

	// ajout des interfaces dans les bridges
	if err := netif.BridgeSetMaster("v-"+subnetID+"-e", bridge); err != nil {
		return fmt.Errorf("add veth-e to bridge: %w", err)
	}
	if err := netns.Call(vpcName, func() error {
		return netif.BridgeSetMaster("v-"+subnetID+"-i", bridge)
	}); err != nil {
		return fmt.Errorf("add veth-i to bridge in netns: %w", err)
	}
	if err := netif.BridgeSetMaster(vxlanIface, bridge); err != nil {
		return fmt.Errorf("add vxlan to bridge: %w", err)
	}

	// montée des interfaces dans le root netns
	for _, iface := range []string{"v-" + subnetID + "-e", vxlanIface, bridge} {
		if err := netif.LinkSetUp(iface); err != nil {
			return fmt.Errorf("set up %s: %w", iface, err)
		}
	}

	// montée des interfaces dans le netns VPC
	if err := netns.Call(vpcName, func() error {
		for _, iface := range []string{"v-" + subnetID + "-i", bridge} {
			if err := netif.LinkSetUp(iface); err != nil {
				return fmt.Errorf("set up %s: %w", iface, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("set up interfaces in netns: %w", err)
	}

	// IP gateway (/32) sur le bridge interne
	if err := netns.Call(vpcName, func() error {
		return netif.AddrAdd(bridge, gatewayIP)
	}); err != nil {
		return fmt.Errorf("add addr to bridge in netns: %w", err)
	}

	// route subnet (scope link) dans le netns VPC
	if err := netns.Call(vpcName, func() error {
		return netif.RouteAdd(bridge, subnet)
	}); err != nil {
		return fmt.Errorf("add route in netns: %w", err)
	}

	if err := ebtables.DropARPToGateway(bridge, gatewayIP.String()); err != nil {
		return err
	}
	if err := ebtables.DropDHCP(bridge); err != nil {
		return err
	}

	// génération de la config dnsmasq et démarrage du service
	conf := dhcp.Config{
		Network: subnet,
		Gateway: gatewayIP,
		Name:    vpcName + "_" + bridge,
		ConfDir: "/etc/dnsmasq.d",
	}
	if _, err := dhcp.GenerateConfig(conf); err != nil {
		return fmt.Errorf("generate dhcp config: %w", err)
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
