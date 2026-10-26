package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// This test runs on every native CI platform without requiring dump3d or Python.
// Full CSV equality includes pair order, orientation, both endpoint errors and
// all metadata; counts alone would not establish compatibility.
func TestCLIBundledKatasterReports(t *testing.T) {
	assertKatasterReports(t, filepath.Join("examples", "kataster", "KATASTER.legs.dump"), "dump")
}

func assertKatasterReports(t *testing.T, input, inputFormat string) {
	t.Helper()
	base := filepath.Join("examples", "kataster")
	inputHashes := map[string]string{
		"KATASTER.3d":        "55c9a785bd4494cd4c8e41dbcdfd106eedfd4e415558e7d4cee856a789821077",
		"KATASTER.legs.dump": "845c9bc863f40dd2375c1004c4d02b0293b15134abab7006de60c72addf6f298",
	}
	provenanceBytes, err := os.ReadFile(filepath.Join(base, "provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var provenance struct {
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(provenanceBytes, &provenance); err != nil {
		t.Fatal(err)
	}
	for name, hash := range inputHashes {
		if provenance.SHA256[name] != hash {
			t.Fatalf("provenance changed for %s: establish new expectations independently", name)
		}
		readVerifiedKataster(t, filepath.Join(base, name), hash)
	}
	verificationBytes := readVerifiedKataster(t, filepath.Join(base, "results", "verification.json"),
		"f184f463b221470b51720c417a0b8fa8286c3d24e56d0add03b2ab0ea2f41a93")
	var verification struct {
		Runs []struct {
			CSV               string `json:"csv"`
			CSVHash           string `json:"csv_sha256"`
			NInput            int    `json:"n_input"`
			NExcluded         int    `json:"n_excluded"`
			N                 int    `json:"n"`
			K                 int    `json:"K"`
			ToleranceMode     string `json:"tolerance_mode"`
			ToleranceValue    string `json:"tolerance_value"`
			ToleranceUnit     string `json:"tolerance_unit"`
			ExcludeSplays     bool   `json:"exclude_splays"`
			IncludeDuplicates bool   `json:"include_duplicates"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(verificationBytes, &verification); err != nil {
		t.Fatal(err)
	}
	if len(verification.Runs) != 8 {
		t.Fatalf("expected eight preserved reports, got %d", len(verification.Runs))
	}
	for _, expected := range verification.Runs {
		t.Run(expected.CSV, func(t *testing.T) {
			want := readVerifiedKataster(t, filepath.Join(base, "results", expected.CSV), expected.CSVHash)
			rows, err := csv.NewReader(bytes.NewReader(want)).ReadAll()
			if err != nil || len(rows) != expected.K+1 || len(rows[0]) != 47 {
				t.Fatalf("invalid saved report: rows=%d, error=%v", len(rows), err)
			}
			output := filepath.Join(t.TempDir(), "report.csv")
			args := []string{input, "--input-format", inputFormat, "--out", output}
			if expected.ToleranceMode == "absolute_m" {
				args = append(args, "--epsilon", expected.ToleranceValue)
			} else if expected.ToleranceMode != "relative_percent" || expected.ToleranceValue != "5" {
				t.Fatal("unexpected saved tolerance")
			}
			if expected.ExcludeSplays {
				args = append(args, "--exclude-splays")
			}
			if expected.IncludeDuplicates {
				args = append(args, "--include-duplicates")
			}
			var stdout, stderr bytes.Buffer
			if status := RunCLI(args, &stdout, &stderr); status != 0 || stdout.Len() != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, &stdout, &stderr)
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("complete CSV differs: got SHA-256 %x, want %s", sha256.Sum256(got), expected.CSVHash)
			}
			var summary struct {
				NInput            int    `json:"n_input"`
				NExcluded         int    `json:"n_excluded"`
				N                 int    `json:"n"`
				K                 int    `json:"K"`
				ToleranceMode     string `json:"tolerance_mode"`
				ToleranceValue    string `json:"tolerance_value"`
				ToleranceUnit     string `json:"tolerance_unit"`
				ExcludeSplays     bool   `json:"exclude_splays"`
				IncludeDuplicates bool   `json:"include_duplicates"`
				InputFormat       string `json:"input_format"`
			}
			if err := json.Unmarshal(stderr.Bytes(), &summary); err != nil {
				t.Fatal(err)
			}
			if summary.NInput != expected.NInput || summary.NExcluded != expected.NExcluded ||
				summary.N != expected.N || summary.K != expected.K || summary.ToleranceMode != expected.ToleranceMode ||
				summary.ToleranceValue != expected.ToleranceValue || summary.ToleranceUnit != expected.ToleranceUnit ||
				summary.ExcludeSplays != expected.ExcludeSplays || summary.IncludeDuplicates != expected.IncludeDuplicates ||
				summary.InputFormat != inputFormat {
				t.Fatalf("unexpected CLI summary: %+v", summary)
			}
		})
	}
	for name, hash := range inputHashes {
		readVerifiedKataster(t, filepath.Join(base, name), hash)
	}
}

func readVerifiedKataster(t *testing.T, path, expectedHash string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != expectedHash {
		t.Fatalf("snapshot hash mismatch: %s", path)
	}
	return data
}
