package nft

import (
	"strings"
	"testing"
)

func TestNATSetScript(t *testing.T) {
	want := `add table ip two
add chain ip two snat-ng-42 { type nat hook postrouting priority srcnat; policy accept; }
flush chain ip two snat-ng-42
add rule ip two snat-ng-42 oifname "ng-42" snat to 198.51.100.50
add chain ip two fwd-ng-42 { type filter hook forward priority filter; policy accept; }
flush chain ip two fwd-ng-42
add rule ip two fwd-ng-42 iifname "ng-42" ct state established,related accept
add rule ip two fwd-ng-42 iifname "ng-42" drop
add chain ip two in-ng-42 { type filter hook input priority filter; policy accept; }
flush chain ip two in-ng-42
add rule ip two in-ng-42 iifname "ng-42" ct state established,related accept
add rule ip two in-ng-42 iifname "ng-42" drop
`
	if got := natSetScript(NATGateway{Iface: "ng-42", IP: "198.51.100.50"}); got != want {
		t.Errorf("script :\n%s\nattendu :\n%s", got, want)
	}
}

func TestNATDeleteScript(t *testing.T) {
	want := `add table ip two
add chain ip two snat-ng-42 { type nat hook postrouting priority srcnat; policy accept; }
flush chain ip two snat-ng-42
delete chain ip two snat-ng-42
add chain ip two fwd-ng-42 { type filter hook forward priority filter; policy accept; }
flush chain ip two fwd-ng-42
delete chain ip two fwd-ng-42
add chain ip two in-ng-42 { type filter hook input priority filter; policy accept; }
flush chain ip two in-ng-42
delete chain ip two in-ng-42
`
	if got := natDeleteScript("ng-42"); got != want {
		t.Errorf("script :\n%s\nattendu :\n%s", got, want)
	}
}

func TestNATScriptsLeaveTheBridgeTableAlone(t *testing.T) {
	for label, script := range map[string]string{
		"set":    natSetScript(NATGateway{Iface: "ng-42", IP: "198.51.100.50"}),
		"delete": natDeleteScript("ng-42"),
	} {
		for _, line := range strings.Split(strings.TrimSpace(script), "\n") {
			if !strings.Contains(line, " ip two") {
				t.Errorf("%s : ligne hors de la table ip two : %q", label, line)
			}
		}
	}
}

func TestNATScriptsOnlyTouchTheirInterface(t *testing.T) {
	script := natSetScript(NATGateway{Iface: "ng-42", IP: "198.51.100.50"}) + natDeleteScript("ng-42")
	for _, line := range strings.Split(strings.TrimSpace(script), "\n") {
		if strings.HasPrefix(line, "add rule") && !strings.Contains(line, `ifname "ng-42"`) {
			t.Errorf("règle qui ne vise pas ng-42 : %q", line)
		}
	}
}
