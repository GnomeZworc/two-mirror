package frr

import (
	"fmt"
	"net"
	"strings"
)

type VNIState struct {
	Type      string
	State     string
	RouterMAC string
}

func (c *Client) NeighborEstablished(peer net.IP) (bool, error) {
	out, err := c.exec("show bgp neighbors " + peer.String())
	if err != nil {
		return false, err
	}
	return strings.Contains(out, "BGP state = Established"), nil
}

func (c *Client) VNI(vni uint32) (VNIState, error) {
	out, err := c.exec(fmt.Sprintf("show evpn vni %d", vni))
	if err != nil {
		return VNIState{}, err
	}
	return parseVNI(out), nil
}

func parseVNI(out string) VNIState {
	var s VNIState
	for _, l := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(l), ": ")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(key) {
		case "type":
			s.Type = value
		case "state":
			s.State = value
		case "router mac":
			s.RouterMAC = strings.ToLower(value)
		}
	}
	return s
}
