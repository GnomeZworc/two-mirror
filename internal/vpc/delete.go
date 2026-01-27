package vpc

import (
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func DeleteVPC(db *badger.DB, name string) error {
	if state, err := kv.GetFromDB(db, "vpc/"+name+"/state"); err != nil {
		return err
	} else if state == "deleting" {
		if err := netif.DeleteLink(name + "-ext"); err != nil {
			return err
		}

		if err := netns.Delete(name); err != nil {
			return err
		}
		kv.AddInDB(db, "vpc/"+name+"/state", "deleted")
	}

	return nil
}
