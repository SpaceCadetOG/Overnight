package lighter

import "testing"

func TestOfficialGroupingModesAndBounds(t *testing.T) {
	for _, mode := range []GroupingMode{GroupingOTO, GroupingOCO, GroupingOTOCO} {
		if err := validateGroupingMode(mode, 2); err != nil {
			t.Fatalf("mode %d rejected: %v", mode, err)
		}
	}
	if err := validateGroupingMode(99, 2); err == nil {
		t.Fatal("unsupported grouping mode accepted")
	}
	if err := validateGroupingMode(GroupingOCO, 1); err == nil {
		t.Fatal("undersized group accepted")
	}
	if err := validateGroupingMode(GroupingOTOCO, 4); err == nil {
		t.Fatal("oversized group accepted")
	}
}
