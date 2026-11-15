package idgen_test

import (
	"math"
	"testing"

	"github.com/duykhanh/worklane/pkg/idgen"
)

func TestBase62Vectors(t *testing.T) {
	for _, tc := range []struct {
		n    uint64
		code string
	}{
		{0, "0"}, {9, "9"}, {10, "a"}, {35, "z"}, {36, "A"}, {61, "Z"},
		{62, "10"}, {3843, "ZZ"}, {3844, "100"},
		{math.MaxInt64, "aZl8N0y58M7"}, {math.MaxUint64, "lYGhA16ahyf"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			if got := idgen.Encode(tc.n); got != tc.code {
				t.Fatalf("Encode(%d) = %q, want %q", tc.n, got, tc.code)
			}
			got, err := idgen.Decode(tc.code)
			if err != nil || got != tc.n {
				t.Fatalf("Decode(%q) = %d, %v, want %d", tc.code, got, err, tc.n)
			}
		})
	}
}

func TestBase62RejectsInvalidCodes(t *testing.T) {
	for _, code := range []string{"", "-1", "+1", "a/b", "a b", "é", "00", "01", "lYGhA16ahyg", "ZZZZZZZZZZZZ"} {
		t.Run(code, func(t *testing.T) {
			if _, err := idgen.Decode(code); err == nil {
				t.Fatalf("Decode(%q) accepted an invalid or overflowing code", code)
			}
		})
	}
}

func FuzzBase62RoundTrip(f *testing.F) {
	for _, n := range []uint64{0, 10, 36, 62, math.MaxInt64, math.MaxUint64} {
		f.Add(n)
	}
	f.Fuzz(func(t *testing.T, n uint64) {
		got, err := idgen.Decode(idgen.Encode(n))
		if err != nil || got != n {
			t.Fatalf("round trip %d = %d, %v", n, got, err)
		}
	})
}
