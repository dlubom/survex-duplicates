package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Real files retain OS publication and cleanup behavior; only the failing
// boundary is replaced. A short write models a disk filling mid-record.
type reportFaultFile struct {
	*os.File
	writeErr, syncErr, closeErr error
	synced, closed              bool
}

func (f *reportFaultFile) Write(p []byte) (int, error) {
	f.synced = false
	if f.writeErr != nil {
		n, err := f.File.Write(p[:min(17, len(p))])
		if err != nil {
			return n, err
		}
		return n, f.writeErr
	}
	return f.File.Write(p)
}

func (f *reportFaultFile) Sync() error {
	if f.syncErr != nil {
		return f.syncErr
	}
	if err := f.File.Sync(); err != nil {
		return err
	}
	f.synced = true
	return nil
}

func (f *reportFaultFile) Close() error {
	f.closed = true
	err := f.File.Close()
	if f.closeErr != nil {
		return f.closeErr
	}
	return err
}

func reportFaultOperations(file **reportFaultFile, configure func(*reportFaultFile)) reportFileOperations {
	ops := osReportFileOperations()
	ops.createTemp = func(dir, pattern string) (reportFile, error) {
		real, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		*file = &reportFaultFile{File: real}
		if configure != nil {
			configure(*file)
		}
		return *file, nil
	}
	return ops
}

func reportFailureInput(t *testing.T, dir string, count int, survey string) string {
	t.Helper()
	var body strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&body, "LEG 0 0 0 1 0 0 [%s]\n", survey)
	}
	input := filepath.Join(dir, "source.dump")
	if err := os.WriteFile(input, []byte(readerFullDump(body.String())), 0600); err != nil {
		t.Fatal(err)
	}
	return input
}

