package hostsvc

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func query(id uint16) []byte {
	q := make([]byte, dnsHeaderLen, dnsHeaderLen+17)
	binary.BigEndian.PutUint16(q[0:2], id)
	q[2] = 0x01
	binary.BigEndian.PutUint16(q[4:6], 1)
	q = append(q, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'o', 'r', 'g', 0, 0, 1, 0, 1)
	return q
}

func answerTo(q []byte) []byte {
	a := append([]byte{}, q...)
	a[2] |= 0x80
	return a
}

func ntpRequest(version byte, transmit uint64) []byte {
	r := make([]byte, ntpPacketLen)
	r[0] = version<<3 | ntpModeClient
	r[2] = 6
	binary.BigEndian.PutUint64(r[40:48], transmit)
	return r
}

func TestValidQuery(t *testing.T) {
	if err := validQuery(query(7)); err != nil {
		t.Fatalf("a standard query must pass: %v", err)
	}
	response := answerTo(query(7))
	opcode := query(7)
	opcode[2] |= 2 << 3
	twoQuestions := query(7)
	binary.BigEndian.PutUint16(twoQuestions[4:6], 2)
	noQuestion := query(7)
	binary.BigEndian.PutUint16(noQuestion[4:6], 0)
	for _, c := range []struct {
		name string
		msg  []byte
		want string
	}{
		{"short", query(7)[:11], "11 bytes is shorter than its header"},
		{"response", response, "is a response, not a query"},
		{"opcode", opcode, "opcode 2 is not a standard query"},
		{"two questions", twoQuestions, "carries 2 questions, want 1"},
		{"no question", noQuestion, "carries 0 questions, want 1"},
	} {
		if err := validQuery(c.msg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want %q", c.name, err, c.want)
		}
	}
}

func TestValidAnswer(t *testing.T) {
	q := query(0x1234)
	if err := validAnswer(q, answerTo(q)); err != nil {
		t.Fatalf("a matching answer must pass: %v", err)
	}
	if err := validAnswer(q, answerTo(query(0x1235))); err == nil || !strings.Contains(err.Error(), "id does not match") {
		t.Errorf("another id: error %v", err)
	}
	if err := validAnswer(q, q); err == nil || !strings.Contains(err.Error(), "not flagged as a response") {
		t.Errorf("a query echoed back: error %v", err)
	}
	if err := validAnswer(q, q[:5]); err == nil || !strings.Contains(err.Error(), "shorter than its header") {
		t.Errorf("short: error %v", err)
	}
}

func TestResolvers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	content := "# comment\nsearch example.org\nnameserver 192.0.2.1\nnameserver   2001:db8::1\nnameserver not-an-ip\nnameserver\noptions edns0\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := resolvers(path)
	if err != nil {
		t.Fatalf("resolvers: %v", err)
	}
	if want := []string{"192.0.2.1:53", "[2001:db8::1]:53"}; !reflect.DeepEqual(got, want) {
		t.Errorf("resolvers = %v, want %v", got, want)
	}
	if _, err := resolvers(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("a missing resolv.conf must be reported")
	}
}

func TestTCPMessageRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := writeTCPMessage(&buf, query(9)); err != nil {
		t.Fatalf("writeTCPMessage: %v", err)
	}
	if got := binary.BigEndian.Uint16(buf.Bytes()[:2]); int(got) != len(query(9)) {
		t.Errorf("length prefix = %d, want %d", got, len(query(9)))
	}
	msg, err := readTCPMessage(&buf)
	if err != nil {
		t.Fatalf("readTCPMessage: %v", err)
	}
	if !bytes.Equal(msg, query(9)) {
		t.Errorf("message changed on the way: %x", msg)
	}
	if err := writeTCPMessage(&buf, make([]byte, maxDNSMessage+1)); err == nil {
		t.Error("a message larger than a tcp frame must be refused")
	}
}

func TestValidNTPRequest(t *testing.T) {
	if err := validNTPRequest(ntpRequest(4, 1)); err != nil {
		t.Fatalf("a client request must pass: %v", err)
	}
	server := ntpRequest(4, 1)
	server[0] = 4<<3 | ntpModeServer
	if err := validNTPRequest(server); err == nil || !strings.Contains(err.Error(), "mode 4 is not a client request") {
		t.Errorf("server mode: error %v", err)
	}
	if err := validNTPRequest(ntpRequest(4, 1)[:47]); err == nil || !strings.Contains(err.Error(), "47 bytes is shorter than 48") {
		t.Errorf("short: error %v", err)
	}
}

