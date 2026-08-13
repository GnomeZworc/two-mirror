package vpc

import (
	"strings"

	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/state"

	"github.com/dgraph-io/badger/v4"
)

func DeleteVPC(db *badger.DB, name string) error {
	if current, err := state.Get(db, "vpc/"+name); err != nil {
		return err
	} else if current == state.Deleting {
		vpcID := strings.SplitN(name, "-", 2)[1]

		if err := netif.DeleteLink("vp-" + vpcID + "-e"); err != nil {
			return err
		}

		if err := netns.Delete(name); err != nil {
			return err
		}
		return state.Set(db, "vpc/"+name, state.Deleted)
	}

	return nil
}
