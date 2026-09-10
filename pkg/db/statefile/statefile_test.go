package statefile

import (
	"os"
	"path/filepath"
	"testing"
)

type payload struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

func path(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "component.state")
}

func TestLoad_CreatesTheFileWhenAbsent(t *testing.T) {
	p := path(t)
	f := New[payload](p)

	value, err := f.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if value.Name != "" || len(value.Items) != 0 {
		t.Errorf("value = %+v, want the zero value", value)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("the file must be created on load: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600", got)
	}
}

func TestLoad_CreatesTheDirectoryWhenAbsent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "deeper", "component.state")

	if _, err := New[payload](p).Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	info, err := os.Stat(filepath.Dir(p))
	if err != nil {
		t.Fatalf("the directory must be created: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("directory mode = %o, want 700", got)
	}
}

func TestLoad_EmptyFileYieldsTheZeroValue(t *testing.T) {
	p := path(t)
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	value, err := New[payload](p).Load()
	if err != nil {
		t.Fatalf("an empty file is a valid starting point: %v", err)
	}
	if value.Name != "" {
		t.Errorf("value = %+v, want the zero value", value)
	}
}

func TestLoad_CorruptedFileIsReported(t *testing.T) {
	p := path(t)
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := New[payload](p).Load(); err == nil {
		t.Fatal("a corrupted file must be reported, not silently ignored")
	}
}

func TestSave_RoundTrips(t *testing.T) {
	p := path(t)
	f := New[payload](p)

	want := payload{Name: "vp-admin_br-000001", Items: []string{"a", "b"}}
	if err := f.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := New[payload](p).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != want.Name || len(got.Items) != len(want.Items) {
		t.Errorf("value = %+v, want %+v", got, want)
	}
}

func TestSave_RestoresTheModeAfterAnExternalChmod(t *testing.T) {
	p := path(t)
	f := New[payload](p)
	if _, err := f.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	if err := f.Save(payload{Name: "x"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600: each save must replace the file, not edit it in place", got)
	}
}

func TestSave_LeavesNoTemporaryFileBehind(t *testing.T) {
	p := path(t)
	f := New[payload](p)

	for range 3 {
		if err := f.Save(payload{Name: "x"}); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	entries, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the state file: %v", len(entries), entries)
	}
}

func TestSave_DoesNotTruncateOnEncodingFailure(t *testing.T) {
	p := path(t)
	good := New[payload](p)
	if err := good.Save(payload{Name: "kept"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	broken := New[chan int](p)
	if err := broken.Save(make(chan int)); err == nil {
		t.Fatal("an unencodable value must be reported")
	}

	got, err := good.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "kept" {
		t.Errorf("value = %+v, want the previous state untouched", got)
	}
}

func TestRemove_DeletesTheFile(t *testing.T) {
	p := path(t)
	f := New[payload](p)
	if _, err := f.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := f.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("the file must be gone, got %v", err)
	}
}

func TestRemove_OnAnAbsentFileIsNotAnError(t *testing.T) {
	if err := New[payload](path(t)).Remove(); err != nil {
		t.Errorf("removing an absent file must be idempotent, got %v", err)
	}
}

func TestPath_ReportsTheFileItOwns(t *testing.T) {
	p := path(t)
	if got := New[payload](p).Path(); got != p {
		t.Errorf("Path = %s, want %s", got, p)
	}
}
