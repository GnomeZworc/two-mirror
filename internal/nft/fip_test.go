package nft

import (
	"strings"
	"testing"
)

func labFloatingIP() FloatingIP {
	return FloatingIP{Iface: "fip-f000002f", Public: "240.0.0.47", Private: "10.46.1.10"}
}

func TestFloatingIPSetScript(t *testing.T) {
	want := `add table ip two
add chain ip two pre-fip-f000002f { type filter hook prerouting priority raw; policy accept; }
flush chain ip two pre-fip-f000002f
add rule ip two pre-fip-f000002f iifname "fip-f000002f" ip daddr 240.0.0.47 ip daddr set 10.46.1.10
add chain ip two post-fip-f000002f { type filter hook postrouting priority srcnat; policy accept; }
flush chain ip two post-fip-f000002f
add rule ip two post-fip-f000002f oifname "fip-f000002f" ip saddr 10.46.1.10 ip saddr set 240.0.0.47
`
	if got := fipSetScript(labFloatingIP()); got != want {
		t.Errorf("script :\n%s\nattendu :\n%s", got, want)
	}
}

func TestFloatingIPDeleteScript(t *testing.T) {
	want := `add table ip two
add chain ip two pre-fip-f000002f { type filter hook prerouting priority raw; policy accept; }
flush chain ip two pre-fip-f000002f
delete chain ip two pre-fip-f000002f
add chain ip two post-fip-f000002f { type filter hook postrouting priority srcnat; policy accept; }
flush chain ip two post-fip-f000002f
delete chain ip two post-fip-f000002f
`
	if got := fipDeleteScript("fip-f000002f"); got != want {
		t.Errorf("script :\n%s\nattendu :\n%s", got, want)
	}
}

func TestFloatingIPScriptsDoNotTrackNorTouchOtherInterfaces(t *testing.T) {
	script := fipSetScript(labFloatingIP()) + fipDeleteScript("fip-f000002f")
	for _, line := range strings.Split(strings.TrimSpace(script), "\n") {
		if !strings.Contains(line, " ip two") {
			t.Errorf("ligne hors de la table ip two : %q", line)
		}
		if strings.Contains(line, "notrack") {
			t.Errorf("notrack présent : %q", line)
		}
		if strings.HasPrefix(line, "add rule") && !strings.Contains(line, `ifname "fip-f000002f"`) {
			t.Errorf("règle qui ne vise pas fip-f000002f : %q", line)
		}
	}
}
