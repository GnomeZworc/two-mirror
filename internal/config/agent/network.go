package configuration

import "fmt"

const (
	MTUVM         = 1500
	MinMTUVM      = 1280
	MTUUnderlay   = 9000
	VXLANOverhead = 50
)

type Network struct {
	MTUVM                     int  `mapstructure:"mtu_vm"`
	AllowMTUVMBelow1500       bool `mapstructure:"allow_mtu_vm_below_1500"`
	AllowMTUUnderlayBelow9000 bool `mapstructure:"allow_mtu_underlay_below_9000"`
}

func (n Network) VM() int {
	if n.MTUVM == 0 {
		return MTUVM
	}
	return n.MTUVM
}

func (n Network) Validate() error {
	switch {
	case n.MTUVM > MTUVM:
		return fmt.Errorf("network.mtu_vm %d is above %d", n.MTUVM, MTUVM)
	case n.MTUVM < MinMTUVM:
		return fmt.Errorf("network.mtu_vm %d is below the minimum %d", n.MTUVM, MinMTUVM)
	case n.MTUVM < MTUVM && !n.AllowMTUVMBelow1500:
		return fmt.Errorf("network.mtu_vm %d is below %d: set network.allow_mtu_vm_below_1500 to allow it", n.MTUVM, MTUVM)
	}
	return nil
}

func (n Network) CheckUnderlay(iface string, mtu int) error {
	if mtu != MTUUnderlay && !n.AllowMTUUnderlayBelow9000 {
		return fmt.Errorf("underlay %s has mtu %d, %d expected: set network.allow_mtu_underlay_below_9000 to allow another value", iface, mtu, MTUUnderlay)
	}
	if mtu < n.VM()+VXLANOverhead {
		return fmt.Errorf("underlay %s has mtu %d, below network.mtu_vm %d plus %d bytes of vxlan header", iface, mtu, n.VM(), VXLANOverhead)
	}
	return nil
}

func (n Network) CheckBridge(iface string, mtu int) error {
	if mtu < n.VM() {
		return fmt.Errorf("bridge %s has mtu %d, below network.mtu_vm %d", iface, mtu, n.VM())
	}
	return nil
}
