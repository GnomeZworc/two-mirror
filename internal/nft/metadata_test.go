package nft

import (
	"strings"
	"testing"
)

func TestMetadataSetScript(t *testing.T) {
	want := `add table ip two
add chain ip two md-t90-m1 { type nat hook prerouting priority dstnat; policy accept; }
flush chain ip two md-t90-m1
add rule ip two md-t90-m1 ip saddr 10.90.1.10 ip daddr 169.254.169.254 tcp dport 80 dnat to 10.90.1.1:8001
add rule ip two md-t90-m1 ip saddr 10.90.2.10 ip daddr 169.254.169.254 tcp dport 80 dnat to 10.90.1.1:8001
`
	if got := metadataSetScript("t90-m1", []string{"10.90.1.10", "10.90.2.10"}, "10.90.1.1:8001"); got != want {
		t.Errorf("script :\n%s\nattendu :\n%s", got, want)
	}
}

func TestMetadataDeleteScript(t *testing.T) {
	want := `add table ip two
add chain ip two md-t90-m1 { type nat hook prerouting priority dstnat; policy accept; }
flush chain ip two md-t90-m1
delete chain ip two md-t90-m1
`
	if got := metadataDeleteScript("t90-m1"); got != want {
		t.Errorf("script :\n%s\nattendu :\n%s", got, want)
	}
}

func TestMetadataScriptsStayInTheirChain(t *testing.T) {
	script := metadataSetScript("t90-m1", []string{"10.90.1.10"}, "10.90.1.1:8001") + metadataDeleteScript("t90-m1")
	for _, line := range strings.Split(strings.TrimSpace(script), "\n") {
		if line != "add table ip two" && !strings.Contains(line, " ip two md-t90-m1") {
			t.Errorf("ligne hors de la chaîne md-t90-m1 : %q", line)
		}
	}
}
