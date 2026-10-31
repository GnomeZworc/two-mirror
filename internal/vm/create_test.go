package vm

import (
	"reflect"
	"testing"
)

func TestNicMACs_KeepsTheOrderAndLowersTheCase(t *testing.T) {
	nics := []nicData{
		{index: 0, mac: "52:54:00:49:01:0C"},
		{index: 1, mac: "52:54:00:49:02:16", primary: true},
	}

	got := nicMACs(nics)

	want := []string{"52:54:00:49:01:0c", "52:54:00:49:02:16"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nicMACs = %v, attendu %v", got, want)
	}
}

func TestNicIPs_EveryInterfaceGetsTheRedirect(t *testing.T) {
	nics := []nicData{
		{index: 0, ip: "10.49.1.10"},
		{index: 1, ip: "10.49.2.10", primary: true},
	}

	got := nicIPs(nics)

	want := []string{"10.49.1.10", "10.49.2.10"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nicIPs = %v, attendu %v", got, want)
	}
}
