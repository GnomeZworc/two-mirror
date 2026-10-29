package l3vni

import (
	"fmt"
	"net"
	"strings"

	"github.com/dgraph-io/badger/v4"
)

func ExportRoute(db *badger.DB, vpcName string, dst *net.IPNet) error {
	return exportRoute(newSystem(), db, vpcName, dst)
}

func WithdrawRoute(db *badger.DB, vpcName string, dst *net.IPNet) error {
	return withdrawRoute(newSystem(), db, vpcName, dst)
}

func ExportDefault(db *badger.DB, vpcName string) error {
	return exportDefault(newSystem(), db, vpcName)
}

func WithdrawDefault(db *badger.DB, vpcName string) error {
	return withdrawDefault(newSystem(), db, vpcName)
}

func loadRouted(db *badger.DB, vpcName string) (l3VPC, bool, error) {
	parts := strings.SplitN(vpcName, "-", 2)
	if len(parts) != 2 {
		return l3VPC{}, false, fmt.Errorf("invalid vpc name %q", vpcName)
	}
	return loadL3(db, vpcName, parts[1])
}

func exportRoute(h host, db *badger.DB, vpcName string, dst *net.IPNet) error {
	v, ok, err := loadRouted(db, vpcName)
	if err != nil || !ok {
		return err
	}
	if !v.supernet.Contains(dst.IP) {
		return fmt.Errorf("vpc %q: %s is outside %s", vpcName, dst, v.supernet)
	}
	if err := h.TableRouteVia(v.vni, dst, transitNetns.IP, v.vethHost()); err != nil {
		return fmt.Errorf("export %s in %s: %w", dst, v.vrf(), err)
	}
	return nil
}

func withdrawRoute(h host, db *badger.DB, vpcName string, dst *net.IPNet) error {
	v, ok, err := loadRouted(db, vpcName)
	if err != nil || !ok {
		return err
	}
	if err := h.TableRouteDel(v.vni, dst); err != nil {
		return fmt.Errorf("withdraw %s from %s: %w", dst, v.vrf(), err)
	}
	return nil
}

func loadRequired(db *badger.DB, vpcName string) (l3VPC, error) {
	v, ok, err := loadRouted(db, vpcName)
	if err != nil {
		return l3VPC{}, err
	}
	if !ok {
		return l3VPC{}, fmt.Errorf("vpc %q has no l3vni", vpcName)
	}
	return v, nil
}

func exportDefault(h host, db *badger.DB, vpcName string) error {
	v, err := loadRequired(db, vpcName)
	if err != nil {
		return err
	}
	if err := h.TableRouteVia(v.vni, defaultRoute, transitNetns.IP, v.vethHost()); err != nil {
		return fmt.Errorf("export default in %s: %w", v.vrf(), err)
	}
	return nil
}

func withdrawDefault(h host, db *badger.DB, vpcName string) error {
	v, err := loadRequired(db, vpcName)
	if err != nil {
		return err
	}
	if err := h.TableRouteDelVia(v.vni, defaultRoute, transitNetns.IP, v.vethHost()); err != nil {
		return fmt.Errorf("withdraw default from %s: %w", v.vrf(), err)
	}
	return nil
}
