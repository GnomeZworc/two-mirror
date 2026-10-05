package provision

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const maxSumsSize = 1 << 20

var sha512Pattern = regexp.MustCompile(`^[0-9a-f]{128}$`)

type Fetcher struct {
	Client   *http.Client
	CacheDir string
}

func (f Fetcher) Image(ctx context.Context, img topology.Image) (string, error) {
	if !filepath.IsAbs(f.CacheDir) {
		return "", fmt.Errorf("image cache dir %q must be an absolute path", f.CacheDir)
	}
	name, err := fileName(img.URL)
	if err != nil {
		return "", fmt.Errorf("image %s: %w", img.Name, err)
	}

	sums, err := f.get(ctx, img.Sums, maxSumsSize)
	if err != nil {
		return "", fmt.Errorf("image %s: sums: %w", img.Name, err)
	}
	want, err := expectedSum(sums, name)
	if err != nil {
		return "", fmt.Errorf("image %s: %s: %w", img.Name, img.Sums, err)
	}

	dir := filepath.Join(f.CacheDir, img.Name)
	target := filepath.Join(dir, name)
	if got, err := fileSum(target); err == nil && got == want {
		return target, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("image %s: %w", img.Name, err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := f.download(ctx, img.URL, dir, target, want); err != nil {
		return "", fmt.Errorf("image %s: %w", img.Name, err)
	}
	return target, nil
}

func (f Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return http.DefaultClient
}

func (f Fetcher) open(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	return resp, nil
}

func (f Fetcher) get(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	resp, err := f.open(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: larger than %d bytes", rawURL, limit)
	}
	return data, nil
}

func (f Fetcher) download(ctx context.Context, rawURL, dir, target, want string) error {
	resp, err := f.open(ctx, rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp(dir, filepath.Base(target)+".part-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	h := sha512.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("GET %s: %w", rawURL, err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("GET %s: sha512 %s, want %s", rawURL, got, want)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

func fileName(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	name := path.Base(u.Path)
	if name == "" || name == "." || name == "/" {
		return "", fmt.Errorf("url %q does not name a file", rawURL)
	}
	return name, nil
}

func expectedSum(sums []byte, name string) (string, error) {
	var found []string
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		sum := strings.ToLower(fields[0])
		if !sha512Pattern.MatchString(sum) {
			return "", fmt.Errorf("entry for %s is not a sha512 sum", name)
		}
		found = append(found, sum)
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	switch {
	case len(found) == 0:
		return "", fmt.Errorf("no sum for %s", name)
	case len(found) > 1:
		return "", fmt.Errorf("%d sums for %s", len(found), name)
	}
	return found[0], nil
}

func fileSum(p string) (string, error) {
	file, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha512.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
