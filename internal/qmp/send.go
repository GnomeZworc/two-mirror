package qmp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

func Send(socketPath string, commands []string) ([]json.RawMessage, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("qmp dial: %w", err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)

	if _, err := r.ReadString('\n'); err != nil {
		return nil, fmt.Errorf("qmp read greeting: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(`{ "execute": "qmp_capabilities" }`)
	for _, cmd := range commands {
		sb.WriteByte('\n')
		sb.WriteString(cmd)
	}
	sb.WriteByte('\n')

	if _, err := fmt.Fprint(conn, sb.String()); err != nil {
		return nil, fmt.Errorf("qmp write: %w", err)
	}

	if _, err := r.ReadString('\n'); err != nil {
		return nil, fmt.Errorf("qmp read capabilities response: %w", err)
	}

	var results []json.RawMessage
	for range commands {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("qmp read response: %w", err)
		}
		results = append(results, json.RawMessage(line))
	}

	return results, nil
}
