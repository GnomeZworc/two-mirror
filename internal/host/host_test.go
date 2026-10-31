package host

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func debian12() fstest.MapFS {
	fsys := fstest.MapFS{}
	for _, name := range []string{
		"proc/cpuinfo", "proc/meminfo", "proc/stat", "proc/uptime", "proc/loadavg",
		"proc/sys/kernel/osrelease", "proc/sys/kernel/hostname", "etc/os-release",
	} {
		data, err := os.ReadFile("testdata/debian12/" + name)
		if err != nil {
			panic(err)
		}
		fsys[name] = &fstest.MapFile{Data: data}
	}
	return fsys
}

func TestReadFacts_Debian12(t *testing.T) {
	got, err := ReadFacts(debian12())
	if err != nil {
		t.Fatalf("ReadFacts: %v", err)
	}
	want := Facts{
		Hostname:    "hv1",
		CPU:         CPU{Vendor: "AuthenticAMD", Model: "QEMU Virtual CPU version 2.5+", Sockets: 1, Cores: 2, Threads: 2},
		MemoryBytes: 2010432 * 1024,
		OS:          OS{ID: "debian", VersionID: "12", PrettyName: "Debian GNU/Linux 12 (bookworm)"},
		Kernel:      "6.8.0-50-generic",
	}
	if got != want {
		t.Errorf("faits %+v, %+v attendus", got, want)
	}
}

func TestReadState_Debian12(t *testing.T) {
	got, err := ReadState(debian12())
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	want := State{BootTime: 1792210880, UptimeSeconds: 1243445.41, Load: [3]float64{0.08, 0.03, 0.04}}
	if got != want {
		t.Errorf("état %+v, %+v attendu", got, want)
	}
}

func TestParseCPUInfo_SocketsCoresAndThreadsAreDistinct(t *testing.T) {
	data, err := os.ReadFile("testdata/cpuinfo-2x2x2")
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseCPUInfo(string(data))
	if err != nil {
		t.Fatalf("parseCPUInfo: %v", err)
	}
	if got.Sockets != 2 || got.Cores != 4 || got.Threads != 8 {
		t.Errorf("%d sockets, %d cœurs, %d threads ; 2, 4 et 8 attendus", got.Sockets, got.Cores, got.Threads)
	}
}

func TestParseCPUInfo_RefusesMissingTopology(t *testing.T) {
	data, err := os.ReadFile("testdata/debian12/proc/cpuinfo")
	if err != nil {
		t.Fatal(err)
	}
	for label, input := range map[string]string{
		"vide":             "",
		"sans physical id": strings.ReplaceAll(string(data), "physical id\t: 0\n", ""),
		"sans core id":     strings.ReplaceAll(strings.ReplaceAll(string(data), "core id\t\t: 0\n", ""), "core id\t\t: 1\n", ""),
	} {
		if _, err := parseCPUInfo(input); err == nil {
			t.Errorf("%s : accepté", label)
		}
	}
}

func TestReadFacts_FailsOnAMissingFile(t *testing.T) {
	for _, name := range []string{"proc/cpuinfo", "proc/meminfo", "etc/os-release", "proc/sys/kernel/osrelease", "proc/sys/kernel/hostname"} {
		fsys := debian12()
		delete(fsys, name)
		if _, err := ReadFacts(fsys); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("sans %s : erreur %v", name, err)
		}
	}
}

func TestReadState_FailsOnAMissingFile(t *testing.T) {
	for _, name := range []string{"proc/stat", "proc/uptime", "proc/loadavg"} {
		fsys := debian12()
		delete(fsys, name)
		if _, err := ReadState(fsys); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("sans %s : erreur %v", name, err)
		}
	}
}

func TestParseOSRelease_Rocky10Quoted(t *testing.T) {
	data, err := os.ReadFile("testdata/rocky10/etc/os-release")
	if err != nil {
		t.Fatal(err)
	}
	got := parseOSRelease(string(data))
	want := OS{ID: "rocky", VersionID: "10.2", PrettyName: "Rocky Linux 10.2 (Red Quartz)"}
	if got != want {
		t.Errorf("%+v, %+v attendu", got, want)
	}
}

func TestParsers_RefuseMalformedInput(t *testing.T) {
	if _, err := parseMemTotal("MemFree: 12 kB\n"); err == nil {
		t.Error("meminfo sans MemTotal accepté")
	}
	if _, err := parseBootTime("cpu 1 2 3\n"); err == nil {
		t.Error("stat sans btime accepté")
	}
	if _, err := parseUptime("12.5"); err == nil {
		t.Error("uptime à un champ accepté")
	}
	if _, err := parseLoadAvg("0.1 0.2 x 1/2 3"); err == nil {
		t.Error("loadavg invalide accepté")
	}
}
