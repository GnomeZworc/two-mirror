package hostsvc

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"git.g3e.fr/syonad/two/pkg/db/statefile"
)

const (
	ServiceIP      = "169.254.169.253"
	DefaultResolv  = "/etc/resolv.conf"
	DefaultTimeout = 2 * time.Second
	tcpIdle        = 10 * time.Second
	maxTCPConns    = 16
)

type Options struct {
	DNS       []string
	NTP       []string
	Rate      int
	Resolv    string
	DNSListen string
	NTPListen string
	Timeout   time.Duration
	Clock     Clock
	Enter     func(vpc string, fn func() error) error
	Exists    func(vpc string) bool
	Logger    *slog.Logger
}

func (o Options) withDefaults() Options {
	if o.Resolv == "" {
		o.Resolv = DefaultResolv
	}
	if o.DNSListen == "" {
		o.DNSListen = net.JoinHostPort(ServiceIP, "53")
	}
	if o.NTPListen == "" {
		o.NTPListen = net.JoinHostPort(ServiceIP, "123")
	}
	if o.Timeout == 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Clock == nil {
		o.Clock = SystemClock{}
	}
	if o.Enter == nil {
		o.Enter = enterNetns
	}
	if o.Exists == nil {
		o.Exists = netnsExists
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return o
}

type State struct {
	VPCs []string `json:"vpcs"`
}

type Service struct {
	opts  Options
	state *statefile.File[State]
	mu    sync.Mutex
	vpcs  map[string]*instance
}

func New(opts Options, statePath string) (*Service, error) {
	if opts.Rate <= 0 {
		return nil, fmt.Errorf("rate limit %d must be a positive number of requests per second", opts.Rate)
	}
	return &Service{
		opts:  opts.withDefaults(),
		state: statefile.New[State](statePath),
		vpcs:  map[string]*instance{},
	}, nil
}

func validVPC(vpc string) error {
	if vpc == "" || vpc == "." || vpc == ".." || strings.ContainsAny(vpc, "/\x00") {
		return fmt.Errorf("invalid vpc name %q", vpc)
	}
	return nil
}

func (s *Service) Restore() error {
	saved, err := s.state.Load()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for _, vpc := range saved.VPCs {
		if !s.opts.Exists(vpc) {
			s.opts.Logger.Info("netns gone, vpc dropped from the state", "vpc", vpc)
			continue
		}
		if err := s.open(vpc); err != nil {
			errs = append(errs, err)
		}
	}
	if err := s.save(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Service) Open(vpc string) error {
	if err := validVPC(vpc); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, open := s.vpcs[vpc]; open {
		return nil
	}
	if err := s.open(vpc); err != nil {
		return err
	}
	return s.save()
}

func (s *Service) Close(vpc string) error {
	if err := validVPC(vpc); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, open := s.vpcs[vpc]
	if !open {
		return nil
	}
	delete(s.vpcs, vpc)
	err := inst.close()
	return errors.Join(err, s.save())
}

func (s *Service) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.names()
}

func (s *Service) Shutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for vpc, inst := range s.vpcs {
		errs = append(errs, inst.close())
		delete(s.vpcs, vpc)
	}
	return errors.Join(errs...)
}

func (s *Service) names() []string {
	out := make([]string, 0, len(s.vpcs))
	for vpc := range s.vpcs {
		out = append(out, vpc)
	}
	sort.Strings(out)
	return out
}

func (s *Service) save() error {
	return s.state.Save(State{VPCs: s.names()})
}

func (s *Service) open(vpc string) error {
	inst := &instance{
		vpc:   vpc,
		svc:   s,
		limit: newLimiter(s.opts.Rate, time.Now),
		conns: make(chan struct{}, maxTCPConns),
		log:   s.opts.Logger.With("vpc", vpc),
	}
	dns, ntp := s.opts.DNSListen, s.opts.NTPListen
	err := s.opts.Enter(vpc, func() error {
		var err error
		if inst.udpDNS, err = net.ListenPacket("udp4", dns); err != nil {
			return fmt.Errorf("listen udp %s: %w", dns, err)
		}
		if inst.tcpDNS, err = net.Listen("tcp4", dns); err != nil {
			return fmt.Errorf("listen tcp %s: %w", dns, err)
		}
		if inst.udpNTP, err = net.ListenPacket("udp4", ntp); err != nil {
			return fmt.Errorf("listen udp %s: %w", ntp, err)
		}
		return nil
	})
	if err != nil {
		return errors.Join(fmt.Errorf("open vpc %s: %w", vpc, err), inst.close())
	}
	inst.serve()
	s.vpcs[vpc] = inst
	inst.log.Info("vpc served", "dns", dns, "ntp", ntp)
	return nil
}