func TestValidNTPAnswer(t *testing.T) {
	req := ntpRequest(4, 0xdeadbeef)
	ans := hostReply(req, time.Unix(0, 0), time.Unix(0, 0), true, 0)
	if err := validNTPAnswer(req, ans); err != nil {
		t.Fatalf("a matching answer must pass: %v", err)
	}
	if err := validNTPAnswer(ntpRequest(4, 0xdeadbeee), ans); err == nil || !strings.Contains(err.Error(), "origin timestamp") {
		t.Errorf("another request: error %v", err)
	}
	if err := validNTPAnswer(req, req); err == nil || !strings.Contains(err.Error(), "mode 3 is not a server reply") {
		t.Errorf("a request echoed back: error %v", err)
	}
}

func TestNTPTimestamp(t *testing.T) {
	if got := ntpTimestamp(time.Unix(0, 0)); got != 2208988800<<32 {
		t.Errorf("unix epoch = %#x, want %#x", got, uint64(2208988800)<<32)
	}
	if got := ntpTimestamp(time.Unix(1, 500_000_000)); got != (2208988801<<32 | 0x80000000) {
		t.Errorf("half a second = %#x", got)
	}
	if got := ntpShort(1500 * time.Millisecond); got != 0x00018000 {
		t.Errorf("ntpShort(1.5 s) = %#x, want 0x18000", got)
	}
	if got := ntpShort(-time.Second); got != 0 {
		t.Errorf("ntpShort of a negative duration = %#x, want 0", got)
	}
}

func TestHostReply_Synced(t *testing.T) {
	req := ntpRequest(3, 0x0102030405060708)
	received := time.Unix(1000, 0)
	sent := time.Unix(1001, 0)
	r := hostReply(req, received, sent, true, 500*time.Millisecond)

	if r[0] != 0<<6|3<<3|4 {
		t.Errorf("li/vn/mode = %#08b, want li 0, the client version 3, mode 4", r[0])
	}
	if r[1] != 3 {
		t.Errorf("stratum = %d, want 3", r[1])
	}
	if r[2] != 6 {
		t.Errorf("poll = %d, want the client's 6", r[2])
	}
	if int8(r[3]) != -20 {
		t.Errorf("precision = %d, want -20", int8(r[3]))
	}
	if got := binary.BigEndian.Uint32(r[8:12]); got != 0x8000 {
		t.Errorf("root dispersion = %#x, want 0.5 s", got)
	}
	if string(r[12:16]) != "LOCL" {
		t.Errorf("reference id = %q, want LOCL", r[12:16])
	}
	if got := binary.BigEndian.Uint64(r[24:32]); got != 0x0102030405060708 {
		t.Errorf("origin = %#x, want the client's transmit timestamp", got)
	}
	if got := binary.BigEndian.Uint64(r[32:40]); got != ntpTimestamp(received) {
		t.Errorf("receive = %#x, want %#x", got, ntpTimestamp(received))
	}
	if got := binary.BigEndian.Uint64(r[40:48]); got != ntpTimestamp(sent) {
		t.Errorf("transmit = %#x, want %#x", got, ntpTimestamp(sent))
	}
}

func TestHostReply_UnsyncedRaisesTheAlarm(t *testing.T) {
	r := hostReply(ntpRequest(4, 1), time.Unix(0, 0), time.Unix(0, 0), false, 0)
	if li := r[0] >> 6; li != 3 {
		t.Errorf("leap indicator = %d, want 3: an unsynchronized host must not be trusted", li)
	}
	if r[1] != 16 {
		t.Errorf("stratum = %d, want 16 (unsynchronized)", r[1])
	}
}

func TestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLimiter(3, func() time.Time { return now })
	for i := 0; i < 3; i++ {
		if !l.allow() {
			t.Fatalf("request %d refused inside the burst", i+1)
		}
	}
	if l.allow() {
		t.Fatal("the burst is exhausted, the next request must be refused")
	}
	now = now.Add(400 * time.Millisecond)
	if !l.allow() {
		t.Fatal("400 ms refill 1.2 tokens at 3 per second, one request must pass")
	}
	if l.allow() {
		t.Fatal("only one token was refilled")
	}
	now = now.Add(time.Hour)
	for i := 0; i < 3; i++ {
		if !l.allow() {
			t.Fatalf("after a long pause, request %d refused: the bucket holds 3", i+1)
		}
	}
	if l.allow() {
		t.Fatal("the bucket must not grow beyond its rate")
	}
}
