package dhcpd

import (
	"bytes"
	"errors"
	"net"
	"testing"

	"git.g3e.fr/syonad/two/internal/metadata"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func cidr(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("ParseCIDR(%q): %v", s, err)
	}
	return n
}

func testConfig(t *testing.T) SubnetConfig {
	t.Helper()
	return SubnetConfig{
		Network:     cidr(t, "10.0.5.0/24"),
		InterfaceIP: net.ParseIP("10.0.5.1"),
	}
}

func testHost(t *testing.T) Host {
	t.Helper()
	mac, err := net.ParseMAC("00:22:33:00:00:0a")
	if err != nil {
		t.Fatalf("ParseMAC: %v", err)
	}
	return Host{MAC: mac, IP: net.ParseIP("10.0.5.10"), VM: "vm-test", DefaultRoute: true}
}

func request(t *testing.T, kind dhcpv4.MessageType, mac net.HardwareAddr) *dhcpv4.DHCPv4 {
	t.Helper()
	req, err := dhcpv4.New(dhcpv4.WithMessageType(kind), dhcpv4.WithHwAddr(mac))
	if err != nil {
		t.Fatalf("New request: %v", err)
	}
	return req
}

func encodedRoute(t *testing.T, routes dhcpv4.Routes, dest string) []byte {
	t.Helper()
	for _, r := range routes {
		if r.Dest.String() == dest {
			return dhcpv4.Routes{r}.ToBytes()
		}
	}
	t.Fatalf("no route to %s in %s", dest, routes)
	return nil
}

func TestRoutes_AlwaysCarriesTheMetadataRoute(t *testing.T) {
	c := testConfig(t)
	routes, err := Routes(c, Host{IP: net.ParseIP("10.0.5.10")})
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected the metadata route alone, got %s", routes)
	}
	if got := routes[0].Dest.String(); got != metadata.ServiceIP+"/32" {
		t.Errorf("destination = %s, want %s/32", got, metadata.ServiceIP)
	}
	if !routes[0].Router.Equal(c.InterfaceIP) {
		t.Errorf("next-hop = %s, want the subnet interface ip %s", routes[0].Router, c.InterfaceIP)
	}
}

func TestRoutes_WithoutInterfaceIPIsRejected(t *testing.T) {
	_, err := Routes(SubnetConfig{Network: cidr(t, "10.0.5.0/24")}, testHost(t))
	if !errors.Is(err, ErrNoInterfaceIP) {
		t.Fatalf("error = %v, want ErrNoInterfaceIP", err)
	}
}

func TestRoutes_VPCRouteUsesTheInterfaceIPAsNextHop(t *testing.T) {
	c := testConfig(t)
	c.VPCRoute = cidr(t, "10.0.0.0/16")
	c.DefaultGateway = net.ParseIP("192.0.2.1")

	routes, err := Routes(c, testHost(t))
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	for _, r := range routes {
		if r.Dest.String() != "10.0.0.0/16" {
			continue
		}
		if !r.Router.Equal(c.InterfaceIP) {
			t.Fatalf("vpc route next-hop = %s, want %s", r.Router, c.InterfaceIP)
		}
		return
	}
	t.Fatalf("no vpc route in %s", routes)
}

func TestRoutes_NoDefaultRouteWithoutDefaultGateway(t *testing.T) {
	routes, err := Routes(testConfig(t), testHost(t))
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	for _, r := range routes {
		if ones, _ := r.Dest.Mask.Size(); ones == 0 {
			t.Fatalf("unexpected default route in %s", routes)
		}
	}
}

func TestRoutes_NoDefaultRouteWhenTheInterfaceDoesNotReceiveIt(t *testing.T) {
	c := testConfig(t)
	c.DefaultGateway = net.ParseIP("10.0.5.254")

	h := testHost(t)
	h.DefaultRoute = false

	routes, err := Routes(c, h)
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	for _, r := range routes {
		if ones, _ := r.Dest.Mask.Size(); ones == 0 {
			t.Fatalf("a secondary interface must not receive the default route, got %s", routes)
		}
	}
}

