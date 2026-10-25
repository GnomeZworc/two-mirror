package vpc

import (
	"fmt"
	"net"

	hostsvcapi "git.g3e.fr/syonad/two/internal/api/hostsvc"
	hostsvcclient "git.g3e.fr/syonad/two/internal/client/hostsvc"
	"git.g3e.fr/syonad/two/internal/hostsvc"
	"git.g3e.fr/syonad/two/internal/iptables"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
)

const HostSvcLink = "hostsvc"

type hostSvcHost interface {
	InNetns(vpc string, fn func() error) error
	CreateDummy(name string) error
	AddrAdd(iface string, ip net.IP) error
	AddRedirect(target string) error
	Open(vpc string) error
	Close(vpc string) error
}

type systemHostSvc struct {
	client *hostsvcclient.Client
}

func newSystemHostSvc() systemHostSvc {
	return systemHostSvc{client: hostsvcclient.New(hostsvcapi.SocketPath(hostsvcapi.DefaultRunDir))}
}

func (systemHostSvc) InNetns(vpc string, fn func() error) error { return netns.Call(vpc, fn) }
func (systemHostSvc) CreateDummy(name string) error             { return netif.CreateDummy(name) }
func (systemHostSvc) AddrAdd(iface string, ip net.IP) error     { return netif.AddrAdd(iface, ip) }
func (systemHostSvc) AddRedirect(target string) error           { return iptables.AddHostSvcRedirect(target) }
func (s systemHostSvc) Open(vpc string) error                   { return s.client.Open(vpc) }
func (s systemHostSvc) Close(vpc string) error                  { return s.client.Close(vpc) }

func setupHostSvc(vpc string, h hostSvcHost) error {
	ip := net.ParseIP(hostsvc.ServiceIP)
	if err := h.InNetns(vpc, func() error {
		steps := []struct {
			name string
			fn   func() error
		}{
			{"create " + HostSvcLink, func() error { return h.CreateDummy(HostSvcLink) }},
			{"address " + hostsvc.ServiceIP + " on " + HostSvcLink, func() error { return h.AddrAdd(HostSvcLink, ip) }},
			{"redirect dns and ntp to " + hostsvc.ServiceIP, func() error { return h.AddRedirect(hostsvc.ServiceIP) }},
		}
		for _, step := range steps {
			if err := step.fn(); err != nil {
				return fmt.Errorf("%s: %w", step.name, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("hostsvc in %s: %w", vpc, err)
	}
	if err := h.Open(vpc); err != nil {
		return fmt.Errorf("hostsvc open %s: %w", vpc, err)
	}
	return nil
}

func teardownHostSvc(vpc string, h hostSvcHost) error {
	if err := h.Close(vpc); err != nil {
		return fmt.Errorf("hostsvc close %s: %w", vpc, err)
	}
	return nil
}
