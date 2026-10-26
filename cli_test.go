package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIContract(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "model.dump"), filepath.Join(dir, "result.csv")
	body := "TITLE \"CLI\"\nDATE \"@0\"\nDATE_NUMERIC 0\nVERSION 8\nSEPARATOR '.'\n--\nLEG 0 0 0 1 0 0 []\nLEG 1 0 0 0 0 0 []\nSTOP\n"
	if err := os.WriteFile(in, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{in, "--out", out}, &stdout, &stderr)
	if code != 0 || stdout.Len() != 0 {
		t.Fatalf("code %d stdout %s stderr %s", code, &stdout, &stderr)
	}
	var summary map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary["K"] != float64(1) || summary["n_input"] != float64(2) || summary["tolerance_value"] != "5" {
		t.Fatal(summary)
	}
	original, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := RunCLI([]string{in, "--out", out}, &stdout, &stderr); code != 1 {
		t.Fatalf("existing output status %d", code)
	}
	if err := os.WriteFile(in, []byte("truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := RunCLI([]string{in, "--out", out, "--force"}, &stdout, &stderr); code != 1 {
		t.Fatalf("invalid input status %d", code)
	}
	after, _ := os.ReadFile(out)
	if !bytes.Equal(after, original) {
		t.Fatal("failed run changed prior output")
	}
}

func TestCLIArgumentStatusAndHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"x", "--out", "y", "--epsilon", "0"}, {"x", "--out", "y", "--epsilon", "NaN"}, {"x", "--out", "y", "--relative-percent", "-5"}} {
		var out, err bytes.Buffer
		if code := RunCLI(args, &out, &err); code != 2 || out.Len() != 0 || err.Len() == 0 {
			t.Fatalf("args %v status %d stdout %s stderr %s", args, code, &out, &err)
		}
	}
	var out, err bytes.Buffer
	if code := RunCLI([]string{"--help"}, &out, &err); code != 0 || !strings.Contains(out.String(), "5%") || !strings.Contains(out.String(), "adjustment") {
		t.Fatalf("help %d %s %s", code, &out, &err)
	}
}

func TestCLIRejectsInvalidRepeatedToleranceBeforeReadingInput(t *testing.T) {
	for _, option := range []string{"--epsilon", "--relative-percent"} {
		var out, err bytes.Buffer
		if code := RunCLI([]string{"missing", "--out", "unused", option, "NaN", option, "5"}, &out, &err); code != 2 {
			t.Fatalf("invalid repeated %s: status %d: %s", option, code, &err)
		}
	}
}

func TestCLIVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if status := RunCLI([]string{"--version"}, &stdout, &stderr); status != 0 {
		t.Fatalf("version status %d: %s", status, &stderr)
	}
	if stdout.String() != "survex-duplicates dev (unknown)\n" || stderr.Len() != 0 {
		t.Fatalf("version stdout %q, stderr %q", &stdout, &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	if status := RunCLI([]string{"--version=value"}, &stdout, &stderr); status != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("assigned version status %d, stdout %q, stderr %q", status, &stdout, &stderr)
	}
}
