package l3vni

import (
	"strings"
	"testing"
)

func TestValid_AcceptsEvery24BitValueOutsideTheKernelTables(t *testing.T) {
	for _, id := range []uint32{1, 252, 256, 4001, 16777215} {
		if err := Valid(id); err != nil {
			t.Errorf("Valid(%d) = %v", id, err)
		}
	}
}

func TestValid_RejectsOutOfRangeAndReservedTables(t *testing.T) {
	cases := map[uint32]string{
		0:        "does not fit in 24 bits",
		16777216: "does not fit in 24 bits",
		253:      "is reserved",
		254:      "is reserved",
		255:      "is reserved",
	}
	for id, want := range cases {
		if err := Valid(id); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Valid(%d) = %v, attendu %q", id, err, want)
		}
	}
}
