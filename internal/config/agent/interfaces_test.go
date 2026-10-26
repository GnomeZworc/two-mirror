package configuration

import "testing"

func TestPublicBridge(t *testing.T) {
	cases := map[string]struct {
		cfg  Config
		want string
	}{
		"clé absente": {Config{DefaultInterface: "br-000000"}, "br-public"},
		"clé vide":    {Config{DefaultInterface: "br-000000", Interfaces: map[string]string{"internet": ""}}, "br-public"},
		"clé posée":   {Config{Interfaces: map[string]string{"internet": "br-wan"}}, "br-wan"},
		"autres types, sans default_interface en repli": {Config{DefaultInterface: "br-000000", Interfaces: map[string]string{"lan": "br-lan"}}, "br-public"},
	}
	for name, c := range cases {
		if got := c.cfg.PublicBridge(); got != c.want {
			t.Errorf("%s : %q, %q attendu", name, got, c.want)
		}
	}
}
