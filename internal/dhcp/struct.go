package dhcp

import (
	"net"
)

type Config struct {
	Network      *net.IPNet
	Gateway      net.IP
	DefaultRoute bool
	Name         string
	ConfDir      string
}
