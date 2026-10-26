package subnet

import (
	"git.g3e.fr/syonad/two/internal/nft"
)

func VxlanFabricPort(vethI, gatewayIP string) nft.Port {
	return nft.Port{
		Name:    vethI,
		Forward: []string{nft.DropARPRequestTo(gatewayIP), nft.DropDHCPFrom(gatewayIP)},
	}
}

func AnycastFabricPort(vethI, mac, gatewayIP, relayIP string) nft.Port {
	return nft.Port{
		Name:       vethI,
		Prerouting: []string{nft.DropSourceMAC(mac)},
		Forward: []string{
			nft.DropSourceMAC(mac),
			nft.DropARPRequestTo(gatewayIP),
			nft.DropARPRequestTo(relayIP),
			nft.DropDHCPFrom(gatewayIP),
			nft.DropDHCPFrom(relayIP),
		},
		Output: []string{nft.DropSourceMAC(mac)},
	}
}

func BridgeFabricPort(vethI, gatewayIP string) nft.Port {
	return nft.Port{
		Name:    vethI,
		Forward: []string{nft.DropARPRequestTo(gatewayIP), nft.DropDHCPFrom(gatewayIP)},
		Output:  []string{nft.DropARPFrom(gatewayIP), nft.DropDHCPFrom(gatewayIP)},
	}
}