func TestRoutes_DefaultRouteEncodesZeroDestinationOctets(t *testing.T) {
	c := testConfig(t)
	c.DefaultGateway = net.ParseIP("10.0.5.254")

	routes, err := Routes(c, testHost(t))
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}

	want := []byte{0x00, 10, 0, 5, 254}
	if got := encodedRoute(t, routes, "0.0.0.0/0"); !bytes.Equal(got, want) {
		t.Errorf("default route encoding = % x, want % x", got, want)
	}
}

func TestRoutes_UnalignedPrefixEncodesOnlyItsSignificantOctets(t *testing.T) {
	c := testConfig(t)
	c.VPCRoute = cidr(t, "10.16.0.0/12")

	routes, err := Routes(c, testHost(t))
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}

	want := []byte{0x0c, 10, 16, 10, 0, 5, 1}
	if got := encodedRoute(t, routes, "10.16.0.0/12"); !bytes.Equal(got, want) {
		t.Errorf("/12 encoding = % x, want % x", got, want)
	}
}

func TestRoutes_MetadataRouteEncodesOnFourDestinationOctets(t *testing.T) {
	routes, err := Routes(testConfig(t), testHost(t))
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}

	want := []byte{0x20, 169, 254, 169, 254, 10, 0, 5, 1}
	if got := encodedRoute(t, routes, metadata.ServiceIP+"/32"); !bytes.Equal(got, want) {
		t.Errorf("metadata route encoding = % x, want % x", got, want)
	}
}

func TestBuildReply_DiscoverIsAnsweredWithAnOffer(t *testing.T) {
	h := testHost(t)
	reply, err := BuildReply(testConfig(t), h, request(t, dhcpv4.MessageTypeDiscover, h.MAC))
	if err != nil {
		t.Fatalf("BuildReply: %v", err)
	}
	if reply.MessageType() != dhcpv4.MessageTypeOffer {
		t.Errorf("message type = %s, want OFFER", reply.MessageType())
	}
}

func TestBuildReply_RequestIsAnsweredWithAnAck(t *testing.T) {
	h := testHost(t)
	reply, err := BuildReply(testConfig(t), h, request(t, dhcpv4.MessageTypeRequest, h.MAC))
	if err != nil {
		t.Fatalf("BuildReply: %v", err)
	}
	if reply.MessageType() != dhcpv4.MessageTypeAck {
		t.Errorf("message type = %s, want ACK", reply.MessageType())
	}
}

func TestBuildReply_ReleaseGetsNoReply(t *testing.T) {
	h := testHost(t)
	if _, err := BuildReply(testConfig(t), h, request(t, dhcpv4.MessageTypeRelease, h.MAC)); err == nil {
		t.Fatal("a RELEASE must not produce a reply")
	}
}

func TestBuildReply_DeclineGetsNoReply(t *testing.T) {
	h := testHost(t)
	if _, err := BuildReply(testConfig(t), h, request(t, dhcpv4.MessageTypeDecline, h.MAC)); err == nil {
		t.Fatal("a DECLINE must not produce a reply")
	}
}

func TestBuildReply_CarriesAddressMaskLeaseAndServerIdentifier(t *testing.T) {
	c := testConfig(t)
	h := testHost(t)

	reply, err := BuildReply(c, h, request(t, dhcpv4.MessageTypeRequest, h.MAC))
	if err != nil {
		t.Fatalf("BuildReply: %v", err)
	}

	if !reply.YourIPAddr.Equal(h.IP) {
		t.Errorf("yiaddr = %s, want %s", reply.YourIPAddr, h.IP)
	}
	if got := net.IP(reply.SubnetMask()).String(); got != net.IP(c.Network.Mask).String() {
		t.Errorf("netmask = %s, want %s", got, net.IP(c.Network.Mask))
	}
	if got := reply.IPAddressLeaseTime(0); got != LeaseTime {
		t.Errorf("lease time = %s, want %s", got, LeaseTime)
	}
	if got := reply.ServerIdentifier(); !got.Equal(c.InterfaceIP) {
		t.Errorf("server identifier = %s, want %s", got, c.InterfaceIP)
	}
	if got := reply.DNS(); len(got) != 2 || !got[0].Equal(net.IPv4(1, 1, 1, 1)) || !got[1].Equal(net.IPv4(8, 8, 8, 8)) {
		t.Errorf("dns = %v, want 1.1.1.1 and 8.8.8.8", got)
	}
}

