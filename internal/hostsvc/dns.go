package hostsvc

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

const (
	dnsHeaderLen  = 12
	maxDNSMessage = 65535
)

func validQuery(b []byte) error {
	if len(b) < dnsHeaderLen {
		return fmt.Errorf("dns message of %d bytes is shorter than its header", len(b))
	}
	if b[2]&0x80 != 0 {
		return errors.New("dns message is a response, not a query")
	}
	if op := (b[2] >> 3) & 0x0f; op != 0 {
		return fmt.Errorf("dns opcode %d is not a standard query", op)
	}
	if n := binary.BigEndian.Uint16(b[4:6]); n != 1 {
		return fmt.Errorf("dns query carries %d questions, want 1", n)
	}
	return nil
}

func validAnswer(query, answer []byte) error {
	if len(answer) < dnsHeaderLen {
		return fmt.Errorf("dns answer of %d bytes is shorter than its header", len(answer))
	}
	if answer[0] != query[0] || answer[1] != query[1] {
		return errors.New("dns answer id does not match the query")
	}
	if answer[2]&0x80 == 0 {
		return errors.New("dns answer is not flagged as a response")
	}
	return nil
}

func resolvers(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var out []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "nameserver" || net.ParseIP(fields[1]) == nil {
			continue
		}
		out = append(out, net.JoinHostPort(fields[1], "53"))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return out, nil
}

func exchangeUDP(servers []string, query []byte, timeout time.Duration) ([]byte, error) {
	var errs []error
	for _, server := range servers {
		answer, err := exchangeUDPOne(server, query, timeout)
		if err == nil {
			return answer, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", server, err))
	}
	return nil, fmt.Errorf("no dns upstream answered: %w", errors.Join(errs...))
}

func exchangeUDPOne(server string, query []byte, timeout time.Duration) ([]byte, error) {
	conn, err := net.DialTimeout("udp", server, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if _, err := conn.Write(query); err != nil {
		return nil, err
	}
	buf := make([]byte, maxDNSMessage)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		if validAnswer(query, buf[:n]) == nil {
			return buf[:n], nil
		}
	}
}

func exchangeTCP(servers []string, query []byte, timeout time.Duration) ([]byte, error) {
	var errs []error
	for _, server := range servers {
		answer, err := exchangeTCPOne(server, query, timeout)
		if err == nil {
			return answer, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", server, err))
	}
	return nil, fmt.Errorf("no dns upstream answered: %w", errors.Join(errs...))
}

func exchangeTCPOne(server string, query []byte, timeout time.Duration) ([]byte, error) {
	conn, err := net.DialTimeout("tcp", server, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if err := writeTCPMessage(conn, query); err != nil {
		return nil, err
	}
	answer, err := readTCPMessage(conn)
	if err != nil {
		return nil, err
	}
	if err := validAnswer(query, answer); err != nil {
		return nil, err
	}
	return answer, nil
}

func readTCPMessage(r io.Reader) ([]byte, error) {
	var size [2]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return nil, err
	}
	msg := make([]byte, binary.BigEndian.Uint16(size[:]))
	if _, err := io.ReadFull(r, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func writeTCPMessage(w io.Writer, msg []byte) error {
	if len(msg) > maxDNSMessage {
		return fmt.Errorf("dns message of %d bytes does not fit a tcp frame", len(msg))
	}
	frame := make([]byte, 2+len(msg))
	binary.BigEndian.PutUint16(frame, uint16(len(msg)))
	copy(frame[2:], msg)
	_, err := w.Write(frame)
	return err
}
