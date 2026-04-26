package subnet

import (
	"fmt"
	"os"

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

	d, err := loadSubnet(db, subnetName)
	if err != nil {
		return err
	}

	vxlanIface := fmt.Sprintf("vxlan-%d", d.vxlanID)

	svc, err := systemd.New()
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer svc.Close()

	if err := svc.Stop("dnsmasq@" + d.vpc + "_" + d.bridge + ".service"); err != nil {
		return fmt.Errorf("stop dnsmasq: %w", err)
	}

	if err := os.Remove("/etc/dnsmasq.d/" + d.vpc + "_" + d.bridge + ".conf"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove dnsmasq config: %w", err)
	}
	if err := kv.DeleteInDB(db, "subnet/"+subnetName+"/dhcp"); err != nil {
		return fmt.Errorf("delete dhcp entries: %w", err)
	}

	if err := ebtables.DeleteARPToGateway(d.bridge, d.gatewayIP.String()); err != nil {
		return fmt.Errorf("delete ebtables arp rule: %w", err)
	}
	if err := ebtables.DeleteDHCP(d.bridge); err != nil {
		return fmt.Errorf("delete ebtables dhcp rule: %w", err)
	}

	if err := netns.Call(d.vpc, func() error {
		return netif.DeleteLink(d.bridge)
	}); err != nil {
		return fmt.Errorf("delete bridge in netns: %w", err)
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

	return kv.AddInDB(db, "subnet/"+subnetName+"/state", "deleted")
}
