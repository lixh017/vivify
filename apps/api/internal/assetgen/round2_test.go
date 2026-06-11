package assetgen_test

import (
	"testing"

	"github.com/opc/api/internal/assetgen"
)

// TestRound2PublicBasicCases covers the standard rounding path.
func TestRound2PublicBasicCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"0.5 rounds up", 0.5, 0.5},
		{"0.004 rounds down to 0.00", 0.004, 0.0},
		{"0.005 rounds up to 0.01", 0.005, 0.01},
		{"0.1234 rounds to 0.12", 0.1234, 0.12},
		{"0.125 rounds to 0.13 (banker's round is not Go's)", 0.125, 0.13},
		{"-0.1234 rounds to -0.12", -0.1234, -0.12},
		{"1.234 rounds to 1.23", 1.234, 1.23},
		{"already 2-decimal preserved", 0.95, 0.95},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := assetgen.Round2Public(tc.in)
			if got != tc.want {
				t.Errorf("Round2Public(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestRound2PublicEdgeCases covers boundaries (0, very large,
// very small).
func TestRound2PublicEdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"zero stays zero", 0.0, 0.0},
		{"negative zero", -0.0, 0.0},
		{"integer preserved", 1.0, 1.0},
		{"1000.0 preserved", 1000.0, 1000.0},
		{"very large", 1e10, 1e10},
		{"very small rounds to 0", 1e-10, 0.0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := assetgen.Round2Public(tc.in)
			if got != tc.want {
				t.Errorf("Round2Public(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
