package vpc

import (
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
)

func CreateVPC(name string) error {
	// missing
	// search data in db
	//  change state in db

	// create netns
	if err := netns.Create(name); err != nil {
		return err
	}

	// create veth public for this netns
	if err := netif.CreateVethToNetns(name+"-ext", "veth-"+name+"-int", "/var/run/netns/"+name, 9000); err != nil {
		return err
	}

	// create public bridge in netns
	if err := netns.Call(name, func() error {
		return netif.CreateBridge("br-public", 1500, false)
	}); err != nil {
		return err
	}

	// set veth to ext public bridge
	if err := netif.BridgeSetMaster(name+"-ext", "br-public"); err != nil {
		return err
	}

	// set veth to int public bridge
	if err := netns.Call(name, func() error {
		return netif.BridgeSetMaster("veth-"+name+"-int", "br-public")
	}); err != nil {
		return err
	}

	// set set ext veth up
	if err := netif.LinkSetUp(name + "-ext"); err != nil {
		return nil
	}
	// set set int veth up
	if err := netns.Call(name, func() error {
		return netif.LinkSetUp("veth-" + name + "-int")
	}); err != nil {
		return err
	}
	// set set int bridge up
	if err := netns.Call(name, func() error {
		return netif.LinkSetUp("br-public")
	}); err != nil {
		return err
	}

	return nil
}
