package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestFixtureSnapshotsAreUnchanged(t *testing.T) {
	data := readVerifiedKataster(t, filepath.Join("tests", "fixtures", "provenance.json"),
		"282ad9665dc31ce37f013369cfb262c522021c5f41dfd220c1a513f9951fd11a")
	var provenance struct {
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(data, &provenance); err != nil {
		t.Fatal(err)
	}
	if len(provenance.SHA256) != 11 {
		t.Fatalf("expected eleven preserved source, model and dump files, got %d", len(provenance.SHA256))
	}
	for name, hash := range provenance.SHA256 {
		readVerifiedKataster(t, filepath.Join("tests", "fixtures", name), hash)
	}
}

func TestCLIKatasterBinaryReports(t *testing.T) {
	if _, err := exec.LookPath("dump3d"); err != nil {
		t.Skip("real binary integration requires Survex dump3d")
	}
	input := os.Getenv("KATASTER_PATH")
	if input == "" {
		input = filepath.Join("examples", "kataster", "KATASTER.3d")
	}
	if _, err := os.Stat(input); os.IsNotExist(err) {
		t.Skip("optional KATASTER binary is not available")
	} else if err != nil {
		t.Fatal(err)
	}
	const hash = "55c9a785bd4494cd4c8e41dbcdfd106eedfd4e415558e7d4cee856a789821077"
	readVerifiedKataster(t, input, hash)
	assertKatasterReports(t, input, "3d")
	readVerifiedKataster(t, input, hash)
}

func TestKataster512SampleMatchesIndependentOracle(t *testing.T) {
	data := readVerifiedKataster(t, filepath.Join("examples", "kataster", "KATASTER.legs.dump"),
		"845c9bc863f40dd2375c1004c4d02b0293b15134abab7006de60c72addf6f298")
	registry, err := ParseDump(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	// Retain the known repeated-survey pairs, then sample additional original
	// record indexes with Go's seeded generator. This is a new reproducible
	// Go test sample, not a claim to reproduce Python's random-number sequence.
	selected := map[int]bool{2835: true, 14208: true, 2836: true, 14227: true,
		1447: true, 7947: true, 1448: true, 7948: true}
	rng := rand.New(rand.NewSource(20261025))
	for len(selected) < 512 {
		selected[rng.Intn(len(registry.Legs))] = true
	}
	indexes := make([]int, 0, len(selected))
	for index := range selected {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	model := &Model{}
	for _, index := range indexes {
		model.Legs = append(model.Legs, registry.Legs[index])
	}
	// Map both complete tuple lists to preserved source IDs; compact subset
	// positions and filtered positions are not report identities.
	sourceIDs := func(pairs []exactTuple) []exactTuple {
		result := make([]exactTuple, len(pairs))
		for i, pair := range pairs {
			pair.a, pair.b = model.Legs[pair.a].ID, model.Legs[pair.b].ID
			result[i] = pair
		}
		return result
	}
	for _, mode := range []struct {
		name, value string
		exact       *big.Rat
	}{
		{"relative_percent", "5", big.NewRat(5, 1)},
		{"absolute_m", ".05", big.NewRat(1, 20)},
	} {
		// Evaluate every geometry pair once per mode with the independent
		// rational oracle. Filtering this complete list avoids eight exhaustive
		// geometry passes without sharing the production selection function.
		all := exhaustiveOracle(model, mode.name, mode.exact, false, true)
		if len(all) == 0 {
			t.Fatal("the deliberate repeated-survey sample has no oracle matches")
		}
		for _, exclude := range []bool{false, true} {
			for _, include := range []bool{false, true} {
				name := fmt.Sprintf("%s/exclude_splays=%v/include_duplicates=%v", mode.name, exclude, include)
				t.Run(name, func(t *testing.T) {
					eligible := func(index int) bool {
						for _, flag := range model.Legs[index].Flags {
							if (flag == "SPLAY" && exclude) || (flag == "DUPLICATE" && !include) {
								return false
							}
						}
						return true
					}
					var want []exactTuple
					for _, pair := range all {
						if eligible(pair.a) && eligible(pair.b) {
							want = append(want, pair)
						}
					}
					want = sourceIDs(want)
					selectedCount := 0
					for index := range model.Legs {
						if eligible(index) {
							selectedCount++
						}
					}
					search := NewSearch(model, mustTolerance(t, mode.name, mode.value), exclude, include)
					if search.NInput != 512 || search.N != selectedCount || search.NExcluded != 512-selectedCount {
						t.Fatalf("incorrect source/selection counts: %+v", search)
					}
					for _, batch := range []int{1, 17} {
						got := sourceIDs(collectedPairs(t, search, batch))
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("batch=%d complete source-ID, orientation and endpoint-error tuples differ\ngot: %v\nwant: %v", batch, got, want)
						}
					}
				})
			}
		}
	}
}

func TestCLIRejectedInputDoesNotPublish(t *testing.T) {
	for _, body := range []string{
		"LEG 0.001 0 0 1 0 0 [] SPLAY", "LEG 0 0 bad 1 0 0 [] SPLAY",
		"LEG 0.001 0 0 1 0 0 [] DUPLICATE", "LEG 0 0 bad 1 0 0 [] DUPLICATE",
	} {
		t.Run(body, func(t *testing.T) {
			input := filepath.Join(t.TempDir(), "invalid.dump")
			if err := os.WriteFile(input, []byte(readerFullDump(body)), 0600); err != nil {
				t.Fatal(err)
			}
			assertCLIInputFailurePreservesOutput(t, input, "line 7", "--exclude-splays")
		})
	}
	for _, mode := range []string{"missing", "exit-empty", "exit-valid"} {
		t.Run(mode, func(t *testing.T) {
			program, diagnostic := filepath.Join(t.TempDir(), "missing-dump3d"), "dump3d not found"
			if mode != "missing" {
				program, diagnostic = readerProcessHelper(t, mode), "code 9"
			}
			assertCLIInputFailurePreservesOutput(t, readerFixture(t, "survey.3d"), diagnostic, "--dump3d", program)
		})
	}
}

func assertCLIInputFailurePreservesOutput(t *testing.T, input, diagnostic string, options ...string) {
	t.Helper()
	for _, existing := range []bool{false, true} {
		dir := t.TempDir()
		output := filepath.Join(dir, "report.csv")
		args := append([]string{input, "--out", output}, options...)
		if existing {
			if err := os.WriteFile(output, []byte("previous report"), 0600); err != nil {
				t.Fatal(err)
			}
			args = append(args, "--force")
		}
		var stdout, stderr bytes.Buffer
		if status := RunCLI(args, &stdout, &stderr); status != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), diagnostic) {
			t.Fatalf("existing=%v status=%d stdout=%q stderr=%q", existing, status, &stdout, &stderr)
		}
		files, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if existing {
			data, err := os.ReadFile(output)
			if err != nil || string(data) != "previous report" || len(files) != 1 {
				t.Fatalf("failed input changed prior report or left temporary files: %q, %v, %v", data, files, err)
			}
		} else if len(files) != 0 {
			t.Fatalf("failed input published output or left temporary files: %v", files)
		}
	}
}

