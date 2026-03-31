package dhcp

import (
	"net"
)

type Config struct {
	Network *net.IPNet
	Gateway net.IP
	Name    string
	ConfDir string
}
