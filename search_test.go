package main

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"reflect"
	"testing"
)

type exactTuple struct {
	a, b     int
	reversed bool
	e1, e2   string
}

// The exhaustive oracle intentionally shares neither the production predicate,
// tolerance, distance helper, index key nor KD tree. It compares rational
// squared errors directly, without the production floor-threshold algorithm.
func exhaustiveOracle(model *Model, mode string, value *big.Rat, excludeSplays, includeDuplicates bool) []exactTuple {
	distance := func(a, b Point) *big.Rat {
		total := new(big.Rat)
		for axis := 0; axis < 3; axis++ {
			d := new(big.Rat).Sub(new(big.Rat).SetInt64(a[axis]), new(big.Rat).SetInt64(b[axis]))
			total.Add(total, d.Mul(d, d))
		}
		return total
	}
	eligible := func(leg Leg) bool {
		for _, flag := range leg.Flags {
			if flag == "SPLAY" && excludeSplays || flag == "DUPLICATE" && !includeDuplicates {
				return false
			}
		}
		return true
	}
	var result []exactTuple
	for i, a := range model.Legs {
		if !eligible(a) {
			continue
		}
		for j := i + 1; j < len(model.Legs); j++ {
			b := model.Legs[j]
			if !eligible(b) {
				continue
			}
			all := [4]*big.Rat{distance(a.A, b.A), distance(a.B, b.B), distance(a.A, b.B), distance(a.B, b.A)}
			direct, reverse := all[0], all[2]
			if all[1].Cmp(direct) > 0 {
				direct = all[1]
			}
			if all[3].Cmp(reverse) > 0 {
				reverse = all[3]
			}
			reversed := reverse.Cmp(direct) < 0
			e1, e2, worst := all[0], all[1], direct
			if reversed {
				e1, e2, worst = all[2], all[3], reverse
			}
			threshold := new(big.Rat).Mul(value, value)
			if mode == "relative_percent" {
				shorter, other := distance(a.A, a.B), distance(b.A, b.B)
				if other.Cmp(shorter) < 0 {
					shorter = other
				}
				threshold.Mul(threshold, shorter)
				threshold.Quo(threshold, big.NewRat(10000, 1))
			} else {
				threshold.Mul(threshold, big.NewRat(10000, 1))
			}
			if worst.Cmp(threshold) <= 0 {
				result = append(result, exactTuple{i, j, reversed, e1.Num().String(), e2.Num().String()})
			}
		}
	}
	return result
}

