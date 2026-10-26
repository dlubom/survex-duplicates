package main

import (
	"math/big"
	"testing"
)

func mustTolerance(t *testing.T, mode, value string) *Tolerance {
	t.Helper()
	tol, err := NewTolerance(mode, value)
	if err != nil {
		t.Fatal(err)
	}
	return tol
}

func testLeg(a, b Point) Leg { return Leg{A: a, B: b} }

func TestToleranceExactDecimalSyntax(t *testing.T) {
	for _, value := range []string{".05", "0.05", "5e-2", "+5.00E-2", "000.0500"} {
		tol := mustTolerance(t, "absolute_m", value)
		if tol.ScaleSq.Cmp(big.NewRat(25, 1)) != 0 {
			t.Errorf("%q: squared scale %v, want 25", value, tol.ScaleSq)
		}
	}
	if got := mustTolerance(t, "relative_percent", "0008").ScaleSq; got.Cmp(big.NewRat(4, 625)) != 0 {
		t.Fatalf("leading zero must remain decimal: %v", got)
	}
	for _, value := range []string{"", "0", "-1", "NaN", "Infinity", "1/2", "0x10", "1_0", " 5", "5 ", "1e", "."} {
		if _, err := NewTolerance("relative_percent", value); err == nil {
			t.Errorf("accepted invalid decimal %q", value)
		}
	}
	if _, err := NewTolerance("unknown", "5"); err == nil {
		t.Fatal("accepted unknown mode")
	}
}

func TestAbsoluteGeometryAcceptance(t *testing.T) {
	base := testLeg(Point{0, 0, 0}, Point{100, 0, 0})
	cases := []struct {
		name         string
		a, b         Leg
		ok, reversed bool
		e1, e2       int64
	}{
		{"identical", base, base, true, false, 0, 0},
		{"reversed", base, testLeg(base.B, base.A), true, true, 0, 0},
		{"inclusive 3-4-5 boundary", base, testLeg(Point{3, 4, 0}, Point{103, 4, 0}), true, false, 25, 25},
		{"Euclidean rejection", base, testLeg(Point{5, 1, 0}, base.B), false, false, 0, 0},
		{"same box is insufficient", testLeg(Point{}, Point{100, 100, 0}), testLeg(Point{0, 100, 0}, Point{100, 0, 0}), false, false, 0, 0},
		{"Z boundary", base, testLeg(Point{0, 0, 5}, Point{100, 0, 5}), true, false, 25, 25},
		{"better reversed", testLeg(Point{}, Point{2, 0, 0}), testLeg(Point{2, 1, 0}, Point{0, 1, 0}), true, true, 1, 1},
		{"orientation tie stays direct", testLeg(Point{}, Point{1, 0, 0}), testLeg(Point{5, 0, 0}, Point{3, 4, 0}), true, false, 25, 20},
		{"point legs", testLeg(Point{}, Point{}), testLeg(Point{3, 4, 0}, Point{3, 4, 0}), true, false, 25, 25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := MatchLegs(tc.a, tc.b, mustTolerance(t, "absolute_m", ".05"))
			if (m != nil) != tc.ok {
				t.Fatalf("match = %v, want accepted=%v", m, tc.ok)
			}
			if m != nil && (m.Reversed != tc.reversed || m.E1.Cmp(big.NewInt(tc.e1)) != 0 || m.E2.Cmp(big.NewInt(tc.e2)) != 0) {
				t.Fatalf("match = %+v, want reversed=%v, errors=%d/%d", m, tc.reversed, tc.e1, tc.e2)
			}
		})
	}
}

func TestRelativeShorterLengthAndExactBoundary(t *testing.T) {
	a := testLeg(Point{}, Point{100, 100, 0})
	b := testLeg(Point{5, 5, 0}, Point{105, 105, 0})
	if m := MatchLegs(a, b, mustTolerance(t, "relative_percent", "5")); m == nil || m.E1.Int64() != 50 || m.E2.Int64() != 50 {
		t.Fatalf("irrational length boundary should match: %v", m)
	}
	if MatchLegs(a, b, mustTolerance(t, "relative_percent", "4.9999")) != nil {
		t.Fatal("rounded a decimal threshold upwards")
	}
	a, b = testLeg(Point{}, Point{100, 0, 0}), testLeg(Point{-2, 3, 0}, Point{110, 3, 0})
	tol := mustTolerance(t, "relative_percent", "10")
	if MatchLegs(a, b, tol) != nil || MatchLegs(b, a, tol) != nil {
		t.Fatal("qualification must use the shorter leg in both orders")
	}
	point := testLeg(Point{}, Point{})
	shift := testLeg(Point{1, 0, 0}, Point{1, 0, 0})
	if MatchLegs(point, point, tol) == nil || MatchLegs(point, shift, tol) != nil || MatchLegs(point, a, tol) != nil {
		t.Fatal("relative point legs must coincide exactly")
	}
}

func TestFullInt32DistanceAndMatching(t *testing.T) {
	lo, hi := int64(-2147483648), int64(2147483647)
	a, b := Point{lo, lo, lo}, Point{hi, hi, hi}
	if got := SquaredDistance(a, b).String(); got != "55340232195358851075" {
		t.Fatalf("full-range squared distance overflow: %s", got)
	}
	one := testLeg(Point{lo, lo, 0}, Point{hi, hi, 0})
	two := testLeg(Point{lo, hi, 0}, Point{hi, lo, 0})
	if MatchLegs(one, two, mustTolerance(t, "relative_percent", "5")) != nil {
		t.Fatal("extreme diagonals must not match at 5%")
	}
	if m := MatchLegs(one, testLeg(one.B, one.A), mustTolerance(t, "relative_percent", "5")); m == nil || !m.Reversed {
		t.Fatal("extreme reversed identical legs must match")
	}
}

func TestQueryRadiusExactOutwardRoundingAndCap(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
	}{
		{".0099", 1}, {".01", 1}, {".010001", 2},
		{"42949672.95", 4294967295}, {"42949672.95001", 4294967296},
		{"42949672.96", 4294967296}, {"1e1000", 4294967296},
	} {
		if got := QueryRadius(big.NewInt(1), mustTolerance(t, "absolute_m", tc.value)); got != tc.want {
			t.Errorf("epsilon %s: radius %d, want %d", tc.value, got, tc.want)
		}
	}
	tol := mustTolerance(t, "relative_percent", "5")
	for _, tc := range []struct{ sq, want int64 }{{0, 0}, {399, 1}, {400, 1}, {401, 2}, {20000, 8}} {
		if got := QueryRadius(big.NewInt(tc.sq), tol); got != tc.want {
			t.Errorf("length squared %d: radius %d, want %d", tc.sq, got, tc.want)
		}
	}
}

func TestToleranceBeyondStandardLibraryRationalExponentLimit(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
	}{{"1e1000001", 1 << 32}, {"1e-1000001", 1}} {
		tol := mustTolerance(t, "absolute_m", tc.value)
		if got := QueryRadius(big.NewInt(1), tol); got != tc.want {
			t.Fatalf("%s: got radius %d, want %d", tc.value, got, tc.want)
		}
	}
}
