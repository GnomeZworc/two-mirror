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