func TestBuildReply_NoRouterOptionWithoutDefaultRoute(t *testing.T) {
	h := testHost(t)
	reply, err := BuildReply(testConfig(t), h, request(t, dhcpv4.MessageTypeRequest, h.MAC))
	if err != nil {
		t.Fatalf("BuildReply: %v", err)
	}
	if got := reply.Router(); len(got) != 0 {
		t.Errorf("router option = %v, want none: the guest would use the server as its gateway", got)
	}
}

func TestBuildReply_RouterOptionCarriesTheDefaultGateway(t *testing.T) {
	c := testConfig(t)
	c.DefaultGateway = net.ParseIP("10.0.5.254")

	h := testHost(t)
	reply, err := BuildReply(c, h, request(t, dhcpv4.MessageTypeRequest, h.MAC))
	if err != nil {
		t.Fatalf("BuildReply: %v", err)
	}

	got := reply.Router()
	if len(got) != 1 || !got[0].Equal(c.DefaultGateway) {
		t.Errorf("router option = %v, want [%s]", got, c.DefaultGateway)
	}
}

func TestBuildReply_SecondaryInterfaceGetsNoRouterOption(t *testing.T) {
	c := testConfig(t)
	c.DefaultGateway = net.ParseIP("10.0.5.254")

	h := testHost(t)
	h.DefaultRoute = false

	reply, err := BuildReply(c, h, request(t, dhcpv4.MessageTypeRequest, h.MAC))
	if err != nil {
		t.Fatalf("BuildReply: %v", err)
	}
	if got := reply.Router(); len(got) != 0 {
		t.Errorf("router option = %v, want none on a secondary interface", got)
	}
}

func TestBuildReply_ClasslessStaticRouteIsPresent(t *testing.T) {
	h := testHost(t)
	reply, err := BuildReply(testConfig(t), h, request(t, dhcpv4.MessageTypeRequest, h.MAC))
	if err != nil {
		t.Fatalf("BuildReply: %v", err)
	}
	if got := reply.ClasslessStaticRoute(); len(got) == 0 {
		t.Fatal("option 121 missing: cloud-init would have no route to the metadata server")
	}
}

func TestBuildReply_WithoutInterfaceIPIsRejected(t *testing.T) {
	c := testConfig(t)
	c.InterfaceIP = nil

	h := testHost(t)
	if _, err := BuildReply(c, h, request(t, dhcpv4.MessageTypeRequest, h.MAC)); !errors.Is(err, ErrNoInterfaceIP) {
		t.Fatalf("error = %v, want ErrNoInterfaceIP", err)
	}
}

func TestBuildReply_WithoutNetworkIsRejected(t *testing.T) {
	c := testConfig(t)
	c.Network = nil

	h := testHost(t)
	if _, err := BuildReply(c, h, request(t, dhcpv4.MessageTypeRequest, h.MAC)); !errors.Is(err, ErrNoNetwork) {
		t.Fatalf("error = %v, want ErrNoNetwork", err)
	}
}

func TestBuildReply_WithoutHostIPIsRejected(t *testing.T) {
	h := testHost(t)
	h.IP = nil

	if _, err := BuildReply(testConfig(t), h, request(t, dhcpv4.MessageTypeRequest, testHost(t).MAC)); !errors.Is(err, ErrNoHostIP) {
		t.Fatalf("error = %v, want ErrNoHostIP", err)
	}
}

func TestBuildReply_NilRequestIsRejected(t *testing.T) {
	if _, err := BuildReply(testConfig(t), testHost(t), nil); !errors.Is(err, ErrNoRequest) {
		t.Fatalf("error = %v, want ErrNoRequest", err)
	}
}
