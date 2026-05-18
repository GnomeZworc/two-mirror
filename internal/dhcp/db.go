package dhcp

import (
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

func StoreDHCPEntries(db *badger.DB, subnetName string, entries map[string]string) error {
	for ip, mac := range entries {
		if err := kv.AddInDB(db, "subnet/"+subnetName+"/dhcp/"+ip, mac); err != nil {
			return err
		}
	}
	return nil
}

func GetMACForIP(db *badger.DB, subnetName, ip string) (string, error) {
	return kv.GetFromDB(db, "subnet/"+subnetName+"/dhcp/"+ip)
}
