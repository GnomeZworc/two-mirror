package watchdog

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

type gatewayState struct {
	MAC          string
	GatewayAddrs []string
	BridgeAddrs  []string
}

func hasAddr(addrs []string, ip string) bool {
	for _, a := range addrs {
		if a == ip {
			return true
		}
	}
	return false
}

func gatewayProblems(gw, bridge string, got gatewayState, wantMAC, gatewayIP, relayIP string) []string {
	var problems []string
	if got.MAC != wantMAC {
		problems = append(problems, fmt.Sprintf("interface %s has mac %q, expected anycast %s", gw, got.MAC, wantMAC))
	}
	if !hasAddr(got.GatewayAddrs, gatewayIP) {
		problems = append(problems, fmt.Sprintf("interface %s lacks the gateway address %s", gw, gatewayIP))
	}
	if !hasAddr(got.BridgeAddrs, relayIP) {
		problems = append(problems, fmt.Sprintf("interface %s lacks the relay address %s", bridge, relayIP))
	}
	if hasAddr(got.BridgeAddrs, gatewayIP) {
		problems = append(problems, fmt.Sprintf("interface %s carries the gateway address %s, guests would bypass the anycast mac", bridge, gatewayIP))
	}
	return problems
}

func readGatewayState(gw, bridge string) (gatewayState, error) {
	var st gatewayState
	mac, err := netif.LinkMAC(gw)
	if err != nil {
		return st, err
	}
	st.MAC = mac.String()
	for _, side := range []struct {
		iface string
		dst   *[]string
	}{{gw, &st.GatewayAddrs}, {bridge, &st.BridgeAddrs}} {
		ips, err := netif.LinkIPv4(side.iface)
		if err != nil {
			return st, err
		}
		*side.dst = ipStrings(ips)
	}
	return st, nil
}

func ipStrings(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out
}

func checkAnycastGateway(db *badger.DB, name, vpc, bridge string, n notify.Notifier) {
	relayIP, err := kv.GetFromDB(db, prefixSubnet+name+"/relay_ip")
	if err != nil || relayIP == "" {
		return
	}
	gatewayIP, err := kv.GetFromDB(db, prefixSubnet+name+"/interface_ip")
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("interface_ip unreadable in database: %v", err))
		return
	}
	raw, err := kv.GetFromDB(db, prefixSubnet+name+"/vxlan_id")
	if err != nil {
		return
	}
	vni, err := strconv.Atoi(raw)
	if err != nil {
		return
	}
	wantMAC, err := netif.AnycastMAC(vni)
	if err != nil {
		n.Notify(kindSubnet, name, err.Error())
		return
	}
	if !netns.Exist(vpc) {
		return
	}
	gw := "gw-" + strings.TrimPrefix(bridge, "br-")
	if err := netns.Call(vpc, func() error {
		if p := linkProblem(gw); p != "" {
			n.Notify(kindSubnet, name, p+" (in netns "+vpc+")")
			return nil
		}
		st, err := readGatewayState(gw, bridge)
		if err != nil {
			n.Notify(kindSubnet, name, fmt.Sprintf("anycast gateway unreadable in netns %s: %v", vpc, err))
			return nil
		}
		for _, p := range gatewayProblems(gw, bridge, st, wantMAC.String(), gatewayIP, relayIP) {
			n.Notify(kindSubnet, name, p+" (in netns "+vpc+")")
		}
		return nil
	}); err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("cannot enter netns %s: %v", vpc, err))
	}
}
