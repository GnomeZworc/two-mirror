package watchdog

import (
	"fmt"

	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/nft"
	"git.g3e.fr/syonad/two/internal/subnet"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func checkBridgeGateway(db *badger.DB, name, vpc, nsVeth string, n notify.Notifier) {
	gatewayIP, err := kv.GetFromDB(db, prefixSubnet+name+"/interface_ip")
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("interface_ip unreadable in database: %v", err))
		return
	}
	if !netns.Exist(vpc) {
		return
	}
	if err := netns.Call(vpc, func() error {
		missing, err := nft.MissingRules(subnet.BridgeFabricPort(nsVeth, gatewayIP))
		if err != nil {
			n.Notify(kindSubnet, name, fmt.Sprintf("gateway nftables rules unreadable in netns %s: %v", vpc, err))
			return nil
		}
		for _, rule := range missing {
			n.Notify(kindSubnet, name, fmt.Sprintf("nftables rule missing in netns %s: %s", vpc, rule))
		}
		return nil
	}); err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("cannot enter netns %s: %v", vpc, err))
	}
}
