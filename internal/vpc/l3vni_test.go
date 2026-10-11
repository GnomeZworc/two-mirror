package vpc

import (
	"strings"
	"testing"
)

func TestValidL3VNI_AcceptsEvery24BitValueOutsideTheKernelTables(t *testing.T) {
	for _, id := range []uint32{1, 252, 256, 4001, 16777215} {
		if err := ValidL3VNI(id); err != nil {
			t.Errorf("ValidL3VNI(%d) = %v", id, err)
		}
	}
}

func TestValidL3VNI_RejectsOutOfRangeAndReservedTables(t *testing.T) {
	cases := map[uint32]string{
		0:        "does not fit in 24 bits",
		16777216: "does not fit in 24 bits",
		253:      "is reserved",
		254:      "is reserved",
		255:      "is reserved",
	}
	for id, want := range cases {
		if err := ValidL3VNI(id); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ValidL3VNI(%d) = %v, attendu %q", id, err, want)
		}
	}
}
