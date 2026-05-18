package vm

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type vmData struct {
	subnetName   string
	vpcName      string
	gatewayIP    string
	bridge       string
	tapID        int
	ip           string
	metadataPort string
	mac          string
	volumePath   string
	memory       int
	cpus         int
	password     string
	sshkey       string
}

func loadVM(db *badger.DB, name string) (vmData, error) {
	var d vmData

	subnetName, err := kv.GetFromDB(db, "vm/"+name+"/subnet")
	if err != nil {
		return d, fmt.Errorf("get subnet: %w", err)
	}
	d.subnetName = subnetName
	d.bridge = "br-" + strings.SplitN(subnetName, "-", 2)[1]

	vpcName, err := kv.GetFromDB(db, "subnet/"+subnetName+"/vpc")
	if err != nil {
		return d, fmt.Errorf("get vpc: %w", err)
	}
	d.vpcName = vpcName

	gatewayIP, err := kv.GetFromDB(db, "subnet/"+subnetName+"/gateway_ip")
	if err != nil {
		return d, fmt.Errorf("get gateway_ip: %w", err)
	}
	d.gatewayIP = gatewayIP

	tapIDStr, err := kv.GetFromDB(db, "vm/"+name+"/tap_id")
	if err != nil {
		d.tapID = rand.Intn(90000000) + 10000000
		if err := kv.AddInDB(db, "vm/"+name+"/tap_id", strconv.Itoa(d.tapID)); err != nil {
			return d, fmt.Errorf("store tap_id: %w", err)
		}
	} else {
		tapID, err := strconv.Atoi(tapIDStr)
		if err != nil {
			return d, fmt.Errorf("parse tap_id: %w", err)
		}
		d.tapID = tapID
	}

	ip, err := kv.GetFromDB(db, "vm/"+name+"/ip")
	if err != nil {
		return d, fmt.Errorf("get ip: %w", err)
	}
	d.ip = ip

	metadataPort, err := kv.GetFromDB(db, "vm/"+name+"/metadata_port")
	if err != nil {
		return d, fmt.Errorf("get metadata_port: %w", err)
	}
	d.metadataPort = metadataPort

	mac, err := dhcp.GetMACForIP(db, d.subnetName, d.ip)
	if err != nil {
		return d, fmt.Errorf("get mac for ip %s: %w", d.ip, err)
	}
	d.mac = mac

	volumePath, err := kv.GetFromDB(db, "vm/"+name+"/volume_path")
	if err != nil {
		return d, fmt.Errorf("get volume_path: %w", err)
	}
	d.volumePath = volumePath

	memoryStr, err := kv.GetFromDB(db, "vm/"+name+"/memory")
	if err != nil {
		return d, fmt.Errorf("get memory: %w", err)
	}
	d.memory, err = strconv.Atoi(memoryStr)
	if err != nil {
		return d, fmt.Errorf("parse memory: %w", err)
	}

	cpusStr, err := kv.GetFromDB(db, "vm/"+name+"/cpus")
	if err != nil {
		return d, fmt.Errorf("get cpus: %w", err)
	}
	d.cpus, err = strconv.Atoi(cpusStr)
	if err != nil {
		return d, fmt.Errorf("parse cpus: %w", err)
	}

	d.password, _ = kv.GetFromDB(db, "vm/"+name+"/password")
	d.sshkey, _ = kv.GetFromDB(db, "vm/"+name+"/sshkey")

	return d, nil
}
