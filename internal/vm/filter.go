package vm

import (
	"fmt"
	"strings"

	"git.g3e.fr/syonad/two/internal/nft"
)

func TapPort(tapID int, mac, ip string, dhcpServer bool) nft.Port {
	mac = strings.ToLower(mac)
	rules := []string{
		nft.DropSourceMACOtherThan(mac),
		nft.DropEtherTypesOtherThanIPAndARP(),
		nft.DropARPSenderMACOtherThan(mac),
		nft.DropARPSenderIPOtherThan(ip),
		nft.DropIPSourceOtherThan(ip),
	}
	rules = append(rules, nft.DropUnspecifiedSourceOutsideDHCP()...)
	if !dhcpServer {
		rules = append(rules, nft.DropDHCPServer())
	}
	return nft.Port{Name: fmt.Sprintf("tap%d", tapID), Prerouting: rules}
}

func (n nicData) tapPort() nft.Port {
	return TapPort(n.tapID, n.mac, n.ip, n.dhcpServer)
}
