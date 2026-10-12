package l3vni

import (
	"fmt"

	"git.g3e.fr/syonad/two/internal/frr"
)

func Valid(id uint32) error {
	if id == 0 || id > frr.MaxL3VNI {
		return fmt.Errorf("l3vni %d out of range 1-%d: the route distinguisher <vtep>:<l3vni> holds 16 bits", id, frr.MaxL3VNI)
	}
	if id >= 253 && id <= 255 {
		return fmt.Errorf("l3vni %d is reserved: it is also the routing table id, and tables 253-255 belong to the kernel", id)
	}
	return nil
}
