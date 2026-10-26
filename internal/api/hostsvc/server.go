package hostsvcapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	"git.g3e.fr/syonad/two/pkg/db/statefile"
)

const SocketMode = 0o600

type Service interface {
	Open(vpc string) error
	Close(vpc string) error
	List() []string
}

type Server struct {
	svc      Service
	listener net.Listener
	logger   *slog.Logger
}

func Listen(svc Service, path string, logger *slog.Logger) (*Server, error) {
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

	return &Server{svc: svc, listener: listener, logger: logger}, nil
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
	case VerbOpen:
		if err := s.svc.Open(req.VPC); err != nil {
			return failure(err)
		}
		return Response{OK: true}

	case VerbClose:
		if err := s.svc.Close(req.VPC); err != nil {
			return failure(err)
		}
		return Response{OK: true}

	case VerbList:
		return Response{OK: true, VPCs: s.svc.List()}

	default:
		return failure(fmt.Errorf("unknown verb %q", req.Verb))
	}
}
