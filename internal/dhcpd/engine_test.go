package dhcpd

import (
	"errors"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func configuredStore(t *testing.T) *Store {
	t.Helper()
	s, _ := loadedStore(t)
	if err := s.SetSubnet(fullConfig(t)); err != nil {
		t.Fatalf("SetSubnet: %v", err)
	}
	if err := s.SetHost(testHost(t)); err != nil {
		t.Fatalf("SetHost: %v", err)
	}
	return s
}

func TestHandle_KnownMACGetsAnOfferOnDiscover(t *testing.T) {
	s := configuredStore(t)

	reply, err := s.Handle(request(t, dhcpv4.MessageTypeDiscover, mac(t, "00:22:33:00:00:0a")))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reply == nil {
		t.Fatal("a known mac must be answered")
	}
	if reply.MessageType() != dhcpv4.MessageTypeOffer {
		t.Errorf("message type = %s, want OFFER", reply.MessageType())
	}
	if !reply.YourIPAddr.Equal(net.ParseIP("10.0.5.10")) {
		t.Errorf("yiaddr = %s, want the reserved 10.0.5.10", reply.YourIPAddr)
	}
}

func TestHandle_UnknownMACIsAnsweredWithSilence(t *testing.T) {
	s := configuredStore(t)

	reply, err := s.Handle(request(t, dhcpv4.MessageTypeDiscover, mac(t, "00:22:33:ff:ff:ff")))
	if err != nil {
		t.Fatalf("an unknown mac is not an error: %v", err)
	}
	if reply != nil {
		t.Error("an unknown mac must get no reply, not a NAK")
	}
}

func TestHandle_UnconfiguredSubnetIsAnsweredWithSilence(t *testing.T) {
	s, _ := loadedStore(t)
	if err := s.SetHost(testHost(t)); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	reply, err := s.Handle(request(t, dhcpv4.MessageTypeDiscover, mac(t, "00:22:33:00:00:0a")))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reply != nil {
		t.Error("without a subnet configuration the server must stay silent")
	}
}

func TestHandle_ReleaseIsANoOp(t *testing.T) {
	s := configuredStore(t)

	reply, err := s.Handle(request(t, dhcpv4.MessageTypeRelease, mac(t, "00:22:33:00:00:0a")))
	if err != nil {
		t.Fatalf("a RELEASE is not an error: %v", err)
	}
	if reply != nil {
		t.Error("a RELEASE must get no reply: reservations are static")
	}
}

func TestHandle_DeclineIsANoOp(t *testing.T) {
	s := configuredStore(t)

	reply, err := s.Handle(request(t, dhcpv4.MessageTypeDecline, mac(t, "00:22:33:00:00:0a")))
	if err != nil {
		t.Fatalf("a DECLINE is not an error: %v", err)
	}
	if reply != nil {
		t.Error("a DECLINE must get no reply: there is nothing to release")
	}
}

func TestHandle_RequestIsAnsweredWithAnAck(t *testing.T) {
	s := configuredStore(t)

	reply, err := s.Handle(request(t, dhcpv4.MessageTypeRequest, mac(t, "00:22:33:00:00:0a")))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reply == nil || reply.MessageType() != dhcpv4.MessageTypeAck {
		t.Fatalf("reply = %v, want an ACK", reply)
	}
}

func TestHandle_NilRequestIsRejected(t *testing.T) {
	s := configuredStore(t)

	if _, err := s.Handle(nil); !errors.Is(err, ErrNoRequest) {
		t.Fatalf("error = %v, want ErrNoRequest", err)
	}
}

func TestHandle_DeletedHostStopsBeingAnswered(t *testing.T) {
	s := configuredStore(t)
	if err := s.DelHost(mac(t, "00:22:33:00:00:0a")); err != nil {
		t.Fatalf("DelHost: %v", err)
	}

	reply, err := s.Handle(request(t, dhcpv4.MessageTypeDiscover, mac(t, "00:22:33:00:00:0a")))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reply != nil {
		t.Error("a deleted host must no longer be served")
	}
}

func TestProbe_ReturnsWhatWouldBeSentToTheMAC(t *testing.T) {
	s := configuredStore(t)

	reply, err := s.Probe(mac(t, "00:22:33:00:00:0A"))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if reply == nil {
		t.Fatal("a known mac must be described")
	}
	if !reply.YourIPAddr.Equal(net.ParseIP("10.0.5.10")) {
		t.Errorf("yiaddr = %s, want 10.0.5.10", reply.YourIPAddr)
	}
	if got := reply.ClasslessStaticRoute(); len(got) != 3 {
		t.Errorf("routes = %s, want metadata, vpc and default", got)
	}
}

func TestProbe_UnknownMACReturnsNothing(t *testing.T) {
	s := configuredStore(t)

	reply, err := s.Probe(mac(t, "00:22:33:ff:ff:ff"))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if reply != nil {
		t.Error("an unknown mac must describe no reply")
	}
}

func TestProbe_WithoutSubnetConfigurationIsRejected(t *testing.T) {
	s, _ := loadedStore(t)

	if _, err := s.Probe(mac(t, "00:22:33:00:00:0a")); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("error = %v, want ErrNotConfigured", err)
	}
}

func TestProbe_EmptyMACIsRejected(t *testing.T) {
	s := configuredStore(t)

	if _, err := s.Probe(nil); !errors.Is(err, ErrNoMAC) {
		t.Fatalf("error = %v, want ErrNoMAC", err)
	}
}

func TestHandle_SecondaryInterfaceGetsNoDefaultRoute(t *testing.T) {
	s := configuredStore(t)
	h := testHost(t)
	h.MAC = mac(t, "00:22:33:00:00:0b")
	h.IP = net.ParseIP("10.0.5.11")
	h.DefaultRoute = false
	if err := s.SetHost(h); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	reply, err := s.Handle(request(t, dhcpv4.MessageTypeRequest, mac(t, "00:22:33:00:00:0b")))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := reply.Router(); len(got) != 0 {
		t.Errorf("router option = %v, want none on a secondary interface", got)
	}
	for _, r := range reply.ClasslessStaticRoute() {
		if ones, _ := r.Dest.Mask.Size(); ones == 0 {
			t.Errorf("unexpected default route for a secondary interface: %s", reply.ClasslessStaticRoute())
		}
	}
}
