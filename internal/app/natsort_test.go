package app

import (
	"sort"
	"testing"
)

func TestNatLessSortsNumericSegmentsNumerically(t *testing.T) {
	in := []string{
		"values/staging10/api.yaml",
		"values/staging2/api.yaml",
		"values/staging1/api.yaml",
		"values/staging9/api.yaml",
		"values/dev2/api.yaml",
		"values/dev1/api.yaml",
	}
	want := []string{
		"values/dev1/api.yaml",
		"values/dev2/api.yaml",
		"values/staging1/api.yaml",
		"values/staging2/api.yaml",
		"values/staging9/api.yaml",
		"values/staging10/api.yaml",
	}
	got := append([]string(nil), in...)
	sort.Slice(got, func(i, j int) bool { return natLess(got[i], got[j]) })
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("natural sort mismatch:\ngot  %v\nwant %v", got, want)
			break
		}
	}
}

func TestNatLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"staging1", "staging2", true},
		{"staging2", "staging10", true},
		{"staging10", "staging9", false},
		{"staging9", "staging10", true},
		{"staging10", "staging10", false}, // equal is not less
		{"a", "a1", true},                 // prefix sorts before longer
		{"a1", "a", false},
		{"foo.2", "foo.10", true},  // array indices in a dotted path
		{"foo.10", "foo.2", false}, //
		{"item", "item0", true},    // digit run vs none at the same spot
		{"v1.2", "v1.10", true},    // multiple numeric segments
		{"v1.10", "v1.9", false},   //
		{"1", "01", true},          // equal value: fewer leading zeros first
		{"01", "1", false},         //
		{"007", "7", false},        // more leading zeros sorts after
		{"a2b", "a10b", true},      // numeric run bounded by trailing text
		{"abc", "abd", true},       // pure lexical fallback
	}
	for _, tt := range tests {
		if got := natLess(tt.a, tt.b); got != tt.want {
			t.Errorf("natLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
