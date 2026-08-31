package dhcpapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	"git.g3e.fr/syonad/two/internal/dhcpd"
	"git.g3e.fr/syonad/two/pkg/db/statefile"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

const SocketMode = 0o600

type Server struct {
	store    *dhcpd.Store
	listener net.Listener
	logger   *slog.Logger
}

func Listen(store *dhcpd.Store, path string, logger *slog.Logger) (*Server, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, statefile.DirMode); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale socket %s: %w", path, err)
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, SocketMode); err != nil {
		listener.Close()
		return nil, fmt.Errorf("chmod %s: %w", path, err)
	}

	return &Server{store: store, listener: listener, logger: logger}, nil
}

func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

func (s *Server) Close() error {
	return s.listener.Close()
}

func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("control connection panicked", "panic", r)
		}
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), MaxMessageBytes)
	encoder := json.NewEncoder(conn)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := encoder.Encode(failure(fmt.Errorf("malformed request: %w", err))); err != nil {
				return
			}
			continue
		}

		if err := encoder.Encode(s.dispatch(req)); err != nil {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		s.logger.Error("control connection read failed", "error", err)
	}
}

func failure(err error) Response {
	return Response{OK: false, Error: err.Error()}
}

func (s *Server) dispatch(req Request) Response {
	switch req.Verb {
	case VerbSetSubnet:
		if req.Subnet == nil {
			return failure(errors.New("set-subnet requires a subnet"))
		}
		config, err := req.Subnet.toConfig()
		if err != nil {
			return failure(err)
		}
		if err := s.store.SetSubnet(config); err != nil {
			return failure(err)
		}
		return Response{OK: true}

	case VerbSetHost:
		if req.Host == nil {
			return failure(errors.New("set-host requires a host"))
		}
		host, err := req.Host.toHost()
		if err != nil {
			return failure(err)
		}
		if err := s.store.SetHost(host); err != nil {
			return failure(err)
		}
		return Response{OK: true}

	case VerbDelHost:
		mac, err := net.ParseMAC(req.MAC)
		if err != nil {
			return failure(fmt.Errorf("invalid mac %q: %w", req.MAC, err))
		}
		if err := s.store.DelHost(mac); err != nil {
			return failure(err)
		}
		return Response{OK: true}

	case VerbGetState:
		state := stateFromStore(s.store)
		digest, err := Digest(state)
		if err != nil {
			return failure(err)
		}
		return Response{OK: true, State: &state, Digest: digest}

	case VerbProbe:
		mac, err := net.ParseMAC(req.MAC)
		if err != nil {
			return failure(fmt.Errorf("invalid mac %q: %w", req.MAC, err))
		}
		reply, err := s.store.Probe(mac)
		if err != nil {
			return failure(err)
		}
		if reply == nil {
			return Response{OK: true, Served: false}
		}
		return Response{OK: true, Served: true, Lease: leaseFromReply(mac, reply)}

	default:
		return failure(fmt.Errorf("unknown verb %q", req.Verb))
	}
}

func leaseFromReply(mac net.HardwareAddr, reply *dhcpv4.DHCPv4) *Lease {
	lease := &Lease{
		MAC:          mac.String(),
		IP:           reply.YourIPAddr.String(),
		Netmask:      net.IP(reply.SubnetMask()).String(),
		DNS:          make([]string, 0, 2),
		Routes:       make([]string, 0, 3),
		LeaseSeconds: uint32(reply.IPAddressLeaseTime(0).Seconds()),
	}

	if routers := reply.Router(); len(routers) > 0 {
		lease.Router = routers[0].String()
	}
	for _, dns := range reply.DNS() {
		lease.DNS = append(lease.DNS, dns.String())
	}
	for _, route := range reply.ClasslessStaticRoute() {
		lease.Routes = append(lease.Routes, route.Dest.String()+" via "+route.Router.String())
	}
	return lease
}
