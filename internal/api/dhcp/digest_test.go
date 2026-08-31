package dhcpapi

import (
	"testing"
)

func subnet() Subnet {
	return Subnet{
		Network:        "10.0.5.0/24",
		InterfaceIP:    "10.0.5.1",
		VPCRoute:       "10.0.0.0/16",
		DefaultGateway: "10.0.5.254",
	}
}

func hosts() []Host {
	return []Host{
		{MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", VM: "vm-a", DefaultRoute: true},
		{MAC: "00:22:33:00:00:0b", IP: "10.0.5.11", VM: "vm-b"},
	}
}

func digestOf(t *testing.T, s State) string {
	t.Helper()
	d, err := Digest(s)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	return d
}

func TestDigest_IsStableAcrossHostOrder(t *testing.T) {
	sub := subnet()
	a := State{Subnet: &sub, Hosts: hosts()}

	reversed := hosts()
	reversed[0], reversed[1] = reversed[1], reversed[0]
	b := State{Subnet: &sub, Hosts: reversed}

	if digestOf(t, a) != digestOf(t, b) {
		t.Error("host order must not change the digest: the watchdog would report a phantom drift")
	}
}

func TestDigest_IsStableAcrossMACCase(t *testing.T) {
	sub := subnet()
	a := State{Subnet: &sub, Hosts: hosts()}

	upper := hosts()
	upper[0].MAC = "00:22:33:00:00:0A"
	b := State{Subnet: &sub, Hosts: upper}

	if digestOf(t, a) != digestOf(t, b) {
		t.Error("mac case must not change the digest")
	}
}

func TestDigest_IsStableAcrossIPv4InIPv6Notation(t *testing.T) {
	sub := subnet()
	a := State{Subnet: &sub, Hosts: hosts()}

	mapped := subnet()
	mapped.InterfaceIP = "::ffff:10.0.5.1"
	b := State{Subnet: &mapped, Hosts: hosts()}

	if digestOf(t, a) != digestOf(t, b) {
		t.Error("the same address written in ipv4-mapped form must hash alike")
	}
}

func TestDigest_ChangesWhenAHostIPChanges(t *testing.T) {
	sub := subnet()
	a := State{Subnet: &sub, Hosts: hosts()}

	moved := hosts()
	moved[0].IP = "10.0.5.99"
	b := State{Subnet: &sub, Hosts: moved}

	if digestOf(t, a) == digestOf(t, b) {
		t.Error("a changed reservation must change the digest")
	}
}

func TestDigest_ChangesWhenTheDefaultRouteFlagChanges(t *testing.T) {
	sub := subnet()
	a := State{Subnet: &sub, Hosts: hosts()}

	flipped := hosts()
	flipped[0].DefaultRoute = false
	b := State{Subnet: &sub, Hosts: flipped}

	if digestOf(t, a) == digestOf(t, b) {
		t.Error("the default route flag is part of the served state")
	}
}

func TestDigest_ChangesWhenTheSubnetChanges(t *testing.T) {
	sub := subnet()
	a := State{Subnet: &sub, Hosts: hosts()}

	other := subnet()
	other.DefaultGateway = "10.0.5.253"
	b := State{Subnet: &other, Hosts: hosts()}

	if digestOf(t, a) == digestOf(t, b) {
		t.Error("the subnet configuration is part of the served state")
	}
}

func TestDigest_DistinguishesNoSubnetFromAConfiguredOne(t *testing.T) {
	sub := subnet()
	configured := State{Subnet: &sub}
	bare := State{}

	if digestOf(t, configured) == digestOf(t, bare) {
		t.Error("an unconfigured subnet must not hash like a configured one")
	}
}

func TestDigest_EmptyAndNilHostsHashAlike(t *testing.T) {
	if digestOf(t, State{Hosts: nil}) != digestOf(t, State{Hosts: []Host{}}) {
		t.Error("nil and empty host lists describe the same state")
	}
}

func TestDigest_RejectsAnInvalidMAC(t *testing.T) {
	if _, err := Digest(State{Hosts: []Host{{MAC: "nope", IP: "10.0.5.10"}}}); err == nil {
		t.Fatal("an invalid mac must be reported, not hashed")
	}
}

func TestDigest_RejectsAnInvalidIP(t *testing.T) {
	if _, err := Digest(State{Hosts: []Host{{MAC: "00:22:33:00:00:0a", IP: "10.0.5.300"}}}); err == nil {
		t.Fatal("an invalid ip must be reported, not hashed")
	}
}

func TestCanonical_RejectsADuplicateMAC(t *testing.T) {
	dup := []Host{
		{MAC: "00:22:33:00:00:0a", IP: "10.0.5.10"},
		{MAC: "00:22:33:00:00:0A", IP: "10.0.5.11"},
	}
	if _, err := Canonical(State{Hosts: dup}); err == nil {
		t.Fatal("the same mac twice is an inconsistent state, not something to hash")
	}
}

func TestCanonical_NormalizesTheNetworkToItsBaseAddress(t *testing.T) {
	sub := subnet()
	sub.Network = "10.0.5.42/24"

	got, err := CanonicalSubnet(sub)
	if err != nil {
		t.Fatalf("CanonicalSubnet: %v", err)
	}
	if got.Network != "10.0.5.0/24" {
		t.Errorf("network = %s, want 10.0.5.0/24", got.Network)
	}
}

func TestCanonical_SortsHostsByMAC(t *testing.T) {
	unsorted := []Host{
		{MAC: "00:22:33:00:00:0c", IP: "10.0.5.12"},
		{MAC: "00:22:33:00:00:0a", IP: "10.0.5.10"},
	}
	got, err := Canonical(State{Hosts: unsorted})
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	if got.Hosts[0].MAC != "00:22:33:00:00:0a" {
		t.Errorf("hosts = %v, want sorted by mac", got.Hosts)
	}
}
