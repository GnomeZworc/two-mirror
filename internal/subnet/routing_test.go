package subnet

import (
	"errors"
	"net"
	"testing"
)

func cidr(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("ParseCIDR(%q) : %v", s, err)
	}
	return n
}

func baseSubnet(t *testing.T, mode string) subnetData {
	t.Helper()
	return subnetData{
		mode:        mode,
		interfaceIP: net.ParseIP("10.1.1.1").To4(),
		cidr:        cidr(t, "10.1.0.0/23"),
		vpcCIDR:     cidr(t, "192.168.0.0/16"),
	}
}

func deduced(ip string) func() (net.IP, error) {
	return func() (net.IP, error) { return net.ParseIP(ip).To4(), nil }
}

func neverDeduced(t *testing.T) func() (net.IP, error) {
	t.Helper()
	return func() (net.IP, error) {
		t.Error("la gateway de l'host ne doit pas être interrogée dans ce cas")
		return nil, nil
	}
}

// --- next-hop de la route par défaut ---

func TestDhcpRouting_DefaultRouteUsesInterfaceIP(t *testing.T) {
	d := baseSubnet(t, ModeVxlan)

	gw, _, err := dhcpRouting(d, neverDeduced(t))
	if err != nil {
		t.Fatalf("dhcpRouting : %v", err)
	}
	if gw.String() != "10.1.1.1" {
		t.Errorf("sans default_route le next-hop doit être l'interface_ip, obtenu %s", gw)
	}
}

func TestDhcpRouting_DefaultRouteUsesSuppliedGateway(t *testing.T) {
	d := baseSubnet(t, ModeVxlan)
	d.defaultRoute = true
	d.gateway = net.ParseIP("10.1.1.254").To4()

	gw, _, err := dhcpRouting(d, neverDeduced(t))
	if err != nil {
		t.Fatalf("dhcpRouting : %v", err)
	}
	if gw.String() != "10.1.1.254" {
		t.Errorf("gateway fournie attendue, obtenu %s", gw)
	}
}

func TestDhcpRouting_DefaultRouteFallsBackToDeducedGateway(t *testing.T) {
	d := baseSubnet(t, ModeBridge)
	d.defaultRoute = true

	gw, _, err := dhcpRouting(d, deduced("192.0.2.1"))
	if err != nil {
		t.Fatalf("dhcpRouting : %v", err)
	}
	if gw.String() != "192.0.2.1" {
		t.Errorf("gateway déduite attendue, obtenu %s", gw)
	}
}

func TestDhcpRouting_SuppliedGatewayIgnoredWithoutDefaultRoute(t *testing.T) {
	d := baseSubnet(t, ModeVxlan)
	d.gateway = net.ParseIP("10.1.1.254").To4()

	gw, _, err := dhcpRouting(d, neverDeduced(t))
	if err != nil {
		t.Fatalf("dhcpRouting : %v", err)
	}
	if gw.String() != "10.1.1.1" {
		t.Errorf("gateway fournie sans default_route : ignorée en silence, next-hop attendu 10.1.1.1, obtenu %s", gw)
	}
}

func TestDhcpRouting_DeductionFailureIsReported(t *testing.T) {
	d := baseSubnet(t, ModeBridge)
	d.defaultRoute = true

	_, _, err := dhcpRouting(d, func() (net.IP, error) { return nil, errors.New("pas de route") })
	if err == nil {
		t.Error("l'échec de déduction de la gateway doit remonter, pas produire une route muette")
	}
}

// --- route VPC ---

func TestDhcpRouting_VPCRouteKeptInVxlan(t *testing.T) {
	_, route, err := dhcpRouting(baseSubnet(t, ModeVxlan), neverDeduced(t))
	if err != nil {
		t.Fatalf("dhcpRouting : %v", err)
	}
	if route == nil || route.String() != "192.168.0.0/16" {
		t.Errorf("route VPC attendue, obtenu %v", route)
	}
}

func TestDhcpRouting_VPCRouteKeptInPublicIP(t *testing.T) {
	d := baseSubnet(t, ModePublicIP)
	d.defaultRoute = true
	d.gateway = net.ParseIP("203.0.113.1").To4()

	gw, route, err := dhcpRouting(d, neverDeduced(t))
	if err != nil {
		t.Fatalf("dhcpRouting : %v", err)
	}
	if route == nil || route.String() != "192.168.0.0/16" {
		t.Fatalf("route VPC attendue sur un subnet public, obtenu %v", route)
	}
	if gw.String() != "203.0.113.1" {
		t.Errorf("next-hop par défaut attendu 203.0.113.1, obtenu %s", gw)
	}
	// C'est tout l'intérêt du mode : la route VPC garde interface_ip comme next-hop,
	// donc le trafic interne ne sort jamais par la gateway publique.
}

func TestDhcpRouting_NoVPCRouteInBridge(t *testing.T) {
	_, route, err := dhcpRouting(baseSubnet(t, ModeBridge), neverDeduced(t))
	if err != nil {
		t.Fatalf("dhcpRouting : %v", err)
	}
	if route != nil {
		t.Errorf("le mode bridge n'a pas de route VPC, obtenu %v", route)
	}
}

// --- modes ---

func TestValidMode(t *testing.T) {
	for _, m := range []string{ModeVxlan, ModeBridge, ModePublicIP} {
		if !ValidMode(m) {
			t.Errorf("%q devrait être un mode valide", m)
		}
	}
	for _, m := range []string{"", "public", "vxlan ", "VXLAN"} {
		if ValidMode(m) {
			t.Errorf("%q ne devrait pas être un mode valide", m)
		}
	}
}
