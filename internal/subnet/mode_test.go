package subnet

import "testing"

func TestBridged(t *testing.T) {
	for mode, want := range map[string]bool{"bridge": true, "public_ip": true, "vxlan": false, "": false} {
		if got := Bridged(mode); got != want {
			t.Errorf("%q : %v, %v attendu", mode, got, want)
		}
	}
}