func collectedPairs(t *testing.T, search *Search, batch int) []exactTuple {
	t.Helper()
	var got []exactTuple
	err := search.Pairs(batch, func(p Pair) error {
		got = append(got, exactTuple{p.A, p.B, p.Match.Reversed, p.Match.E1.String(), p.Match.E2.String()})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if search.Matches != len(got) || search.Candidates < search.Matches {
		t.Fatalf("bad counters: C=%d K=%d returned=%d", search.Candidates, search.Matches, len(got))
	}
	return got
}

func TestSearchSelectionIdentityAndUnionCount(t *testing.T) {
	model := &Model{Legs: []Leg{
		{ID: 1, A: Point{}, B: Point{100, 0, 0}},
		{ID: 2, A: Point{}, B: Point{100, 0, 0}, Flags: []string{"SPLAY"}},
		{ID: 3, A: Point{}, B: Point{100, 0, 0}, Flags: []string{"DUPLICATE"}},
		{ID: 4, A: Point{}, B: Point{100, 0, 0}, Flags: []string{"DUPLICATE", "SPLAY", "SURFACE"}},
		{ID: 5, A: Point{}, B: Point{100, 0, 0}, Flags: []string{"SURFACE"}},
	}}
	for _, tc := range []struct {
		excludeSplays, includeDuplicates bool
		excluded                         int
		want                             [][2]int
	}{
		{false, false, 2, [][2]int{{0, 1}, {0, 4}, {1, 4}}},
		{true, false, 3, [][2]int{{0, 4}}},
		{false, true, 0, [][2]int{{0, 1}, {0, 2}, {0, 3}, {0, 4}, {1, 2}, {1, 3}, {1, 4}, {2, 3}, {2, 4}, {3, 4}}},
		{true, true, 2, [][2]int{{0, 2}, {0, 4}, {2, 4}}},
	} {
		search := NewSearch(model, mustTolerance(t, "relative_percent", "5"), tc.excludeSplays, tc.includeDuplicates)
		if search.NInput != 5 || search.NExcluded != tc.excluded || search.N != 5-tc.excluded {
			t.Fatalf("incorrect selection counts: %+v", search)
		}
		var pairs [][2]int
		for _, p := range collectedPairs(t, search, 2) {
			pairs = append(pairs, [2]int{p.a, p.b})
		}
		if !reflect.DeepEqual(pairs, tc.want) {
			t.Fatalf("pairs=%v want %v", pairs, tc.want)
		}
		if search.Candidates != len(tc.want) {
			t.Fatalf("wrong identical-record candidate count: %d", search.Candidates)
		}
	}
}

func TestSearchMatchesIndependentOracleAcrossBatches(t *testing.T) {
	rng := rand.New(rand.NewSource(20261026))
	model := &Model{}
	for i := 0; i < 72; i++ {
		var a, b Point
		for k := range a {
			a[k] = int64(rng.Intn(101) - 50)
			b[k] = a[k] + int64(rng.Intn(61)-30)
		}
		leg := testLeg(a, b)
		switch i % 5 {
		case 1:
			leg.Flags = []string{"SPLAY"}
		case 2:
			leg.Flags = []string{"DUPLICATE"}
		case 3:
			leg.Flags = []string{"DUPLICATE", "SPLAY"}
		}
		model.Legs = append(model.Legs, leg)
		if i%4 == 0 {
			model.Legs = append(model.Legs, testLeg(b, a), leg)
		}
	}
	// These regressions exercise min/max coordinates instead of lexicographic
	// endpoints, L-infinity instead of Euclidean broad phase, and point radii.
	model.Legs = append(model.Legs,
		testLeg(Point{-2, 0, 0}, Point{2, 100, 0}), testLeg(Point{2, 0, 0}, Point{-2, 100, 0}),
		testLeg(Point{}, Point{100, 100, 0}), testLeg(Point{5, 5, 0}, Point{105, 105, 0}),
		testLeg(Point{}, Point{}), testLeg(Point{}, Point{}), testLeg(Point{1, 0, 0}, Point{1, 0, 0}),
		testLeg(Point{}, Point{110, 0, 0}), testLeg(Point{3, 0, 0}, Point{103, 0, 0}),
	)
	for i := range model.Legs {
		model.Legs[i].ID = i + 1
	}
	for _, mode := range []struct {
		mode, value string
		rational    *big.Rat
	}{
		{"absolute_m", ".05", big.NewRat(1, 20)},
		{"absolute_m", ".0099", big.NewRat(99, 10000)},
		{"relative_percent", "5", big.NewRat(5, 1)},
		{"relative_percent", "10", big.NewRat(10, 1)},
		{"relative_percent", "4.9999", big.NewRat(49999, 10000)},
	} {
		for _, excludeSplays := range []bool{false, true} {
			for _, includeDuplicates := range []bool{false, true} {
				name := fmt.Sprintf("%s_%s_splays_%v_duplicates_%v", mode.mode, mode.value, excludeSplays, includeDuplicates)
				t.Run(name, func(t *testing.T) {
					want := exhaustiveOracle(model, mode.mode, mode.rational, excludeSplays, includeDuplicates)
					search := NewSearch(model, mustTolerance(t, mode.mode, mode.value), excludeSplays, includeDuplicates)
					candidates := -1
					for _, batch := range []int{1, 7, 1000} {
						got := collectedPairs(t, search, batch)
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("batch=%d\ngot:  %v\nwant: %v", batch, got, want)
						}
						if candidates >= 0 && search.Candidates != candidates {
							t.Fatal("batch changed candidate count")
						}
						candidates = search.Candidates
					}
				})
			}
		}
	}
}

func TestSearchFullRangeAndCappedRadius(t *testing.T) {
	lo, hi := int64(-2147483648), int64(2147483647)
	model := &Model{Legs: []Leg{
		testLeg(Point{lo, lo, lo}, Point{hi, hi, hi}),
		testLeg(Point{hi, hi, hi}, Point{lo, lo, lo}),
		testLeg(Point{lo, hi, lo}, Point{hi, lo, hi}),
		testLeg(Point{lo, lo, lo}, Point{lo, lo, lo}),
		testLeg(Point{hi, hi, hi}, Point{hi, hi, hi}),
	}}
	for _, tc := range []struct {
		mode, value string
		rational    *big.Rat
	}{
		{"relative_percent", "5", big.NewRat(5, 1)},
		{"relative_percent", "1000", big.NewRat(1000, 1)},
		{"absolute_m", "1e12", big.NewRat(1000000000000, 1)},
	} {
		search := NewSearch(model, mustTolerance(t, tc.mode, tc.value), false, true)
		if got, want := collectedPairs(t, search, 3), exhaustiveOracle(model, tc.mode, tc.rational, false, true); !reflect.DeepEqual(got, want) {
			t.Fatalf("full-range %s %s\ngot: %v\nwant: %v", tc.mode, tc.value, got, want)
		}
	}
}

func TestSearchEmptySingleAndExcludedModels(t *testing.T) {
	for _, model := range []*Model{{}, {Legs: []Leg{testLeg(Point{}, Point{1, 0, 0})}}, {Legs: []Leg{{Flags: []string{"DUPLICATE"}}}}} {
		search := NewSearch(model, mustTolerance(t, "absolute_m", ".05"), false, false)
		if got := collectedPairs(t, search, 1); len(got) != 0 {
			t.Fatalf("unexpected pairs %v", got)
		}
	}
}

func TestSearchStreamingErrorAndCounterReset(t *testing.T) {
	model := &Model{Legs: []Leg{testLeg(Point{}, Point{100, 0, 0}), testLeg(Point{}, Point{100, 0, 0}), testLeg(Point{}, Point{100, 0, 0})}}
	search := NewSearch(model, mustTolerance(t, "relative_percent", "5"), false, false)
	stop := errors.New("consumer stopped")
	if err := search.Pairs(1, func(Pair) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("consumer error was not preserved: %v", err)
	}
	if search.Candidates != 1 || search.Matches != 1 {
		t.Fatalf("search continued after consumer error: C=%d K=%d", search.Candidates, search.Matches)
	}
	if got := collectedPairs(t, search, 1); len(got) != 3 || search.Candidates != 3 {
		t.Fatalf("second iteration did not reset counters: %v", got)
	}
	for _, batch := range []int{0, -1} {
		if err := search.Pairs(batch, func(Pair) error { t.Fatal("invalid batch emitted a pair"); return nil }); err == nil {
			t.Fatal("accepted invalid batch")
		}
	}
}

func TestSearchOutwardRadiusDoesNotRelaxQualification(t *testing.T) {
	for _, tc := range []struct {
		epsilon    string
		separation int64
		matched    bool
	}{
		{".0499999999999999999999", 5, false},
		{".05", 5, true},
		{".0500000000000000000001", 6, false},
		{".0099", 1, false},
		{".01", 1, true},
	} {
		model := &Model{Legs: []Leg{
			testLeg(Point{}, Point{100, 0, 0}),
			testLeg(Point{0, tc.separation, 0}, Point{100, tc.separation, 0}),
		}}
		search := NewSearch(model, mustTolerance(t, "absolute_m", tc.epsilon), false, false)
		got := collectedPairs(t, search, 1)
		if search.Candidates != 1 || (len(got) == 1) != tc.matched {
			t.Fatalf("epsilon %s separation %d: C=%d got %v", tc.epsilon, tc.separation, search.Candidates, got)
		}
	}
}

func TestSearchTranslationAndNonTransitivity(t *testing.T) {
	want := []exactTuple{{0, 1, false, "16", "16"}, {1, 2, false, "16", "16"}}
	for _, origin := range []Point{{}, {-100000000, -100000000, -100000000}, {100000000, 100000000, 100000000}} {
		model := &Model{}
		for _, offset := range []int64{0, 4, 8} {
			a, b := origin, origin
			a[1] += offset
			b[0] += 100
			b[1] += offset
			model.Legs = append(model.Legs, testLeg(a, b))
		}
		for _, tol := range []*Tolerance{mustTolerance(t, "absolute_m", ".05"), mustTolerance(t, "relative_percent", "5")} {
			if got := collectedPairs(t, NewSearch(model, tol, false, false), 2); !reflect.DeepEqual(got, want) {
				t.Fatalf("origin=%v mode=%s: got %v want %v", origin, tol.Mode, got, want)
			}
		}
	}
}

func TestSearchDenseRecordsHaveNoNeighborLimit(t *testing.T) {
	model := &Model{Legs: make([]Leg, 80)}
	for i := range model.Legs {
		model.Legs[i] = testLeg(Point{}, Point{100, 0, 0})
		if i%2 == 1 {
			model.Legs[i].A, model.Legs[i].B = model.Legs[i].B, model.Legs[i].A
		}
	}
	want := exhaustiveOracle(model, "relative_percent", big.NewRat(5, 1), false, false)
	for _, batch := range []int{1, 3, 81} {
		search := NewSearch(model, mustTolerance(t, "relative_percent", "5"), false, false)
		if got := collectedPairs(t, search, batch); len(got) != 3160 || !reflect.DeepEqual(got, want) {
			t.Fatalf("batch %d: dense output differs from 3160 oracle pairs", batch)
		}
	}
}
