package provision

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const imageName = "debian-12-generic-amd64.qcow2"

type mirror struct {
	mu     sync.Mutex
	files  map[string][]byte
	status map[string]int
	hits   map[string]int
	server *httptest.Server
}

func newMirror(t *testing.T) *mirror {
	t.Helper()
	m := &mirror{files: map[string][]byte{}, status: map[string]int{}, hits: map[string]int{}}
	m.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.hits[r.URL.Path]++
		if code := m.status[r.URL.Path]; code != 0 {
			w.WriteHeader(code)
			return
		}
		data, ok := m.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(m.server.Close)
	return m
}

func (m *mirror) put(path string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = data
}

func (m *mirror) count(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hits[path]
}

func (m *mirror) image() topology.Image {
	return topology.Image{
		Name: "debian12",
		URL:  m.server.URL + "/bookworm/latest/" + imageName,
		Sums: m.server.URL + "/bookworm/latest/SHA512SUMS",
	}
}

func sum(data []byte) string {
	h := sha512.Sum512(data)
	return hex.EncodeToString(h[:])
}

func publish(m *mirror, content []byte) {
	m.put("/bookworm/latest/"+imageName, content)
	m.put("/bookworm/latest/SHA512SUMS", []byte(
		sum([]byte("other"))+"  debian-12-genericcloud-amd64.qcow2\n"+
			sum(content)+"  "+imageName+"\n"+
			sum([]byte("raw"))+"  debian-12-generic-amd64.raw\n"))
}

func fetch(t *testing.T, m *mirror, cache string) string {
	t.Helper()
	got, err := Fetcher{Client: m.server.Client(), CacheDir: cache}.Image(context.Background(), m.image())
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	return got
}

func fetchError(t *testing.T, m *mirror, cache string) string {
	t.Helper()
	_, err := Fetcher{Client: m.server.Client(), CacheDir: cache}.Image(context.Background(), m.image())
	if err == nil {
		t.Fatal("Image: no error")
	}
	return err.Error()
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func TestImage_DownloadsAndVerifies(t *testing.T) {
	m := newMirror(t)
	publish(m, []byte("qcow2 image"))
	cache := t.TempDir()

	got := fetch(t, m, cache)

	if want := filepath.Join(cache, "debian12", "debian-12-generic-amd64.qcow2"); got != want {
		t.Errorf("path = %s, want %s", got, want)
	}
	data, err := os.ReadFile(got)
	if err != nil || string(data) != "qcow2 image" {
		t.Errorf("content = %q, %v", data, err)
	}
	info, err := os.Stat(got)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, %v", info.Mode().Perm(), err)
	}
	if names := entries(t, filepath.Join(cache, "debian12")); len(names) != 1 {
		t.Errorf("cache dir holds %v, want only the image", names)
	}
}

func TestImage_ReusesAVerifiedCacheButAlwaysRereadsTheSums(t *testing.T) {
	m := newMirror(t)
	publish(m, []byte("qcow2 image"))
	cache := t.TempDir()

	fetch(t, m, cache)
	fetch(t, m, cache)

	if n := m.count("/bookworm/latest/" + imageName); n != 1 {
		t.Errorf("image downloaded %d times, want 1", n)
	}
	if n := m.count("/bookworm/latest/SHA512SUMS"); n != 2 {
		t.Errorf("sums read %d times, want 2", n)
	}
}

func TestImage_ReplacesACorruptedCache(t *testing.T) {
	m := newMirror(t)
	publish(m, []byte("qcow2 image"))
	cache := t.TempDir()
	path := fetch(t, m, cache)
	if err := os.WriteFile(path, []byte("bit rot"), 0o644); err != nil {
		t.Fatal(err)
	}

	fetch(t, m, cache)

	if data, _ := os.ReadFile(path); string(data) != "qcow2 image" {
		t.Errorf("content = %q, want the published image", data)
	}
	if n := m.count("/bookworm/latest/" + imageName); n != 2 {
		t.Errorf("image downloaded %d times, want 2", n)
	}
}

func TestImage_FollowsANewReleaseOfTheSameFile(t *testing.T) {
	m := newMirror(t)
	publish(m, []byte("release 1"))
	cache := t.TempDir()
	path := fetch(t, m, cache)

	publish(m, []byte("release 2"))
	fetch(t, m, cache)

	if data, _ := os.ReadFile(path); string(data) != "release 2" {
		t.Errorf("content = %q, want release 2", data)
	}
}

func TestImage_RejectsAMismatchAndLeavesNothing(t *testing.T) {
	m := newMirror(t)
	publish(m, []byte("qcow2 image"))
	m.put("/bookworm/latest/"+imageName, []byte("tampered"))
	cache := t.TempDir()

	msg := fetchError(t, m, cache)

	if !strings.Contains(msg, "sha512") {
		t.Errorf("error = %q, want a sha512 mismatch", msg)
	}
	if names := entries(t, filepath.Join(cache, "debian12")); len(names) != 0 {
		t.Errorf("cache dir holds %v after a mismatch, want nothing", names)
	}
}

