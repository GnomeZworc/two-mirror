package nft

import "testing"

func TestGuardScript(t *testing.T) {
	want := `add table ip two
add chain ip two in-br-public { type filter hook input priority filter; policy accept; }
flush chain ip two in-br-public
add rule ip two in-br-public iifname "br-public" drop
add chain ip two fwd-br-public { type filter hook forward priority filter; policy accept; }
flush chain ip two fwd-br-public
add rule ip two fwd-br-public iifname "br-public" drop
`
	if got := guardScript("br-public"); got != want {
		t.Errorf("script :\n%s\nattendu :\n%s", got, want)
	}
}
