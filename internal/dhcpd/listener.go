package dhcpd

import (
	"log/slog"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

const MaxDatagramBytes = 1500

var clientBroadcast = net.IPv4bcast

func replyTo(peer net.Addr) net.Addr {
	udp, ok := peer.(*net.UDPAddr)
	if !ok {
		return peer
	}
	if udp.IP == nil || udp.IP.IsUnspecified() {
		return &net.UDPAddr{IP: clientBroadcast, Port: udp.Port}
	}
	return udp
}

func (s *Store) serveDatagram(conn net.PacketConn, raw []byte, peer net.Addr, logger *slog.Logger) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("dhcp datagram handling panicked", "peer", peer, "panic", r)
		}
	}()

	req, err := dhcpv4.FromBytes(raw)
	if err != nil {
		logger.Debug("malformed dhcp datagram", "peer", peer, "error", err)
		return
	}

	reply, err := s.Handle(req)
	if err != nil {
		logger.Error("building dhcp reply failed",
			"peer", peer, "mac", req.ClientHWAddr, "type", req.MessageType(), "error", err)
		return
	}
	if reply == nil {
		logger.Debug("no reply for datagram", "mac", req.ClientHWAddr, "type", req.MessageType())
		return
	}

	target := replyTo(peer)
	if _, err := conn.WriteTo(reply.ToBytes(), target); err != nil {
		logger.Error("sending dhcp reply failed", "target", target, "mac", req.ClientHWAddr, "error", err)
		return
	}
	logger.Info("dhcp reply sent",
		"mac", req.ClientHWAddr, "type", reply.MessageType(), "ip", reply.YourIPAddr, "target", target)
}

func (s *Store) Serve(conn net.PacketConn, logger *slog.Logger) error {
	buf := make([]byte, MaxDatagramBytes)

	for {
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return err
		}
		s.serveDatagram(conn, buf[:n], peer, logger)
	}
}
