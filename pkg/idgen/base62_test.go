package idgen

import (
	"errors"
	"math"
	"testing"
)

func TestEncodeKnownVectors(t *testing.T) {
	cases := []struct {
		n    uint64
		want string
	}{
		{0, "0"},
		{9, "9"},
		{10, "a"},
		{35, "z"},
		{36, "A"},
		{61, "Z"},
		{62, "10"},
		{3843, "ZZ"},
		{3844, "100"},
		{math.MaxUint64, "lYGhA16ahyf"},
	}
	for _, c := range cases {
		if got := Encode(c.n); got != c.want {
			t.Errorf("Encode(%d) = %q, want %q", c.n, got, c.want)
		}
		got, err := Decode(c.want)
		if err != nil || got != c.n {
			t.Errorf("Decode(%q) = %d, %v, want %d", c.want, got, err, c.n)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	// A multiplicative walk covers every magnitude without a random source.
	for n := uint64(1); n < math.MaxUint64/7; n = n*7 + 3 {
		got, err := Decode(Encode(n))
		if err != nil || got != n {
			t.Fatalf("round trip %d: got %d, %v", n, got, err)
		}
	}
}

func TestDecodeRejectsBadInput(t *testing.T) {
	cases := []struct {
		in   string
		want error
	}{
		{"", ErrEmptyCode},
		{"ab-c", ErrInvalidChar},
		{"ab c", ErrInvalidChar},
		{"é", ErrInvalidChar},
		{"lYGhA16ahyg", ErrOverflow},  // MaxUint64 + 1
		{"ZZZZZZZZZZZZ", ErrOverflow}, // 12 digits
	}
	for _, c := range cases {
		if _, err := Decode(c.in); !errors.Is(err, c.want) {
			t.Errorf("Decode(%q) err = %v, want %v", c.in, err, c.want)
		}
	}
}
