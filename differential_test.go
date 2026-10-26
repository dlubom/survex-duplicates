package main

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// This probe is compiled only into the test binary. It keeps inspection and
// collection of complete results out of the production streaming executable.
func TestDifferentialProbe(t *testing.T) {
	request := os.Getenv("SURVEX_PROBE_REQUEST")
	if request == "" {
		t.Skip("used by benchmarks.compare_go")
	}
	data, err := os.ReadFile(request)
	if err != nil {
		t.Fatal(err)
	}
	var requests []struct {
		Input, Mode, Value string
		Exclude, Include   bool
		Batch              int
		Inspect            bool
	}
	if err := json.Unmarshal(data, &requests); err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(os.Getenv("SURVEX_PROBE_OUTPUT"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	encoder := json.NewEncoder(out)
	for _, req := range requests {
		model, err := ReadModel(req.Input, "auto", "dump3d")
		if err != nil {
			t.Fatal(err)
		}
		tolerance, err := NewTolerance(req.Mode, req.Value)
		if err != nil {
			t.Fatal(err)
		}
		search := NewSearch(model, tolerance, req.Exclude, req.Include)
		pairs := make([][]any, 0)
		if err := search.Pairs(req.Batch, func(p Pair) error {
			pairs = append(pairs, []any{model.Legs[p.A].ID, model.Legs[p.B].ID, p.Match.Reversed, p.Match.E1, p.Match.E2})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		type stationRow struct {
			Point   Point
			Station Station
		}
		stations := make([]stationRow, 0, len(model.Stations))
		for p, s := range model.Stations {
			stations = append(stations, stationRow{p, s})
		}
		sort.Slice(stations, func(i, j int) bool {
			for k := 0; k < 3; k++ {
				if stations[i].Point[k] != stations[j].Point[k] {
					return stations[i].Point[k] < stations[j].Point[k]
				}
			}
			return false
		})
		result := map[string]any{"pairs": pairs, "input_format": model.InputFormat, "C": search.Candidates, "n": search.N, "n_excluded": search.NExcluded}
		if req.Inspect {
			result["legs"] = model.Legs
			result["stations"] = stations
			result["header"] = model.Header
		}
		if err := encoder.Encode(result); err != nil {
			t.Fatal(err)
		}
	}
}
