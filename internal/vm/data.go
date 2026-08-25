package vm

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type diskEntry struct {
	path string
	dev  string
}

type nicData struct {
	index       int
	subnetName  string
	vpcName     string
	bridge      string
	interfaceIP string
	ip          string
	mac         string
	tapID       int
	primary     bool
}

type vmData struct {
	nics         []nicData
	metadataPort string
	disks        []diskEntry
	memory       int
	cpus         int
	uefi         bool
	password     string
	sshkey       string
	documents    map[string]string
}

// primary retourne l'interface portant la route par défaut et le serveur de
// métadonnées. loadVM garantit qu'il y en a exactement une.
func (d vmData) primary() nicData {
	for _, n := range d.nics {
		if n.primary {
			return n
		}
	}
	return nicData{}
}

func loadVM(db *badger.DB, name string) (vmData, error) {
	var d vmData

	nics, err := loadNICs(db, name)
	if err != nil {
		return d, err
	}
	d.nics = nics

	metadataPort, err := kv.GetFromDB(db, "vm/"+name+"/metadata_port")
	if err != nil {
		return d, fmt.Errorf("get metadata_port: %w", err)
	}
	d.metadataPort = metadataPort

	diskEntries, err := kv.ListByPrefix(db, "vm/"+name+"/disk/")
	if err != nil {
		return d, fmt.Errorf("list disks: %w", err)
	}
	if len(diskEntries) == 0 {
		return d, fmt.Errorf("no disks found for vm %q", name)
	}
	diskPrefix := "vm/" + name + "/disk/"
	for key, path := range diskEntries {
		dev := strings.TrimPrefix(key, diskPrefix)
		d.disks = append(d.disks, diskEntry{path: path, dev: dev})
	}

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

	if v, _ := kv.GetFromDB(db, "vm/"+name+"/uefi"); v == "true" {
		d.uefi = true
	}
	d.password, _ = kv.GetFromDB(db, "vm/"+name+"/password")
	d.sshkey, _ = kv.GetFromDB(db, "vm/"+name+"/sshkey")

	docPrefix := "vm/" + name + "/metadata/"
	docEntries, err := kv.ListByPrefix(db, docPrefix)
	if err != nil {
		return d, fmt.Errorf("list metadata documents: %w", err)
	}
	if len(docEntries) > 0 {
		d.documents = make(map[string]string, len(docEntries))
		for key, content := range docEntries {
			d.documents[strings.TrimPrefix(key, docPrefix)] = content
		}
	}

	return d, nil
}

// loadNICs lit les interfaces d'une VM sous vm/<name>/nic/<index>/.
// Le tap est alloué à la première lecture et persisté, comme avant le passage
// au multi-interfaces — mais désormais par interface.
func loadNICs(db *badger.DB, name string) ([]nicData, error) {
	prefix := "vm/" + name + "/nic/"
	entries, err := kv.ListByPrefix(db, prefix)
	if err != nil {
		return nil, fmt.Errorf("list nics: %w", err)
	}

	indexes := make(map[int]bool)
	for key := range entries {
		parts := strings.Split(strings.TrimPrefix(key, prefix), "/")
		if len(parts) != 2 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid nic index %q for vm %s", parts[0], name)
		}
		indexes[idx] = true
	}
	if len(indexes) == 0 {
		return nil, fmt.Errorf("no interface found for vm %q", name)
	}

	nics := make([]nicData, 0, len(indexes))
	for idx := range indexes {
		n, err := loadNIC(db, name, idx, entries)
		if err != nil {
			return nil, err
		}
		nics = append(nics, n)
	}
	sort.Slice(nics, func(i, j int) bool { return nics[i].index < nics[j].index })

	primaries := 0
	for _, n := range nics {
		if n.primary {
			primaries++
		}
	}
	if primaries != 1 {
		return nil, fmt.Errorf("vm %q has %d primary interfaces, expected exactly one", name, primaries)
	}
	return nics, nil
}

func loadNIC(db *badger.DB, name string, idx int, entries map[string]string) (nicData, error) {
	n := nicData{index: idx}
	prefix := fmt.Sprintf("vm/%s/nic/%d/", name, idx)

	n.subnetName = entries[prefix+"subnet"]
	if n.subnetName == "" {
		return n, fmt.Errorf("nic %d of vm %s has no subnet", idx, name)
	}
	n.bridge = "br-" + strings.SplitN(n.subnetName, "-", 2)[1]
	n.primary = entries[prefix+"primary"] == "true"

	vpcName, err := kv.GetFromDB(db, "subnet/"+n.subnetName+"/vpc")
	if err != nil {
		return n, fmt.Errorf("get vpc of subnet %s: %w", n.subnetName, err)
	}
	n.vpcName = vpcName

	interfaceIP, err := kv.GetFromDB(db, "subnet/"+n.subnetName+"/interface_ip")
	if err != nil {
		return n, fmt.Errorf("get interface_ip of subnet %s: %w", n.subnetName, err)
	}
	n.interfaceIP = interfaceIP

	n.ip = entries[prefix+"ip"]
	if n.ip == "" {
		return n, fmt.Errorf("nic %d of vm %s has no ip", idx, name)
	}

	mac, err := dhcp.GetMACForIP(db, n.subnetName, n.ip)
	if err != nil {
		return n, fmt.Errorf("get mac for ip %s: %w", n.ip, err)
	}
	n.mac = mac

	if tapIDStr, ok := entries[prefix+"tap_id"]; ok {
		tapID, err := strconv.Atoi(tapIDStr)
		if err != nil {
			return n, fmt.Errorf("parse tap_id of nic %d: %w", idx, err)
		}
		n.tapID = tapID
		return n, nil
	}

	n.tapID = rand.Intn(90000000) + 10000000
	if err := kv.AddInDB(db, prefix+"tap_id", strconv.Itoa(n.tapID)); err != nil {
		return n, fmt.Errorf("store tap_id of nic %d: %w", idx, err)
	}
	return n, nil
}
