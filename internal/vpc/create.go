package vpc

import (
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func CreateVPC(db *badger.DB, name string) error {
	// missing
	// search data in db
	//  change state in db

	// create netns
	if state, err := kv.GetFromDB(db, "vpc/"+name+"/state"); err != nil {
		return err
	} else if state == "creating" {
		if err := netns.Create(name); err != nil {
			return err
		}

		// create veth public for this netns
		if err := netif.CreateVethToNetns("veth"+name+"ext", "vethpublicint", "/var/run/netns/"+name, 9000); err != nil {
			return err
		}

		// create public bridge in netns
		if err := netns.Call(name, func() error {
			return netif.CreateBridge("br-public", 1500)
		}); err != nil {
			return err
		}

		// set veth to ext public bridge
		if err := netif.BridgeSetMaster("veth"+name+"ext", "br-public"); err != nil {
			return err
		}

		// set veth to int public bridge
		if err := netns.Call(name, func() error {
			return netif.BridgeSetMaster("vethpublicint", "br-public")
		}); err != nil {
			return err
		}

		// set set ext veth up
		if err := netif.LinkSetUp("veth" + name + "ext"); err != nil {
			return nil
		}
		// set set int veth up
		if err := netns.Call(name, func() error {
			return netif.LinkSetUp("vethpublicint")
		}); err != nil {
			return err
		}
		kv.AddInDB(db, "vpc/"+name+"/state", "created")
	}
	return nil
}
