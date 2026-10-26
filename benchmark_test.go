package main

import (
	"bytes"
	"fmt"
	"math/big"
	"path/filepath"
	"reflect"
	"testing"
)

var qualityBenchmarkModel *Model
var qualityBenchmarkSearch *Search

func BenchmarkParseDump(b *testing.B) {
	data := qualityVerifiedBytes(b, filepath.Join("examples", "kataster", "KATASTER.legs.dump"),
		"845c9bc863f40dd2375c1004c4d02b0293b15134abab7006de60c72addf6f298")
	model, err := ParseDump(bytes.NewReader(data))
	if err != nil || len(model.Legs) != 18584 {
		b.Fatalf("verified bundled parser fixture: legs=%v error=%v", model, err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		qualityBenchmarkModel, err = ParseDump(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Generator v1 is deterministic, with no PRNG: origin = group*10000 cm,
// length = 100+(group%5)*25 cm, alternating endpoint direction, and flags
// cycling through plain/SPLAY/DUPLICATE/both. Distinct groups cannot match
// at either benchmark tolerance. Each repeated group has four records.
func qualityBenchmarkDataset(n, repeats int, dense bool) *Model {
	model := &Model{Legs: make([]Leg, n)}
	for i := range model.Legs {
		group := i / repeats
		if dense {
			group = 0
		}
		a := Point{int64(group) * 10000, int64(group%7) * 100, -int64(group%11) * 100}
		c := a
		c[0] += 100 + int64(group%5)*25
		leg := Leg{ID: i + 1, A: a, B: c}
		if i%2 != 0 {
			leg.A, leg.B = leg.B, leg.A
		}
		switch i % 4 {
		case 1:
			leg.Flags = []string{"SPLAY"}
		case 2:
			leg.Flags = []string{"DUPLICATE"}
		case 3:
			leg.Flags = []string{"DUPLICATE", "SPLAY"}
		}
		model.Legs[i] = leg
	}
	return model
}

// Count eligible records per construction group, independently of search and
// the production predicate. Every within-group pair matches exactly; no
// between-group pair can match. This also gives the expected exclusion union.
func qualityBenchmarkExpected(n, repeats int, dense, excludeSplays, includeDuplicates bool) (selected, pairs int) {
	groupCounts := make(map[int]int)
	for i := 0; i < n; i++ {
		if excludeSplays && i%2 == 1 || !includeDuplicates && i%4 >= 2 {
			continue
		}
		selected++
		group := i / repeats
		if dense {
			group = 0
		}
		pairs += groupCounts[group]
		groupCounts[group]++
	}
	return selected, pairs
}

func BenchmarkSearch(b *testing.B) {
	for _, dataset := range []struct {
		name       string
		n, repeats int
		dense      bool
	}{
		{"sparse_50000", 50000, 1, false}, {"sparse_100000", 100000, 1, false},
		{"repeated_50000", 50000, 4, false}, {"repeated_100000", 100000, 4, false},
		{"dense_128", 128, 128, true},
	} {
		b.Run(dataset.name+"/generator_v1_no_rng", func(b *testing.B) {
			model := qualityBenchmarkDataset(dataset.n, dataset.repeats, dataset.dense)
			for _, setting := range []struct {
				mode, value                      string
				excludeSplays, includeDuplicates bool
				batch                            int
			}{
				{"relative_percent", "5", false, false, 1},
				{"relative_percent", "5", false, true, 7},
				{"absolute_m", "0.05", true, false, 1},
				{"absolute_m", "0.05", true, true, 7},
			} {
				name := fmt.Sprintf("%s_%s/exclude_splays_%t/include_duplicates_%t/batch_%d", setting.mode, setting.value, setting.excludeSplays, setting.includeDuplicates, setting.batch)
				b.Run(name, func(b *testing.B) {
					tolerance, err := NewTolerance(setting.mode, setting.value)
					if err != nil {
						b.Fatal(err)
					}
					selected, wantPairs := qualityBenchmarkExpected(dataset.n, dataset.repeats, dataset.dense, setting.excludeSplays, setting.includeDuplicates)
					// Qualify a small construction prefix against the independent
					// rational oracle before measuring the large workload.
					small := &Model{Legs: model.Legs[:min(16, len(model.Legs))]}
					value, ok := new(big.Rat).SetString(setting.value)
					if !ok {
						b.Fatal("invalid fixed rational")
					}
					var smallPairs []exactTuple
					if err := NewSearch(small, tolerance, setting.excludeSplays, setting.includeDuplicates).Pairs(setting.batch, func(pair Pair) error {
						smallPairs = append(smallPairs, exactTuple{pair.A, pair.B, pair.Match.Reversed, pair.Match.E1.String(), pair.Match.E2.String()})
						return nil
					}); err != nil {
						b.Fatal(err)
					}
					if want := exhaustiveOracle(small, setting.mode, value, setting.excludeSplays, setting.includeDuplicates); !reflect.DeepEqual(smallPairs, want) {
						b.Fatalf("small benchmark construction differs from independent oracle: got %v want %v", smallPairs, want)
					}
					search := NewSearch(model, tolerance, setting.excludeSplays, setting.includeDuplicates)
					count := 0
					if err := search.Pairs(setting.batch, func(Pair) error { count++; return nil }); err != nil {
						b.Fatal(err)
					}
					if search.NInput != dataset.n || search.N != selected || search.NExcluded != dataset.n-selected || count != wantPairs || search.Matches != wantPairs {
						b.Fatalf("construction mismatch: input=%d selected=%d excluded=%d K=%d callback=%d; want selected=%d K=%d", search.NInput, search.N, search.NExcluded, search.Matches, count, selected, wantPairs)
					}
					metrics := func(b *testing.B) {
						b.ReportMetric(float64(dataset.n), "input_legs/op")
						b.ReportMetric(float64(selected), "selected_legs/op")
						b.ReportMetric(float64(dataset.n-selected), "excluded_legs/op")
						b.ReportMetric(float64(wantPairs), "pairs/op")
						b.ReportMetric(float64(search.Candidates), "candidates/op")
					}
					b.Run("Build", func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							qualityBenchmarkSearch = NewSearch(model, tolerance, setting.excludeSplays, setting.includeDuplicates)
						}
						metrics(b)
					})
					b.Run("Pairs", func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							if err := search.Pairs(setting.batch, func(Pair) error { return nil }); err != nil {
								b.Fatal(err)
							}
						}
						metrics(b)
					})
				})
			}
		})
	}
}
