package dhcpapi

import (
	"path/filepath"
)

const (
	DefaultRunDir = "/run/two/dhcp"
	SocketExt     = ".sock"
	StateExt      = ".state"
	UnitExt       = ".service"
	UnitName      = "dhcp@"
)

func Instance(vpc, bridge string) string {
	return vpc + "_" + bridge
}

func Unit(instance string) string {
	return UnitName + instance + UnitExt
}

func SocketPath(runDir, instance string) string {
	return filepath.Join(runDir, instance+SocketExt)
}

func StatePath(runDir, instance string) string {
	return filepath.Join(runDir, instance+StateExt)
}
