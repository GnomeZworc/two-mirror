package hostsvc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

const (
	ntpPacketLen   = 48
	ntpModeClient  = 3
	ntpModeServer  = 4
	ntpUnsynced    = 3
	ntpStratumHost = 3
	ntpStratumNone = 16
	ntpPrecision   = -20
	ntpEpochOffset = 2208988800
)

var ntpRefID = [4]byte{'L', 'O', 'C', 'L'}

type Clock interface {
	Now() time.Time
	Sync() (synced bool, maxError time.Duration, err error)
}

func validNTPRequest(b []byte) error {
	if len(b) < ntpPacketLen {
		return fmt.Errorf("ntp packet of %d bytes is shorter than %d", len(b), ntpPacketLen)
	}
	if mode := b[0] & 0x07; mode != ntpModeClient {
		return fmt.Errorf("ntp mode %d is not a client request", mode)
	}
	return nil
}

func validNTPAnswer(request, answer []byte) error {
	if len(answer) < ntpPacketLen {
		return fmt.Errorf("ntp answer of %d bytes is shorter than %d", len(answer), ntpPacketLen)
	}
	if mode := answer[0] & 0x07; mode != ntpModeServer {
		return fmt.Errorf("ntp answer mode %d is not a server reply", mode)
	}
	if !bytes.Equal(answer[24:32], request[40:48]) {
		return errors.New("ntp answer origin timestamp does not match the request")
	}
	return nil
}

func ntpTimestamp(t time.Time) uint64 {
	secs := uint64(t.Unix() + ntpEpochOffset)
	frac := (uint64(t.Nanosecond()) << 32) / uint64(time.Second)
	return secs<<32 | frac
}

func ntpShort(d time.Duration) uint32 {
	if d < 0 {
		d = 0
	}
	secs := uint64(d / time.Second)
	if secs > 0xffff {
		return 0xffffffff
	}
	frac := (uint64(d%time.Second) << 16) / uint64(time.Second)
	return uint32(secs<<16 | frac)
}

func hostReply(request []byte, received, sent time.Time, synced bool, maxError time.Duration) []byte {
	reply := make([]byte, ntpPacketLen)
	version := (request[0] >> 3) & 0x07
	leap, stratum := byte(0), byte(ntpStratumHost)
	if !synced {
		leap, stratum = ntpUnsynced, ntpStratumNone
	}
	reply[0] = leap<<6 | version<<3 | ntpModeServer
	reply[1] = stratum
	reply[2] = request[2]
	precision := int8(ntpPrecision)
	reply[3] = byte(precision)
	binary.BigEndian.PutUint32(reply[8:12], ntpShort(maxError))
	copy(reply[12:16], ntpRefID[:])
	binary.BigEndian.PutUint64(reply[16:24], ntpTimestamp(received))
	copy(reply[24:32], request[40:48])
	binary.BigEndian.PutUint64(reply[32:40], ntpTimestamp(received))
	binary.BigEndian.PutUint64(reply[40:48], ntpTimestamp(sent))
	return reply
}

func exchangeNTP(servers []string, request []byte, timeout time.Duration) ([]byte, error) {
	var errs []error
	for _, server := range servers {
		answer, err := exchangeNTPOne(server, request, timeout)
		if err == nil {
			return answer, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", server, err))
	}
	return nil, fmt.Errorf("no ntp upstream answered: %w", errors.Join(errs...))
}

func exchangeNTPOne(server string, request []byte, timeout time.Duration) ([]byte, error) {
	conn, err := net.DialTimeout("udp", server, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if _, err := conn.Write(request[:ntpPacketLen]); err != nil {
		return nil, err
	}
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		if validNTPAnswer(request, buf[:n]) == nil {
			return buf[:n], nil
		}
	}
}
