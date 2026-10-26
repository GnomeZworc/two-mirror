package hostsvcclient

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	hostsvcapi "git.g3e.fr/syonad/two/internal/api/hostsvc"
)

type fakeService struct {
	mu     sync.Mutex
	open   map[string]bool
	failOn string
}

func (f *fakeService) Open(vpc string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if vpc == f.failOn {
		return errors.New("netns not found")
	}
	f.open[vpc] = true
	return nil
}

func (f *fakeService) Close(vpc string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if vpc == f.failOn {
		return errors.New("close failed")
	}
	delete(f.open, vpc)
	return nil
}

func (f *fakeService) List() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for vpc := range f.open {
		out = append(out, vpc)
	}
	return out
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hostsvc")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func serve(t *testing.T) (*Client, *fakeService, string) {
	t.Helper()
	svc := &fakeService{open: map[string]bool{}, failOn: "vpc-broken"}
	socketPath := hostsvcapi.SocketPath(shortTempDir(t))
	server, err := hostsvcapi.Listen(svc, socketPath, discardLogger())
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go server.Serve()
	t.Cleanup(func() { server.Close() })
	return New(socketPath), svc, socketPath
}

func TestListen_SocketIsOwnerOnly(t *testing.T) {
	_, _, socketPath := serve(t)
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600: whoever reaches it opens sockets in any vpc", got)
	}
}

func TestListen_ReplacesAStaleSocketFile(t *testing.T) {
	socketPath := filepath.Join(shortTempDir(t), "stale.sock")
	if err := os.WriteFile(socketPath, []byte("leftover"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	server, err := hostsvcapi.Listen(&fakeService{open: map[string]bool{}}, socketPath, discardLogger())
	if err != nil {
		t.Fatalf("a socket left by an unclean stop must not block startup: %v", err)
	}
	server.Close()
}

func TestOpenCloseList(t *testing.T) {
	client, svc, _ := serve(t)
	if err := client.Open("vpc-00001"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := client.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"vpc-00001"}) {
		t.Errorf("List = %v", got)
	}
	if err := client.Close("vpc-00001"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(svc.List()) != 0 {
		t.Errorf("the vpc is still open on the service: %v", svc.List())
	}
}

func TestServiceErrorsAreReported(t *testing.T) {
	client, _, _ := serve(t)
	if err := client.Open("vpc-broken"); err == nil || !strings.Contains(err.Error(), "open refused: netns not found") {
		t.Errorf("Open: error %v", err)
	}
	if err := client.Close("vpc-broken"); err == nil || !strings.Contains(err.Error(), "close refused: close failed") {
		t.Errorf("Close: error %v", err)
	}
}

func TestUnknownVerbAndMalformedRequest(t *testing.T) {
	_, _, socketPath := serve(t)
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("{\"verb\":\"reboot\"}\nnot json\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	buf := make([]byte, 4096)
	var all string
	for !strings.Contains(all, "malformed request") {
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("Read: %v (got %q)", err, all)
		}
		all += string(buf[:n])
	}
	if !strings.Contains(all, `unknown verb \"reboot\"`) {
		t.Errorf("replies = %q, want the unknown verb refused", all)
	}
}

func TestClient_NoServer(t *testing.T) {
	client := New(filepath.Join(shortTempDir(t), "absent.sock"))
	if err := client.Open("vpc-00001"); err == nil || !strings.Contains(err.Error(), "dial") {
		t.Errorf("error %v, want a dial failure", err)
	}
}
