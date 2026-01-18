package vpc

import (
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
)

func DeleteVPC(name string) error {
	if err := netif.DeleteLink(name + "-ext"); err != nil {
		return err
	}

	if err := netns.Delete(name); err != nil {
		return err
	}

	return nil
}
