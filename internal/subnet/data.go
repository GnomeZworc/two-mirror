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
	vpc          string
	subnetID     string
	bridge       string
	mode         string
	vxlanID      int
	localIface   string
	interfaceIP  net.IP
	cidr         *net.IPNet
	vpcCIDR      *net.IPNet
	defaultRoute bool
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

	mode, err := kv.GetFromDB(db, "subnet/"+name+"/mode")
	if err != nil {
		return d, fmt.Errorf("get mode: %w", err)
	}
	d.mode = mode

	if d.mode == "vxlan" {
		vxlanIDStr, err := kv.GetFromDB(db, "subnet/"+name+"/vxlan_id")
		if err != nil {
			return d, fmt.Errorf("get vxlan_id: %w", err)
		}
		vxlanID, err := strconv.Atoi(vxlanIDStr)
		if err != nil {
			return d, fmt.Errorf("parse vxlan_id: %w", err)
		}
		d.vxlanID = vxlanID
	}

	localIface, err := kv.GetFromDB(db, "subnet/"+name+"/local_iface")
	if err != nil {
		return d, fmt.Errorf("get local_iface: %w", err)
	}
	d.localIface = localIface

	interfaceIPStr, err := kv.GetFromDB(db, "subnet/"+name+"/interface_ip")
	if err != nil {
		return d, fmt.Errorf("get interface_ip: %w", err)
	}
	interfaceIP := net.ParseIP(interfaceIPStr)
	if interfaceIP == nil {
		return d, fmt.Errorf("invalid interface_ip: %s", interfaceIPStr)
	}
	d.interfaceIP = interfaceIP

	cidrStr, err := kv.GetFromDB(db, "subnet/"+name+"/cidr")
	if err != nil {
		return d, fmt.Errorf("get cidr: %w", err)
	}
	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return d, fmt.Errorf("parse cidr: %w", err)
	}
	d.cidr = ipNet

	defaultRouteStr, err := kv.GetFromDB(db, "subnet/"+name+"/default_route")
	if err != nil {
		return d, fmt.Errorf("get default_route: %w", err)
	}
	d.defaultRoute = defaultRouteStr == "true"

	vpcCIDRStr, err := kv.GetFromDB(db, "vpc/"+d.vpc+"/cidr")
	if err != nil {
		return d, fmt.Errorf("get vpc cidr: %w", err)
	}
	_, vpcIPNet, err := net.ParseCIDR(vpcCIDRStr)
	if err != nil {
		return d, fmt.Errorf("parse vpc cidr: %w", err)
	}
	d.vpcCIDR = vpcIPNet

	return d, nil
}
