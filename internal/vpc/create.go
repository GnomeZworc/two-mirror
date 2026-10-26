package vpc

import (
	"strings"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/l3vni"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/state"

	"github.com/dgraph-io/badger/v4"
)

func CreateVPC(db *badger.DB, name string, cfg *configuration.Config) error {
	if current, err := state.Get(db, "vpc/"+name); err != nil {
		return err
	} else if current == state.Creating {
		vpcID := strings.SplitN(name, "-", 2)[1]

		if err := netns.Create(name); err != nil {
			return err
		}

		if err := netif.CreateVethToNetns("vp-"+vpcID+"-e", "vp-"+vpcID+"-i", "/var/run/netns/"+name, 9000); err != nil {
			return err
		}

		if err := netns.Call(name, func() error {
			return netif.CreateBridge("br-public", 1500, nil)
		}); err != nil {
			return err
		}

		if err := netif.BridgeSetMaster("vp-"+vpcID+"-e", cfg.PublicBridge()); err != nil {
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
		if err := l3vni.Create(db, name, cfg); err != nil {
			return err
		}
		if err := setupHostSvc(name, newSystemHostSvc()); err != nil {
			return err
		}
		return state.Set(db, "vpc/"+name, state.Running)
	}
	return nil
}
