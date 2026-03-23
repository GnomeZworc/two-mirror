package netif

import (
	"fmt"
	"runtime"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

func CreateVethToNetns(rootIf, nsIf, netnsPath string, mtu int) error {
	// Obligatoire : netns lié au thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Ouvrir le netns cible
	ns, err := netns.GetFromPath(netnsPath)
	if err != nil {
		return fmt.Errorf("open netns: %w, %s", err, netnsPath)
	}
	defer ns.Close()

	// Créer le veth dans le netns courant
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: rootIf,
			MTU:  mtu,
		},
		PeerName: nsIf,
	}

	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("link add: %w", err)
	}

	// Récupérer l'interface peer
	peer, err := netlink.LinkByName(nsIf)
	if err != nil {
		return fmt.Errorf("peer not found: %w", err)
	}

	// Déplacer le peer dans le netns cible
	if err := netlink.LinkSetNsFd(peer, int(ns)); err != nil {
		return fmt.Errorf("set ns: %w", err)
	}

	return nil
}
