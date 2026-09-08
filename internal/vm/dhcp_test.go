package vm

import (
	"net"
	"testing"
)

func nic(idx int, subnet, vpc, bridge, ip, mac string, primary bool) nicData {
	return nicData{
		index:       idx,
		subnetName:  subnet,
		vpcName:     vpc,
		bridge:      bridge,
		interfaceIP: "10.0.5.1",
		ip:          ip,
		mac:         mac,
		primary:     primary,
	}
}

func group(t *testing.T, groups map[string]*subnetReservations, key string) *subnetReservations {
	t.Helper()
	g, ok := groups[key]
	if !ok || g == nil {
		t.Fatalf("no group %q, got %v", key, keysOf(groups))
	}
	return g
}

func keysOf(groups map[string]*subnetReservations) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	return keys
}

func TestDHCPReservations_GroupsInterfacesBySubnet(t *testing.T) {
	d := vmData{nics: []nicData{
		nic(0, "sn-000001", "vp-admin", "br-000001", "10.0.5.10", "00:22:33:00:00:0a", true),
		nic(1, "sn-000001", "vp-admin", "br-000001", "10.0.5.11", "00:22:33:00:00:0b", false),
		nic(2, "sn-000002", "vp-admin", "br-000002", "10.0.6.10", "00:22:33:00:00:0c", false),
	}}

	got := dhcpReservations(d)
	if len(got) != 2 {
		t.Fatalf("groups = %d, want one per subnet", len(got))
	}
	if n := len(group(t, got, "vp-admin_br-000001").reservations); n != 2 {
		t.Errorf("br-000001 carries %d reservations, want 2", n)
	}
	if n := len(group(t, got, "vp-admin_br-000002").reservations); n != 1 {
		t.Errorf("br-000002 carries %d reservations, want 1", n)
	}
}

func TestDHCPReservations_OnlyThePrimaryCarriesTheDefaultRoute(t *testing.T) {
	d := vmData{nics: []nicData{
		nic(0, "sn-000001", "vp-admin", "br-000001", "10.0.5.10", "00:22:33:00:00:0a", true),
		nic(1, "sn-000001", "vp-admin", "br-000001", "10.0.5.11", "00:22:33:00:00:0b", false),
	}}

	res := group(t, dhcpReservations(d), "vp-admin_br-000001").reservations
	byMAC := map[string]bool{}
	for _, r := range res {
		byMAC[r.MAC] = r.DefaultRoute
	}
	if !byMAC["00:22:33:00:00:0a"] {
		t.Error("the primary interface must carry the default route")
	}
	if byMAC["00:22:33:00:00:0b"] {
		t.Error("a secondary interface must not carry the default route")
	}
}

func TestDHCPReservations_KeepsTheInterfaceIndex(t *testing.T) {
	d := vmData{nics: []nicData{
		nic(3, "sn-000001", "vp-admin", "br-000001", "10.0.5.10", "00:22:33:00:00:0a", true),
	}}

	res := group(t, dhcpReservations(d), "vp-admin_br-000001").reservations
	if res[0].Index != 3 {
		t.Errorf("index = %d, want 3: the dnsmasq tag is derived from it", res[0].Index)
	}
}

func TestDHCPReservations_CarriesTheSubnetIdentity(t *testing.T) {
	_, vpcCIDR, err := net.ParseCIDR("10.0.0.0/16")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	n := nic(0, "sn-000001", "vp-admin", "br-000001", "10.0.5.10", "00:22:33:00:00:0a", true)
	n.vpcCIDR = vpcCIDR

	g := group(t, dhcpReservations(vmData{nics: []nicData{n}}), "vp-admin_br-000001")
	if g.subnet.Name != "sn-000001" {
		t.Errorf("subnet name = %s, want sn-000001", g.subnet.Name)
	}
	if !g.subnet.InterfaceIP.Equal(net.ParseIP("10.0.5.1")) {
		t.Errorf("interface ip = %s, want 10.0.5.1", g.subnet.InterfaceIP)
	}
	if g.subnet.VPCRoute == nil || g.subnet.VPCRoute.String() != "10.0.0.0/16" {
		t.Errorf("vpc route = %v, want 10.0.0.0/16", g.subnet.VPCRoute)
	}
}

func TestDHCPReservations_SameBridgeInTwoVPCsStaysSeparate(t *testing.T) {
	d := vmData{nics: []nicData{
		nic(0, "sn-000001", "vp-admin", "br-000001", "10.0.5.10", "00:22:33:00:00:0a", true),
		nic(1, "sn-000009", "vp-other", "br-000001", "10.9.5.10", "00:22:33:00:00:0d", false),
	}}

	if got := len(dhcpReservations(d)); got != 2 {
		t.Errorf("groups = %d, want 2: the vpc is part of the instance identity", got)
	}
}

func TestDHCPReservations_NoNICYieldsNoGroup(t *testing.T) {
	if got := len(dhcpReservations(vmData{})); got != 0 {
		t.Errorf("groups = %d, want none", got)
	}
}
