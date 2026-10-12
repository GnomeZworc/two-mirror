package l3vni

import (
	"fmt"

	"git.g3e.fr/syonad/two/internal/netif"
)

func Valid(id uint32) error {
	if id == 0 || id > netif.MaxVNI {
		return fmt.Errorf("l3vni %d does not fit in 24 bits (1-%d)", id, netif.MaxVNI)
	}
	if id >= 253 && id <= 255 {
		return fmt.Errorf("l3vni %d is reserved: it is also the routing table id, and tables 253-255 belong to the kernel", id)
	}
	return nil
}
