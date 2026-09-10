package dhcpbackend

import (
	"fmt"
	"net"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
)

type Subnet struct {
	Name           string
	VPC            string
	Bridge         string
	Network        *net.IPNet
	InterfaceIP    net.IP
	VPCRoute       *net.IPNet
	DefaultGateway net.IP
}

func (s Subnet) Instance() string {
	return s.VPC + "_" + s.Bridge
}

type Reservation struct {
	Index        int
	MAC          string
	IP           string
	DefaultRoute bool
}

type Backend interface {
	Unit(s Subnet) string
	ConfigureSubnet(s Subnet) error
	TeardownSubnet(s Subnet) error
	SetVM(s Subnet, vmName string, res []Reservation) error
	DelVM(s Subnet, vmName string, res []Reservation) error
}

func New(cfg *configuration.Config) (Backend, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required to pick a dhcp backend")
	}
	if err := configuration.ValidBackend(cfg.DHCP.Backend); err != nil {
		return nil, err
	}

	switch cfg.DHCP.Backend {
	case configuration.BackendTwo:
		return Two{}, nil
	default:
		return Dnsmasq{}, nil
	}
}
