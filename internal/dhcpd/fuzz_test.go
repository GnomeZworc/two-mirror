package dhcpd

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func FuzzBuildReply(f *testing.F) {
	mac, err := net.ParseMAC("00:22:33:00:00:0a")
	if err != nil {
		f.Fatalf("ParseMAC: %v", err)
	}
	for _, kind := range []dhcpv4.MessageType{
		dhcpv4.MessageTypeDiscover,
		dhcpv4.MessageTypeRequest,
		dhcpv4.MessageTypeRelease,
		dhcpv4.MessageTypeDecline,
	} {
		req, err := dhcpv4.New(dhcpv4.WithMessageType(kind), dhcpv4.WithHwAddr(mac))
		if err != nil {
			f.Fatalf("New request: %v", err)
		}
		f.Add(req.ToBytes())
	}

	_, network, err := net.ParseCIDR("10.0.5.0/24")
	if err != nil {
		f.Fatalf("ParseCIDR: %v", err)
	}
	_, vpcRoute, err := net.ParseCIDR("10.0.0.0/16")
	if err != nil {
		f.Fatalf("ParseCIDR: %v", err)
	}

	c := SubnetConfig{
		Network:        network,
		InterfaceIP:    net.ParseIP("10.0.5.1"),
		VPCRoute:       vpcRoute,
		DefaultGateway: net.ParseIP("10.0.5.254"),
	}
	h := Host{MAC: mac, IP: net.ParseIP("10.0.5.10"), VM: "vm-fuzz", DefaultRoute: true}

	f.Fuzz(func(t *testing.T, raw []byte) {
		req, err := dhcpv4.FromBytes(raw)
		if err != nil {
			return
		}
		reply, err := BuildReply(c, h, req)
		if err != nil {
			return
		}
		if reply == nil {
			t.Fatal("nil reply without an error")
		}
		reply.ToBytes()
	})
}
