package subnet

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type subnetData struct {
	vpc        string
	subnetID   string
	bridge     string
	vxlanID    int
	localIface string
	gatewayIP  net.IP
	cidr       *net.IPNet
}

func loadSubnet(db *badger.DB, name string) (subnetData, error) {
	var d subnetData

	d.subnetID = strings.SplitN(name, "-", 2)[1]
	d.bridge = "br-" + d.subnetID

	vpc, err := kv.GetFromDB(db, "subnet/"+name+"/vpc")
	if err != nil {
		return d, fmt.Errorf("get vpc: %w", err)
	}
	d.vpc = vpc

	vxlanIDStr, err := kv.GetFromDB(db, "subnet/"+name+"/vxlan_id")
	if err != nil {
		return d, fmt.Errorf("get vxlan_id: %w", err)
	}
	vxlanID, err := strconv.Atoi(vxlanIDStr)
	if err != nil {
		return d, fmt.Errorf("parse vxlan_id: %w", err)
	}
	d.vxlanID = vxlanID

	localIface, err := kv.GetFromDB(db, "subnet/"+name+"/local_iface")
	if err != nil {
		return d, fmt.Errorf("get local_iface: %w", err)
	}
	d.localIface = localIface

	gatewayIPStr, err := kv.GetFromDB(db, "subnet/"+name+"/gateway_ip")
	if err != nil {
		return d, fmt.Errorf("get gateway_ip: %w", err)
	}
	gatewayIP := net.ParseIP(gatewayIPStr)
	if gatewayIP == nil {
		return d, fmt.Errorf("invalid gateway_ip: %s", gatewayIPStr)
	}
	d.gatewayIP = gatewayIP

	cidrStr, err := kv.GetFromDB(db, "subnet/"+name+"/cidr")
	if err != nil {
		return d, fmt.Errorf("get cidr: %w", err)
	}
	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return d, fmt.Errorf("parse cidr: %w", err)
	}
	d.cidr = ipNet

	return d, nil
}
