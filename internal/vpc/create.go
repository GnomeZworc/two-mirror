package vpc

import (
	"strings"

	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func CreateVPC(db *badger.DB, name string) error {
	if state, err := kv.GetFromDB(db, "vpc/"+name+"/state"); err != nil {
		return err
	} else if state == "creating" {
		vpcID := strings.SplitN(name, "-", 2)[1]

		if err := netns.Create(name); err != nil {
			return err
		}

		if err := netif.CreateVethToNetns("vp-"+vpcID+"-e", "vp-"+vpcID+"-i", "/var/run/netns/"+name, 9000); err != nil {
			return err
		}

		if err := netns.Call(name, func() error {
			return netif.CreateBridge("br-public", 1500)
		}); err != nil {
			return err
		}

		if err := netif.BridgeSetMaster("vp-"+vpcID+"-e", "br-public"); err != nil {
			return err
		}

		if err := netns.Call(name, func() error {
			return netif.BridgeSetMaster("vp-"+vpcID+"-i", "br-public")
		}); err != nil {
			return err
		}

		if err := netif.LinkSetUp("vp-" + vpcID + "-e"); err != nil {
			return err
		}
		if err := netns.Call(name, func() error {
			return netif.LinkSetUp("vp-" + vpcID + "-i")
		}); err != nil {
			return err
		}
		kv.AddInDB(db, "vpc/"+name+"/state", "created")
	}
	return nil
}