func assertReportFailureFiles(t *testing.T, dir, output string, old bool) {
	t.Helper()
	data, err := os.ReadFile(output)
	if old {
		if err != nil || string(data) != "old report" {
			t.Fatalf("previous report damaged: %q, %v", data, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed report became visible: %q, %v", data, err)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary reports remain: %v, %v", temps, err)
	}
}

// Removing an error guard must not turn a partial, unflushed, unsynced or
// unclosed report into visible output or a CLI success summary.
func TestReportIOFailuresReachCLIAndPreserveOutput(t *testing.T) {
	injected := errors.New("injected report disk failure")
	cases := []struct {
		name      string
		count     int
		survey    string
		configure func(*reportFaultFile)
		operation string
	}{
		{"create", 2, "", nil, "create"},
		{"final-flush", 2, "", func(f *reportFaultFile) { f.writeErr = injected }, ""},
		{"stream-buffer-flush", 40, "", func(f *reportFaultFile) { f.writeErr = injected }, ""},
		{"stream-large-row", 2, strings.Repeat("survey", 12000), func(f *reportFaultFile) { f.writeErr = injected }, ""},
		{"sync", 2, "", func(f *reportFaultFile) { f.syncErr = injected }, ""},
		{"close", 2, "", func(f *reportFaultFile) { f.closeErr = injected }, ""},
		{"publication", 2, "", nil, "publish"},
	}
	for _, tc := range cases {
		for _, destination := range []string{"new-no-force", "new-force", "existing-force"} {
			t.Run(tc.name+"/"+destination, func(t *testing.T) {
				dir := t.TempDir()
				input := reportFailureInput(t, dir, tc.count, tc.survey)
				output := filepath.Join(dir, "report.csv")
				old := destination == "existing-force"
				if old {
					if err := os.WriteFile(output, []byte("old report"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				var file *reportFaultFile
				ops := reportFaultOperations(&file, tc.configure)
				if tc.operation == "create" {
					ops.createTemp = func(string, string) (reportFile, error) { return nil, injected }
				}
				if tc.operation == "publish" {
					ops.link = func(string, string) error { return injected }
					ops.rename = func(string, string) error { return injected }
				}
				var search *Search
				writer := func(path string, model *Model, s *Search, tol *Tolerance, batch int, force bool) error {
					search = s
					return writeReportWithFiles(path, model, s, tol, batch, force, ops)
				}
				args := []string{input, "--out", output}
				if destination != "new-no-force" {
					args = append(args, "--force")
				}
				var stdout, stderr bytes.Buffer
				status := runCLI(args, &stdout, &stderr, writer)
				if status != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), injected.Error()) || strings.Contains(stderr.String(), `"K"`) {
					t.Fatalf("failure reported as completion: status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
				}
				assertReportFailureFiles(t, dir, output, old)
				if file != nil && !file.closed {
					t.Fatal("failed output file was not closed")
				}
				if tc.name == "stream-buffer-flush" && (search.Matches == 0 || search.Matches >= 780) {
					t.Fatalf("write failure did not stop streaming before all 780 pairs: %d", search.Matches)
				}
				if tc.name == "final-flush" && search.Matches != 1 {
					t.Fatalf("expected final flush after the one pair, got %d", search.Matches)
				}
			})
		}
	}
}

// The destination appears after CLI validation but before no-clobber publish.
// Replacing Link with Rename would overwrite this concurrent writer's result.
func TestReportNoClobberRacePreservesConcurrentOutput(t *testing.T) {
	dir := t.TempDir()
	input := reportFailureInput(t, dir, 2, "")
	output := filepath.Join(dir, "report.csv")
	ops := osReportFileOperations()
	ops.link = func(source, destination string) error {
		if err := os.WriteFile(destination, []byte("old report"), 0600); err != nil {
			return err
		}
		return os.Link(source, destination)
	}
	writer := func(path string, model *Model, s *Search, tol *Tolerance, batch int, force bool) error {
		return writeReportWithFiles(path, model, s, tol, batch, force, ops)
	}
	var stdout, stderr bytes.Buffer
	if status := runCLI([]string{input, "--out", output}, &stdout, &stderr, writer); status != 1 || !strings.Contains(stderr.String(), "Error:") || strings.Contains(stderr.String(), `"K"`) {
		t.Fatalf("no-clobber race reported as success: status=%d stderr=%q", status, stderr.String())
	}
	assertReportFailureFiles(t, dir, output, true)
}

// A durability-aware publication boundary rejects bytes which were not synced
// and closed. This catches removing Sync or moving publication before it.
func TestReportPublishesOnlySyncedClosedCompleteCSV(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprint("force=", force), func(t *testing.T) {
			var file *reportFaultFile
			ops := reportFaultOperations(&file, nil)
			publish := func(source, destination string) error {
				if !file.synced || !file.closed {
					return errors.New("attempt to publish undurable or open report")
				}
				data, err := os.ReadFile(source)
				if err != nil || !bytes.HasSuffix(data, []byte("\r\n")) || bytes.Count(data, []byte("\r\n")) != 2 {
					return fmt.Errorf("attempt to publish incomplete CSV: %v", err)
				}
				if force {
					return os.Rename(source, destination)
				}
				return os.Link(source, destination)
			}
			ops.link, ops.rename = publish, publish
			model := &Model{Legs: []Leg{{ID: 1, B: Point{100, 0, 0}}, {ID: 2, B: Point{100, 0, 0}}}}
			tol, err := NewTolerance("relative_percent", "5")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			output := filepath.Join(dir, "report.csv")
			if err := writeReportWithFiles(output, model, NewSearch(model, tol, false, false), tol, 1, force, ops); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil || !bytes.Contains(data, []byte("\r\n1,2,")) {
				t.Fatalf("complete report not published: %q, %v", data, err)
			}
			temps, err := filepath.Glob(filepath.Join(dir, ".*.tmp"))
			if err != nil || len(temps) != 0 {
				t.Fatalf("successful publication left temporary files: %v, %v", temps, err)
			}
		})
	}
}
