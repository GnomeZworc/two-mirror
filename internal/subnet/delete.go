package subnet

import (
	"fmt"
	"os"
	"strings"

	"git.g3e.fr/syonad/two/internal/ebtables"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"git.g3e.fr/syonad/two/pkg/systemd"

	"github.com/dgraph-io/badger/v4"
)

func DeleteSubnet(db *badger.DB, subnetName string) error {
	state, err := kv.GetFromDB(db, "subnet/"+subnetName+"/state")
	if err != nil {
		return err
	}
	if state != "deleting" {
		return nil
	}

	vpcName, err := kv.GetFromDB(db, "subnet/"+subnetName+"/vpc")
	if err != nil {
		return fmt.Errorf("get vpc: %w", err)
	}

	vxlanIDStr, err := kv.GetFromDB(db, "subnet/"+subnetName+"/vxlan_id")
	if err != nil {
		return fmt.Errorf("get vxlan_id: %w", err)
	}

	gatewayIP, err := kv.GetFromDB(db, "subnet/"+subnetName+"/gateway_ip")
	if err != nil {
		return fmt.Errorf("get gateway_ip: %w", err)
	}

	subnetID := strings.SplitN(subnetName, "-", 2)[1]
	bridge := "br-" + subnetID
	vxlanIface := "vxlan-" + vxlanIDStr

	// arrêt du service dnsmasq
	svc, err := systemd.New()
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer svc.Close()

	svcName := "dnsmasq@" + vpcName + "_" + bridge + ".service"
	if err := svc.Stop(svcName); err != nil {
		return fmt.Errorf("stop dnsmasq: %w", err)
	}

	// suppression de la config dnsmasq
	if err := os.Remove("/etc/dnsmasq.d/" + vpcName + "_" + bridge + ".conf"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove dnsmasq config: %w", err)
	}

	// suppression des règles ebtables
	if err := ebtables.DeleteARPToGateway(bridge, gatewayIP); err != nil {
		return fmt.Errorf("delete ebtables arp rule: %w", err)
	}
	if err := ebtables.DeleteDHCP(bridge); err != nil {
		return fmt.Errorf("delete ebtables dhcp rule: %w", err)
	}

	// suppression du bridge dans le netns VPC
	if err := netns.Call(vpcName, func() error {
		return netif.DeleteLink(bridge)
	}); err != nil {
		return fmt.Errorf("delete bridge in netns: %w", err)
	}

	// suppression du vxlan
	if err := netif.DeleteLink(vxlanIface); err != nil {
		return fmt.Errorf("delete vxlan: %w", err)
	}

	// suppression du veth pair (supprime les deux côtés)
	if err := netif.DeleteLink("v-" + subnetID + "-e"); err != nil {
		return fmt.Errorf("delete veth: %w", err)
	}

	// suppression du bridge dans le root netns
	if err := netif.DeleteLink(bridge); err != nil {
		return fmt.Errorf("delete bridge: %w", err)
	}

	return kv.AddInDB(db, "subnet/"+subnetName+"/state", "deleted")
}
