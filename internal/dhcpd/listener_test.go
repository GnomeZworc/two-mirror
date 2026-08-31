package dhcpd

import (
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func loopbackPair(t *testing.T) (*net.UDPConn, *net.UDPConn) {
	t.Helper()

	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("ListenUDP server: %v", err)
	}
	t.Cleanup(func() { server.Close() })

	client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("ListenUDP client: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	return server, client
}

func exchange(t *testing.T, s *Store, raw []byte) *dhcpv4.DHCPv4 {
	t.Helper()

	server, client := loopbackPair(t)
	go s.Serve(server, discard())

	if _, err := client.WriteToUDP(raw, server.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatalf("WriteToUDP: %v", err)
	}

	if err := client.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, MaxDatagramBytes)
	n, _, err := client.ReadFromUDP(buf)
	if err != nil {
		return nil
	}

	reply, err := dhcpv4.FromBytes(buf[:n])
	if err != nil {
		t.Fatalf("the reply must be a valid dhcp packet: %v", err)
	}
	return reply
}

func TestServe_AnswersAKnownMAC(t *testing.T) {
	s := configuredStore(t)

	reply := exchange(t, s, request(t, dhcpv4.MessageTypeDiscover, mac(t, "00:22:33:00:00:0a")).ToBytes())
	if reply == nil {
		t.Fatal("a known mac must be answered on the wire")
	}
	if reply.MessageType() != dhcpv4.MessageTypeOffer {
		t.Errorf("message type = %s, want OFFER", reply.MessageType())
	}
	if !reply.YourIPAddr.Equal(net.ParseIP("10.0.5.10")) {
		t.Errorf("yiaddr = %s, want 10.0.5.10", reply.YourIPAddr)
	}
}

func TestServe_StaysSilentForAnUnknownMAC(t *testing.T) {
	s := configuredStore(t)

	if reply := exchange(t, s, request(t, dhcpv4.MessageTypeDiscover, mac(t, "00:22:33:ff:ff:ff")).ToBytes()); reply != nil {
		t.Errorf("an unknown mac must get nothing on the wire, got %s", reply.MessageType())
	}
}

func TestServe_StaysSilentOnARelease(t *testing.T) {
	s := configuredStore(t)

	if reply := exchange(t, s, request(t, dhcpv4.MessageTypeRelease, mac(t, "00:22:33:00:00:0a")).ToBytes()); reply != nil {
		t.Errorf("a RELEASE must get nothing on the wire, got %s", reply.MessageType())
	}
}

func TestServe_SurvivesAMalformedDatagram(t *testing.T) {
	s := configuredStore(t)
	server, client := loopbackPair(t)
	go s.Serve(server, discard())

	target := server.LocalAddr().(*net.UDPAddr)
	for _, garbage := range [][]byte{{}, {0x01}, make([]byte, 1200)} {
		if _, err := client.WriteToUDP(garbage, target); err != nil {
			t.Fatalf("WriteToUDP: %v", err)
		}
	}

	if _, err := client.WriteToUDP(request(t, dhcpv4.MessageTypeDiscover, mac(t, "00:22:33:00:00:0a")).ToBytes(), target); err != nil {
		t.Fatalf("WriteToUDP: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, MaxDatagramBytes)
	if _, _, err := client.ReadFromUDP(buf); err != nil {
		t.Fatalf("the loop must survive garbage and keep serving: %v", err)
	}
}

func TestServe_ReturnsWhenTheConnectionCloses(t *testing.T) {
	s := configuredStore(t)
	server, _ := loopbackPair(t)

	done := make(chan error, 1)
	go func() { done <- s.Serve(server, discard()) }()

	server.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Serve must report why it stopped")
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after the connection closed")
	}
}

type explodingConn struct {
	net.PacketConn
	mu     sync.Mutex
	writes int
}

func (c *explodingConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.mu.Lock()
	first := c.writes == 0
	c.writes++
	c.mu.Unlock()

	if first {
		panic("write exploded")
	}
	return c.PacketConn.WriteTo(b, addr)
}

func TestServe_SurvivesAPanicWhileHandlingADatagram(t *testing.T) {
	s := configuredStore(t)
	server, client := loopbackPair(t)
	go s.Serve(&explodingConn{PacketConn: server}, discard())

	target := server.LocalAddr().(*net.UDPAddr)
	raw := request(t, dhcpv4.MessageTypeDiscover, mac(t, "00:22:33:00:00:0a")).ToBytes()

	for range 2 {
		if _, err := client.WriteToUDP(raw, target); err != nil {
			t.Fatalf("WriteToUDP: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, MaxDatagramBytes)
	if _, _, err := client.ReadFromUDP(buf); err != nil {
		t.Fatalf("a panic on one datagram must not kill the serving loop: %v", err)
	}
}

func TestReplyTo_BroadcastsWhenTheClientHasNoAddress(t *testing.T) {
	got := replyTo(&net.UDPAddr{IP: net.IPv4zero, Port: 68})

	udp, ok := got.(*net.UDPAddr)
	if !ok {
		t.Fatalf("target = %T, want *net.UDPAddr", got)
	}
	if !udp.IP.Equal(net.IPv4bcast) {
		t.Errorf("target = %s, want 255.255.255.255: the client cannot be reached by unicast yet", udp.IP)
	}
	if udp.Port != 68 {
		t.Errorf("port = %d, want the client port to be kept", udp.Port)
	}
}

func TestReplyTo_KeepsTheUnicastPeerWhenItHasAnAddress(t *testing.T) {
	got := replyTo(&net.UDPAddr{IP: net.ParseIP("10.0.5.10"), Port: 68})

	udp := got.(*net.UDPAddr)
	if !udp.IP.Equal(net.ParseIP("10.0.5.10")) {
		t.Errorf("target = %s, want the renewing client itself", udp.IP)
	}
}

func TestReplyTo_BroadcastsWhenThePeerIPIsNil(t *testing.T) {
	udp := replyTo(&net.UDPAddr{Port: 68}).(*net.UDPAddr)
	if !udp.IP.Equal(net.IPv4bcast) {
		t.Errorf("target = %s, want 255.255.255.255", udp.IP)
	}
}