func TestImage_KeepsTheVerifiedCacheWhenANewDownloadFails(t *testing.T) {
	m := newMirror(t)
	publish(m, []byte("release 1"))
	cache := t.TempDir()
	path := fetch(t, m, cache)

	publish(m, []byte("release 2"))
	m.put("/bookworm/latest/"+imageName, []byte("truncated"))
	fetchError(t, m, cache)

	if data, _ := os.ReadFile(path); string(data) != "release 1" {
		t.Errorf("content = %q, want release 1 kept", data)
	}
	if names := entries(t, filepath.Join(cache, "debian12")); len(names) != 1 {
		t.Errorf("cache dir holds %v, want only the image", names)
	}
}

func TestImage_AcceptsBinaryModeSumLines(t *testing.T) {
	m := newMirror(t)
	content := []byte("qcow2 image")
	m.put("/bookworm/latest/"+imageName, content)
	m.put("/bookworm/latest/SHA512SUMS", []byte(strings.ToUpper(sum(content))+" *"+imageName+"\n"))

	fetch(t, m, t.TempDir())
}

func TestImage_SumsRejections(t *testing.T) {
	content := []byte("qcow2 image")
	cases := map[string]struct {
		sums string
		want string
	}{
		"no entry":          {sum(content) + "  debian-12-generic-arm64.qcow2\n", "no sum for " + imageName},
		"two entries":       {sum(content) + "  " + imageName + "\n" + sum(content) + "  " + imageName + "\n", "2 sums for " + imageName},
		"not sha512":        {"d41d8cd98f00b204e9800998ecf8427e  " + imageName + "\n", "is not a sha512 sum"},
		"prefix of name":    {sum(content) + "  " + imageName + ".sig\n", "no sum for " + imageName},
		"name in directory": {sum(content) + "  nested/" + imageName + "\n", "no sum for " + imageName},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m := newMirror(t)
			m.put("/bookworm/latest/"+imageName, content)
			m.put("/bookworm/latest/SHA512SUMS", []byte(c.sums))
			if msg := fetchError(t, m, t.TempDir()); !strings.Contains(msg, c.want) {
				t.Errorf("error = %q, want %q", msg, c.want)
			}
			if n := m.count("/bookworm/latest/" + imageName); n != 0 {
				t.Errorf("image downloaded %d times before the sums were trusted", n)
			}
		})
	}
}

func TestImage_RefusesOversizedSums(t *testing.T) {
	m := newMirror(t)
	m.put("/bookworm/latest/SHA512SUMS", make([]byte, 1<<20+1))
	if msg := fetchError(t, m, t.TempDir()); !strings.Contains(msg, "larger than 1048576 bytes") {
		t.Errorf("error = %q", msg)
	}
}

func TestImage_AcceptsSumsOfExactlyTheLimit(t *testing.T) {
	m := newMirror(t)
	content := []byte("qcow2 image")
	line := sum(content) + "  " + imageName + "\n"
	m.put("/bookworm/latest/"+imageName, content)
	m.put("/bookworm/latest/SHA512SUMS", []byte(strings.Repeat("\n", 1<<20-len(line))+line))

	fetch(t, m, t.TempDir())
}

func TestImage_HTTPErrors(t *testing.T) {
	for _, path := range []string{"/bookworm/latest/SHA512SUMS", "/bookworm/latest/" + imageName} {
		t.Run(path, func(t *testing.T) {
			m := newMirror(t)
			publish(m, []byte("qcow2 image"))
			m.status[path] = http.StatusServiceUnavailable
			cache := t.TempDir()
			if msg := fetchError(t, m, cache); !strings.Contains(msg, "503") {
				t.Errorf("error = %q, want the http status", msg)
			}
			if names := entries(t, filepath.Join(cache, "debian12")); len(names) != 0 {
				t.Errorf("cache dir holds %v, want nothing", names)
			}
		})
	}
}

func TestImage_RefusesARelativeCacheDir(t *testing.T) {
	m := newMirror(t)
	publish(m, []byte("qcow2 image"))
	if msg := fetchError(t, m, "cache"); !strings.Contains(msg, "must be an absolute path") {
		t.Errorf("error = %q", msg)
	}
	if n := m.count("/bookworm/latest/SHA512SUMS"); n != 0 {
		t.Errorf("sums read %d times", n)
	}
}

func TestImage_RefusesAURLWithoutFileName(t *testing.T) {
	m := newMirror(t)
	img := m.image()
	img.URL = m.server.URL + "/"
	_, err := Fetcher{Client: m.server.Client(), CacheDir: t.TempDir()}.Image(context.Background(), img)
	if err == nil || !strings.Contains(err.Error(), "does not name a file") {
		t.Errorf("error = %v", err)
	}
}

func TestImage_RejectsAnInterruptedDownloadAndLeavesNothing(t *testing.T) {
	content := []byte("qcow2 image")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHA512SUMS") {
			w.Write([]byte(sum(content) + "  " + imageName + "\n"))
			return
		}
		w.Header().Set("Content-Length", "1000")
		w.Write(content[:4])
	}))
	defer server.Close()
	cache := t.TempDir()
	img := topology.Image{Name: "debian12", URL: server.URL + "/" + imageName, Sums: server.URL + "/SHA512SUMS"}

	_, err := Fetcher{Client: server.Client(), CacheDir: cache}.Image(context.Background(), img)

	if err == nil || !strings.Contains(err.Error(), "unexpected EOF") {
		t.Errorf("error = %v, want an interrupted transfer", err)
	}
	if names := entries(t, filepath.Join(cache, "debian12")); len(names) != 0 {
		t.Errorf("cache dir holds %v, want nothing", names)
	}
}
