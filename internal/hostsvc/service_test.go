package hostsvc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"git.g3e.fr/syonad/two/pkg/db/statefile"
)

type fakeClock struct {
	now      time.Time
	synced   bool
	maxError time.Duration
	err      error
}

func (c fakeClock) Now() time.Time { return c.now }
func (c fakeClock) Sync() (bool, time.Duration, error) {
	return c.synced, c.maxError, c.err
}

type netnsRecorder struct {
	mu      sync.Mutex
	entered []string
	fail    error
}

func (r *netnsRecorder) enter(vpc string, fn func() error) error {
	r.mu.Lock()
	r.entered = append(r.entered, vpc)
	r.mu.Unlock()
	if err := fn(); err != nil {
		return err
	}
	return r.fail
}

func (r *netnsRecorder) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.entered...)
}

func emptyResolv(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func newService(t *testing.T, opts Options) (*Service, *netnsRecorder, string) {
	t.Helper()
	rec := &netnsRecorder{}
	opts.DNSListen = "127.0.0.1:0"
	opts.NTPListen = "127.0.0.1:0"
	if opts.Rate == 0 {
		opts.Rate = 100
	}
	if opts.Resolv == "" {
		opts.Resolv = emptyResolv(t)
	}
	if opts.Clock == nil {
		opts.Clock = fakeClock{now: time.Unix(2000, 0), synced: true}
	}
	if opts.Timeout == 0 {
		opts.Timeout = 300 * time.Millisecond
	}
	opts.Enter = rec.enter
	if opts.Exists == nil {
		opts.Exists = func(string) bool { return true }
	}
	statePath := filepath.Join(t.TempDir(), "hostsvc.state")
	s, err := New(opts, statePath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Shutdown() })
	return s, rec, statePath
}

func (s *Service) addrs(t *testing.T, vpc string) (dnsUDP, dnsTCP, ntp string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, ok := s.vpcs[vpc]
	if !ok {
		t.Fatalf("vpc %s is not open", vpc)
	}
	return inst.udpDNS.LocalAddr().String(), inst.tcpDNS.Addr().String(), inst.udpNTP.LocalAddr().String()
}

func savedVPCs(t *testing.T, path string) []string {
	t.Helper()
	st, err := statefile.New[State](path).Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	return st.VPCs
}

func udpUpstream(t *testing.T, reply func(req []byte) [][]byte) (string, func() int) {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	var mu sync.Mutex
	count := 0
	go func() {
		buf := make([]byte, 65535)
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			mu.Lock()
			count++
			mu.Unlock()
			for _, r := range reply(append([]byte{}, buf[:n]...)) {
				conn.WriteTo(r, from)
			}
		}
	}()
	return conn.LocalAddr().String(), func() int {
		mu.Lock()
		defer mu.Unlock()
		return count
	}
}

func tcpUpstream(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				q, err := readTCPMessage(conn)
				if err != nil {
					return
				}
				writeTCPMessage(conn, answerTo(q))
			}()
		}
	}()
	return l.Addr().String()
}

func deadAddr(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()
	return addr
}

