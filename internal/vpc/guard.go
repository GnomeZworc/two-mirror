package vpc

import (
	"git.g3e.fr/syonad/two/internal/nft"
	"git.g3e.fr/syonad/two/internal/sysctl"
)

const netnsPublicBridge = "br-public"

var bridgeNetfilterKeys = []string{
	"net.bridge.bridge-nf-call-iptables",
	"net.bridge.bridge-nf-call-ip6tables",
	"net.bridge.bridge-nf-call-arptables",
}

func guardPublicBridge() error {
	for _, key := range bridgeNetfilterKeys {
		if _, err := sysctl.SetIfPresent(key, "0"); err != nil {
			return err
		}
	}
	if err := sysctl.Set("net.ipv4.conf."+netnsPublicBridge+".arp_ignore", "1"); err != nil {
		return err
	}
	return nft.SetBridgeGuard(netnsPublicBridge)
}