func TestCLIRealInputFormatsProduceEquivalentReports(t *testing.T) {
	for _, mode := range []struct{ name, option, value string }{
		{"relative", "--relative-percent", "5"}, {"absolute", "--epsilon", ".05"},
	} {
		for _, exclude := range []bool{false, true} {
			for _, include := range []bool{false, true} {
				name := fmt.Sprintf("%s/exclude_splays=%v/include_duplicates=%v", mode.name, exclude, include)
				t.Run(name, func(t *testing.T) {
					var expected [][]string
					for _, format := range []string{"survey.legs.dump", "survey.lines.dump", "survey.3d"} {
						t.Run(format, func(t *testing.T) {
							if format == "survey.3d" {
								if _, err := exec.LookPath("dump3d"); err != nil {
									t.Skip("real binary integration requires Survex dump3d")
								}
							}
							output := filepath.Join(t.TempDir(), "report.csv")
							args := []string{readerFixture(t, format), "--out", output, mode.option, mode.value, "--batch-size", "2"}
							if exclude {
								args = append(args, "--exclude-splays")
							}
							if include {
								args = append(args, "--include-duplicates")
							}
							var stdout, stderr bytes.Buffer
							if status := RunCLI(args, &stdout, &stderr); status != 0 || stdout.Len() != 0 {
								t.Fatalf("status=%d stdout=%q stderr=%q", status, &stdout, &stderr)
							}
							data, err := os.ReadFile(output)
							if err != nil {
								t.Fatal(err)
							}
							rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
							if err != nil || len(rows) == 0 || len(rows[0]) != 47 || rows[0][4] != "source_line_a" || rows[0][5] != "source_line_b" {
								t.Fatalf("invalid CSV: %v, %v", rows, err)
							}
							// LEG and MOVE/LINE layouts have different dump line numbers.
							// Every other saved field, including both errors, must agree.
							for _, row := range rows[1:] {
								row[4], row[5] = "", ""
							}
							if expected == nil {
								expected = rows
							} else if !reflect.DeepEqual(rows, expected) {
								t.Fatalf("complete report differs after normalizing dump source lines\ngot: %v\nwant: %v", rows, expected)
							}
						})
					}
				})
			}
		}
	}
}
