package hostsvc

import (
	"bytes"
	"testing"
	"time"
)

func FuzzValidQuery(f *testing.F) {
	f.Add(query(1))
	f.Add(answerTo(query(1)))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		if validQuery(b) == nil && len(b) < dnsHeaderLen {
			t.Fatalf("a %d-byte message passed", len(b))
		}
	})
}

func FuzzValidAnswer(f *testing.F) {
	f.Add(query(1), answerTo(query(1)))
	f.Add(query(1), []byte{1})
	f.Fuzz(func(t *testing.T, q, a []byte) {
		if validQuery(q) != nil {
			return
		}
		if validAnswer(q, a) == nil && (len(a) < dnsHeaderLen || a[0] != q[0] || a[1] != q[1]) {
			t.Fatalf("answer %x passed for query %x", a, q)
		}
	})
}

func FuzzReadTCPMessage(f *testing.F) {
	f.Add([]byte{0, 3, 'a', 'b', 'c'})
	f.Add([]byte{0xff, 0xff})
	f.Fuzz(func(t *testing.T, b []byte) {
		msg, err := readTCPMessage(bytes.NewReader(b))
		if err == nil && len(msg) != int(b[0])<<8|int(b[1]) {
			t.Fatalf("message of %d bytes for prefix %x", len(msg), b[:2])
		}
	})
}

func FuzzHostReply(f *testing.F) {
	f.Add(ntpRequest(4, 1))
	f.Add([]byte("short"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if validNTPRequest(b) != nil {
			return
		}
		r := hostReply(b, time.Unix(0, 0), time.Unix(1, 0), true, 0)
		if err := validNTPAnswer(b, r); err != nil {
			t.Fatalf("own reply rejected: %v", err)
		}
	})
}