func ask(t *testing.T, addr string, msg []byte, wait time.Duration) ([]byte, error) {
	t.Helper()
	conn, err := net.Dial("udp4", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(wait))
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func TestNew_RefusesANullRate(t *testing.T) {
	if _, err := New(Options{Rate: 0}, filepath.Join(t.TempDir(), "s")); err == nil || !strings.Contains(err.Error(), "rate limit 0") {
		t.Errorf("error %v: a null rate would silently drop every request", err)
	}
}

func TestOpen_EntersTheVPCNetnsAndSavesIt(t *testing.T) {
	s, rec, statePath := newService(t, Options{})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := rec.calls(); !reflect.DeepEqual(got, []string{"vpc-00001"}) {
		t.Errorf("netns entered = %v, want the vpc's", got)
	}
	if got := savedVPCs(t, statePath); !reflect.DeepEqual(got, []string{"vpc-00001"}) {
		t.Errorf("state = %v", got)
	}
	if got := s.List(); !reflect.DeepEqual(got, []string{"vpc-00001"}) {
		t.Errorf("List = %v", got)
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	s, rec, _ := newService(t, Options{})
	for i := 0; i < 2; i++ {
		if err := s.Open("vpc-00001"); err != nil {
			t.Fatalf("Open %d: %v", i+1, err)
		}
	}
	if got := rec.calls(); len(got) != 1 {
		t.Errorf("netns entered %d times, want once: a second open must not bind again", len(got))
	}
}

func TestOpen_InvalidNamesAreRefusedWithoutEnteringAnything(t *testing.T) {
	s, rec, statePath := newService(t, Options{})
	for _, name := range []string{"", ".", "..", "../etc", "a/b", "x\x00y"} {
		if err := s.Open(name); err == nil || !strings.Contains(err.Error(), "invalid vpc name") {
			t.Errorf("Open(%q): error %v", name, err)
		}
		if err := s.Close(name); err == nil || !strings.Contains(err.Error(), "invalid vpc name") {
			t.Errorf("Close(%q): error %v", name, err)
		}
	}
	if got := rec.calls(); len(got) != 0 {
		t.Errorf("netns entered for an invalid name: %v", got)
	}
	if got := savedVPCs(t, statePath); len(got) != 0 {
		t.Errorf("state = %v, want empty", got)
	}
}

func TestOpen_FailureLeavesNothingBehind(t *testing.T) {
	s, rec, statePath := newService(t, Options{})
	rec.fail = errors.New("boom")
	if err := s.Open("vpc-00001"); err == nil || !strings.Contains(err.Error(), "open vpc vpc-00001: boom") {
		t.Fatalf("error %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Errorf("List = %v, want empty after a failed open", got)
	}
	if got := savedVPCs(t, statePath); len(got) != 0 {
		t.Errorf("state = %v, want empty", got)
	}
}

func TestClose_StopsServingAndForgetsTheVPC(t *testing.T) {
	up, _ := udpUpstream(t, func(q []byte) [][]byte { return [][]byte{answerTo(q)} })
	s, _, statePath := newService(t, Options{DNS: []string{up}})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	dns, _, _ := s.addrs(t, "vpc-00001")
	if _, err := ask(t, dns, query(1), time.Second); err != nil {
		t.Fatalf("witness: the open vpc must answer: %v", err)
	}
	if err := s.Close("vpc-00001"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := ask(t, dns, query(2), 300*time.Millisecond); err == nil {
		t.Error("a closed vpc still answers")
	}
	if got := savedVPCs(t, statePath); len(got) != 0 {
		t.Errorf("state = %v, want empty", got)
	}
	if err := s.Close("vpc-00001"); err != nil {
		t.Errorf("closing twice must succeed: %v", err)
	}
}

func TestRestore_ReopensOnlyTheVPCsWhoseNetnsExists(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "hostsvc.state")
	if err := statefile.New[State](statePath).Save(State{VPCs: []string{"vpc-gone", "vpc-here"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rec := &netnsRecorder{}
	s, err := New(Options{
		Rate: 10, DNSListen: "127.0.0.1:0", NTPListen: "127.0.0.1:0", Resolv: emptyResolv(t),
		Enter:  rec.enter,
		Exists: func(vpc string) bool { return vpc == "vpc-here" },
	}, statePath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Shutdown() })
	if err := s.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := rec.calls(); !reflect.DeepEqual(got, []string{"vpc-here"}) {
		t.Errorf("netns entered = %v, want only vpc-here", got)
	}
	if got := savedVPCs(t, statePath); !reflect.DeepEqual(got, []string{"vpc-here"}) {
		t.Errorf("state = %v, the vanished vpc must be dropped", got)
	}
}

func TestDNS_UDPQueryIsRelayedToTheUpstream(t *testing.T) {
	var got []byte
	var mu sync.Mutex
	up, _ := udpUpstream(t, func(q []byte) [][]byte {
		mu.Lock()
		got = q
		mu.Unlock()
		return [][]byte{answerTo(q)}
	})
	s, _, _ := newService(t, Options{DNS: []string{up}})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	dns, _, _ := s.addrs(t, "vpc-00001")
	reply, err := ask(t, dns, query(0x4242), time.Second)
	if err != nil {
		t.Fatalf("no answer: %v", err)
	}
	if !bytes.Equal(reply, answerTo(query(0x4242))) {
		t.Errorf("reply = %x, want the upstream's answer untouched", reply)
	}
	mu.Lock()
	defer mu.Unlock()
	if !bytes.Equal(got, query(0x4242)) {
		t.Errorf("upstream received %x, want the query untouched", got)
	}
}

func TestDNS_SpoofedAnswerIsIgnored(t *testing.T) {
	up, _ := udpUpstream(t, func(q []byte) [][]byte {
		bad := answerTo(q)
		bad[0] ^= 0xff
		return [][]byte{bad, answerTo(q)}
	})
	s, _, _ := newService(t, Options{DNS: []string{up}})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	dns, _, _ := s.addrs(t, "vpc-00001")
	reply, err := ask(t, dns, query(0x1000), time.Second)
	if err != nil {
		t.Fatalf("no answer: %v", err)
	}
	if binary.BigEndian.Uint16(reply[0:2]) != 0x1000 {
		t.Errorf("reply id = %#x, the answer with a foreign id must be skipped", binary.BigEndian.Uint16(reply[0:2]))
	}
}

func TestDNS_NextUpstreamWhenTheFirstIsDown(t *testing.T) {
	up, _ := udpUpstream(t, func(q []byte) [][]byte { return [][]byte{answerTo(q)} })
	s, _, _ := newService(t, Options{DNS: []string{deadAddr(t), up}})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	dns, _, _ := s.addrs(t, "vpc-00001")
	if _, err := ask(t, dns, query(3), 2*time.Second); err != nil {
		t.Fatalf("the second upstream must answer: %v", err)
	}
}

func TestDNS_HostResolverComesAfterTheConfiguredUpstreams(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 192.0.2.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, _, _ := newService(t, Options{DNS: []string{"198.51.100.1:53"}, Resolv: path})
	if got, want := s.dnsServers(), []string{"198.51.100.1:53", "192.0.2.1:53"}; !reflect.DeepEqual(got, want) {
		t.Errorf("dnsServers = %v, want %v", got, want)
	}
}

func TestDNS_InvalidQueryGetsNoAnswerAndIsNotRelayed(t *testing.T) {
	up, count := udpUpstream(t, func(q []byte) [][]byte { return [][]byte{answerTo(q)} })
	s, _, _ := newService(t, Options{DNS: []string{up}})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	dns, _, _ := s.addrs(t, "vpc-00001")
	if _, err := ask(t, dns, answerTo(query(5)), 300*time.Millisecond); err == nil {
		t.Error("a response sent to the relay must not be answered")
	}
	if _, err := ask(t, dns, query(6), time.Second); err != nil {
		t.Fatalf("witness: a valid query must be answered: %v", err)
	}
	if n := count(); n != 1 {
		t.Errorf("upstream saw %d messages, want only the valid one", n)
	}
}

func TestDNS_TCPQueryIsRelayed(t *testing.T) {
	s, _, _ := newService(t, Options{DNS: []string{tcpUpstream(t)}})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, tcp, _ := s.addrs(t, "vpc-00001")
	conn, err := net.Dial("tcp4", tcp)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	for id := uint16(1); id <= 2; id++ {
		if err := writeTCPMessage(conn, query(id)); err != nil {
			t.Fatalf("write: %v", err)
		}
		reply, err := readTCPMessage(conn)
		if err != nil {
			t.Fatalf("query %d: no answer: %v", id, err)
		}
		if !bytes.Equal(reply, answerTo(query(id))) {
			t.Errorf("query %d: reply %x", id, reply)
		}
	}
}

func TestDNS_RateLimitPerVPC(t *testing.T) {
	up, _ := udpUpstream(t, func(q []byte) [][]byte { return [][]byte{answerTo(q)} })
	s, _, _ := newService(t, Options{DNS: []string{up}, Rate: 2})
	for _, vpc := range []string{"vpc-00001", "vpc-00002"} {
		if err := s.Open(vpc); err != nil {
			t.Fatalf("Open %s: %v", vpc, err)
		}
	}
	dns1, _, _ := s.addrs(t, "vpc-00001")
	dns2, _, _ := s.addrs(t, "vpc-00002")
	for i := 0; i < 2; i++ {
		if _, err := ask(t, dns1, query(uint16(i)), time.Second); err != nil {
			t.Fatalf("query %d within the burst: %v", i+1, err)
		}
	}
	if _, err := ask(t, dns1, query(9), 200*time.Millisecond); err == nil {
		t.Error("the third query in a row must be dropped at 2 per second")
	}
	if _, err := ask(t, dns2, query(10), time.Second); err != nil {
		t.Errorf("another vpc has its own bucket: %v", err)
	}
}

func TestNTP_HostClockWithoutUpstream(t *testing.T) {
	clock := fakeClock{now: time.Unix(5000, 0), synced: true, maxError: time.Second}
	s, _, _ := newService(t, Options{Clock: clock})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, _, ntp := s.addrs(t, "vpc-00001")
	req := ntpRequest(4, 0xabcdef)
	reply, err := ask(t, ntp, req, time.Second)
	if err != nil {
		t.Fatalf("no answer: %v", err)
	}
	if err := validNTPAnswer(req, reply); err != nil {
		t.Fatalf("invalid answer: %v", err)
	}
	if got := binary.BigEndian.Uint64(reply[40:48]); got != ntpTimestamp(clock.now) {
		t.Errorf("transmit = %#x, want the host clock %#x", got, ntpTimestamp(clock.now))
	}
	if reply[0]>>6 != 0 || reply[1] != 3 {
		t.Errorf("li %d stratum %d, want 0 and 3 for a synced host", reply[0]>>6, reply[1])
	}
}

func TestNTP_UnreadableClockStatusIsAnsweredAsUnsynced(t *testing.T) {
	s, _, _ := newService(t, Options{Clock: fakeClock{now: time.Unix(5000, 0), synced: true, err: errors.New("eperm")}})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, _, ntp := s.addrs(t, "vpc-00001")
	reply, err := ask(t, ntp, ntpRequest(4, 1), time.Second)
	if err != nil {
		t.Fatalf("no answer: %v", err)
	}
	if reply[0]>>6 != 3 {
		t.Errorf("li = %d, want 3 when the clock status cannot be read", reply[0]>>6)
	}
}

func TestNTP_UpstreamIsPreferredAndChecked(t *testing.T) {
	up, _ := udpUpstream(t, func(req []byte) [][]byte {
		spoofed := hostReply(ntpRequest(4, 0), time.Unix(0, 0), time.Unix(0, 0), true, 0)
		real := hostReply(req, time.Unix(7000, 0), time.Unix(7000, 0), true, 0)
		real[1] = 1
		return [][]byte{spoofed, real}
	})
	s, _, _ := newService(t, Options{NTP: []string{up}})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, _, ntp := s.addrs(t, "vpc-00001")
	req := ntpRequest(4, 0x55)
	reply, err := ask(t, ntp, req, time.Second)
	if err != nil {
		t.Fatalf("no answer: %v", err)
	}
	if reply[1] != 1 {
		t.Errorf("stratum = %d, want the upstream's 1 (spoofed answer skipped, host clock unused)", reply[1])
	}
}

func TestNTP_HostClockWhenUpstreamsFail(t *testing.T) {
	clock := fakeClock{now: time.Unix(6000, 0), synced: true}
	s, _, _ := newService(t, Options{NTP: []string{deadAddr(t)}, Clock: clock})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, _, ntp := s.addrs(t, "vpc-00001")
	reply, err := ask(t, ntp, ntpRequest(4, 2), 2*time.Second)
	if err != nil {
		t.Fatalf("no answer: %v", err)
	}
	if got := binary.BigEndian.Uint64(reply[40:48]); got != ntpTimestamp(clock.now) {
		t.Errorf("transmit = %#x, want the host clock as fallback", got)
	}
}

func TestNTP_InvalidRequestGetsNoAnswer(t *testing.T) {
	s, _, _ := newService(t, Options{})
	if err := s.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, _, ntp := s.addrs(t, "vpc-00001")
	if _, err := ask(t, ntp, []byte("not ntp"), 300*time.Millisecond); err == nil {
		t.Error("a short packet must not be answered")
	}
}