func (s *Service) dnsServers() []string {
	servers := append([]string{}, s.opts.DNS...)
	host, err := resolvers(s.opts.Resolv)
	if err != nil {
		s.opts.Logger.Warn("host resolver unreadable", "error", err)
	}
	return append(servers, host...)
}

func (s *Service) ntpAnswer(request []byte, received time.Time) []byte {
	if len(s.opts.NTP) > 0 {
		answer, err := exchangeNTP(s.opts.NTP, request, s.opts.Timeout)
		if err == nil {
			return answer
		}
		s.opts.Logger.Warn("ntp upstreams failed, answering from the host clock", "error", err)
	}
	synced, maxError, err := s.opts.Clock.Sync()
	if err != nil {
		s.opts.Logger.Warn("host clock status unreadable, answering as unsynchronized", "error", err)
		synced = false
	}
	return hostReply(request, received, s.opts.Clock.Now(), synced, maxError)
}

type instance struct {
	vpc    string
	svc    *Service
	limit  *limiter
	conns  chan struct{}
	log    *slog.Logger
	udpDNS net.PacketConn
	tcpDNS net.Listener
	udpNTP net.PacketConn
	wg     sync.WaitGroup
}

func (i *instance) serve() {
	i.wg.Add(3)
	go i.loopUDP(i.udpDNS, i.answerDNS)
	go i.loopUDP(i.udpNTP, i.answerNTP)
	go i.loopTCP()
}

func (i *instance) close() error {
	var errs []error
	for _, c := range []interface{ Close() error }{i.udpDNS, i.tcpDNS, i.udpNTP} {
		if c == nil {
			continue
		}
		if err := c.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
		}
	}
	i.wg.Wait()
	return errors.Join(errs...)
}

func (i *instance) loopUDP(conn net.PacketConn, answer func([]byte, time.Time) ([]byte, error)) {
	defer i.wg.Done()
	buf := make([]byte, maxDNSMessage)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				i.log.Error("udp socket stopped", "error", err)
			}
			return
		}
		received := i.svc.opts.Clock.Now()
		if !i.limit.allow() {
			i.log.Debug("rate limit reached, request dropped", "from", from)
			continue
		}
		request := append([]byte{}, buf[:n]...)
		go func() {
			reply, err := answer(request, received)
			if err != nil {
				i.log.Debug("request dropped", "from", from, "error", err)
				return
			}
			if _, err := conn.WriteTo(reply, from); err != nil && !errors.Is(err, net.ErrClosed) {
				i.log.Debug("reply not sent", "to", from, "error", err)
			}
		}()
	}
}

func (i *instance) answerDNS(query []byte, _ time.Time) ([]byte, error) {
	if err := validQuery(query); err != nil {
		return nil, err
	}
	return exchangeUDP(i.svc.dnsServers(), query, i.svc.opts.Timeout)
}

func (i *instance) answerNTP(request []byte, received time.Time) ([]byte, error) {
	if err := validNTPRequest(request); err != nil {
		return nil, err
	}
	return i.svc.ntpAnswer(request, received), nil
}

func (i *instance) loopTCP() {
	defer i.wg.Done()
	for {
		conn, err := i.tcpDNS.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				i.log.Error("tcp socket stopped", "error", err)
			}
			return
		}
		select {
		case i.conns <- struct{}{}:
			go func() {
				defer func() { <-i.conns }()
				i.serveTCP(conn)
			}()
		default:
			i.log.Debug("too many tcp connections, connection refused", "from", conn.RemoteAddr())
			conn.Close()
		}
	}
}

func (i *instance) serveTCP(conn net.Conn) {
	defer conn.Close()
	for {
		if err := conn.SetDeadline(time.Now().Add(tcpIdle)); err != nil {
			return
		}
		query, err := readTCPMessage(conn)
		if err != nil {
			return
		}
		if !i.limit.allow() {
			i.log.Debug("rate limit reached, connection closed", "from", conn.RemoteAddr())
			return
		}
		if err := validQuery(query); err != nil {
			i.log.Debug("request dropped", "from", conn.RemoteAddr(), "error", err)
			return
		}
		answer, err := exchangeTCP(i.svc.dnsServers(), query, i.svc.opts.Timeout)
		if err != nil {
			i.log.Debug("request dropped", "from", conn.RemoteAddr(), "error", err)
			return
		}
		if err := writeTCPMessage(conn, answer); err != nil {
			return
		}
	}
}
