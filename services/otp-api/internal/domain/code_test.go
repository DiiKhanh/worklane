package domain

import (
	"strconv"
	"testing"
)

func TestGenerateCode_LengthAndNumeric(t *testing.T) {
	for _, n := range []int{4, 6, 8} {
		code, err := GenerateCode(n)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(code) != n {
			t.Fatalf("want length %d, got %d (%q)", n, len(code), code)
		}
		if _, err := strconv.Atoi(code); err != nil {
			t.Fatalf("code not numeric: %q", code)
		}
	}
}

func TestGenerateCode_Distribution(t *testing.T) {
	seen := map[string]int{}
	for i := 0; i < 1000; i++ {
		c, _ := GenerateCode(6)
		seen[c]++
	}
	// 1000 draws from a 1e6 space: collisions must be rare if the source is uniform.
	if len(seen) < 990 {
		t.Fatalf("suspicious distribution, unique=%d", len(seen))
	}
}

func TestGenerateCode_RejectsOutOfRangeLength(t *testing.T) {
	for _, n := range []int{0, 3, 11} {
		if _, err := GenerateCode(n); err == nil {
			t.Fatalf("length %d should be rejected", n)
		}
	}
}
