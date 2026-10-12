package l3vni

import "fmt"

const maxL3VNI = 1<<24 - 1

func Valid(id uint32) error {
	if id == 0 || id > maxL3VNI {
		return fmt.Errorf("l3vni %d does not fit in 24 bits (1-%d)", id, maxL3VNI)
	}
	if id >= 253 && id <= 255 {
		return fmt.Errorf("l3vni %d is reserved: it is also the routing table id, and tables 253-255 belong to the kernel", id)
	}
	return nil
}
