package dhcpapi

import (
	"net"
	"testing"

	"git.g3e.fr/syonad/two/internal/dhcpd"
	"github.com/insomniacslk/dhcp/dhcpv4"
)

func TestSubnet_MTUSurvivesTheConversions(t *testing.T) {
	c, err := Subnet{Network: "10.0.5.0/24", InterfaceIP: "10.0.5.1", MTU: 1450}.toConfig()
	if err != nil {
		t.Fatalf("toConfig : %v", err)
	}
	if c.MTU != 1450 {
		t.Errorf("toConfig : mtu %d", c.MTU)
	}
	if got := subnetFromConfig(c).MTU; got != 1450 {
		t.Errorf("subnetFromConfig : mtu %d", got)
	}
	canonical, err := CanonicalSubnet(Subnet{Network: "10.0.5.0/24", InterfaceIP: "10.0.5.1", MTU: 1450})
	if err != nil || canonical.MTU != 1450 {
		t.Errorf("CanonicalSubnet : %+v, %v", canonical, err)
	}
}

func TestLeaseFromReply_ShowsTheMTU(t *testing.T) {
	_, network, _ := net.ParseCIDR("10.0.5.0/24")
	mac, _ := net.ParseMAC("00:22:33:00:00:0a")
	c := dhcpd.SubnetConfig{Network: network, InterfaceIP: net.ParseIP("10.0.5.1"), MTU: 1450}
	h := dhcpd.Host{MAC: mac, IP: net.ParseIP("10.0.5.10")}
	req, err := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest), dhcpv4.WithHwAddr(mac))
	if err != nil {
		t.Fatal(err)
	}
	reply, err := dhcpd.BuildReply(c, h, req)
	if err != nil {
		t.Fatalf("BuildReply : %v", err)
	}
	if got := leaseFromReply(mac, reply).MTU; got != 1450 {
		t.Errorf("lease.MTU = %d, attendu 1450", got)
	}
}
