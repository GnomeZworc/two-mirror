package dhcp

import (
	"strings"
	"testing"
)

func TestGenerateConfig_CarriesTheInterfaceMTU(t *testing.T) {
	c := newConf(t, "192.168.1.0/29")
	c.MTU = 1450
	if content := confLines(t, c); !strings.Contains(content, "dhcp-option=26,1450\n") {
		t.Errorf("option 26 absente :\n%s", content)
	}
}

func TestGenerateConfig_InterfaceMTUIs1500WhenUnset(t *testing.T) {
	if content := confLines(t, newConf(t, "192.168.1.0/29")); !strings.Contains(content, "dhcp-option=26,1500\n") {
		t.Errorf("option 26 absente :\n%s", content)
	}
}
