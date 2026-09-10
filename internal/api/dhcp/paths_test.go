package dhcpapi

import (
	"os"
	"strings"
	"testing"
)

func TestInstance_JoinsVPCAndBridge(t *testing.T) {
	if got := Instance("vp-admin", "br-000001"); got != "vp-admin_br-000001" {
		t.Errorf("Instance = %s, want vp-admin_br-000001", got)
	}
}

func TestUnit_NamesTheTemplatedService(t *testing.T) {
	if got := Unit(Instance("vp-admin", "br-000001")); got != "dhcp@vp-admin_br-000001.service" {
		t.Errorf("Unit = %s", got)
	}
}

func TestSocketPath_SitsUnderTheRunDir(t *testing.T) {
	got := SocketPath(DefaultRunDir, Instance("vp-admin", "br-000001"))
	if got != "/run/two/dhcp/vp-admin_br-000001.sock" {
		t.Errorf("SocketPath = %s", got)
	}
}

func TestStatePath_SitsUnderTheRunDir(t *testing.T) {
	got := StatePath(DefaultRunDir, Instance("vp-admin", "br-000001"))
	if got != "/run/two/dhcp/vp-admin_br-000001.state" {
		t.Errorf("StatePath = %s", got)
	}
}

func TestPaths_NameTheVPCSoAListingIsReadable(t *testing.T) {
	got := SocketPath(DefaultRunDir, Instance("vp-admin", "br-000001"))
	if !strings.Contains(got, "vp-admin") {
		t.Errorf("path = %s, want the vpc visible when listing the run dir", got)
	}
}

func TestPaths_DistinguishTwoSubnetsOfTheSameVPC(t *testing.T) {
	a := SocketPath(DefaultRunDir, Instance("vp-admin", "br-000001"))
	b := SocketPath(DefaultRunDir, Instance("vp-admin", "br-000002"))
	if a == b {
		t.Error("two subnets must not share a control socket")
	}
}

func TestSocketPath_StaysUnderTheUnixPathLimit(t *testing.T) {
	got := SocketPath(DefaultRunDir, Instance("vp-000000", "br-000000"))
	if len(got) > 100 {
		t.Errorf("socket path is %d bytes (%s): sun_path caps at 104 on darwin and 108 on linux", len(got), got)
	}
}

func TestDefaultRunDir_MatchesTheWrapperScript(t *testing.T) {
	const script = "../../../scripts/run-dhcp-in-netns.sh"

	raw, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read %s: %v", script, err)
	}

	want := `RUN_DIR="` + DefaultRunDir + `"`
	if !strings.Contains(string(raw), want) {
		t.Errorf("%s does not set %s: the agent would talk to a socket the server never creates", script, want)
	}
}
