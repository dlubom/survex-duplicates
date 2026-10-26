package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestClassificationRequiresAnActualFailingTest(t *testing.T) {
	for _, tc := range []struct {
		name, output, want string
		exit               int
		timedOut           bool
	}{
		{"pass", `{"Action":"pass","Package":"example"}`, "survived", 0, false},
		{"test failure", `{"Action":"fail","Package":"example","Test":"TestBoundary"}`, "killed", 1, false},
		{"package failure only", `{"Action":"fail","Package":"example"}`, "invalid", 1, false},
		{"compiler failure", "{\"Action\":\"fail\",\"Test\":\"TestOther\"}\n{\"Action\":\"build-fail\",\"Package\":\"broken\"}", "invalid", 1, false},
		{"legacy compiler failure", `{"Action":"output","Output":"FAIL example [build failed]\n"}`, "invalid", 1, false},
		{"deadline", `{"Action":"fail","Test":"TestBoundary"}`, "timeout", 1, true},
		{"go test timeout", `{"Action":"output","Output":"panic: test timed out after 90s\n"}`, "timeout", 1, false},
		{"launch error", "", "invalid", -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := classify([]byte(tc.output), tc.exit, tc.timedOut)
			if status != tc.want {
				t.Fatalf("status = %q, want %q", status, tc.want)
			}
		})
	}
	_, failures := classify([]byte("{\"Action\":\"fail\",\"Test\":\"TestB\"}\n{\"Action\":\"fail\",\"Test\":\"TestA\"}\n{\"Action\":\"fail\",\"Test\":\"TestB\"}"), 1, false)
	if !reflect.DeepEqual(failures, []string{"TestA", "TestB"}) {
		t.Fatalf("failures = %v", failures)
	}
}

func TestReplacementRequiresExactlyOneOccurrenceAndRestoresBytes(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "source.go")
	original := []byte("prefix\nunique target\nsuffix\n")
	if err := os.WriteFile(name, original, 0755); err != nil {
		t.Fatal(err)
	}
	restore, err := replaceMutation(root, mutation{Name: "one", File: "source.go", Before: "unique target", After: "changed"})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(name); string(b) != "prefix\nchanged\nsuffix\n" {
		t.Fatalf("mutated bytes = %q", b)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(name); string(b) != string(original) {
		t.Fatalf("restored bytes = %q", b)
	}
	if s, _ := os.Stat(name); runtime.GOOS != "windows" && s.Mode().Perm() != 0755 {
		t.Fatalf("mode = %v", s.Mode())
	}
	for _, before := range []string{"absent", "\n", ""} {
		if _, err := replaceMutation(root, mutation{File: "source.go", Before: before, After: "changed"}); !errors.Is(err, errStale) {
			t.Fatalf("replacement %q: %v", before, err)
		}
		if b, _ := os.ReadFile(name); string(b) != string(original) {
			t.Fatalf("stale mutation edited file: %q", b)
		}
	}
	for _, file := range []string{"../outside", "/absolute", ".git/config"} {
		if _, err := replaceMutation(root, mutation{File: file, Before: "x", After: "y"}); err == nil {
			t.Fatalf("unsafe path %q accepted", file)
		}
	}
}

func gitFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{".gitignore": "ignored\n", "tracked": "original\n", "deleted": "old\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	return root
}

func TestSnapshotCopiesWorkingTreeWithoutOriginalGitOrIgnoredFiles(t *testing.T) {
	root := gitFixture(t)
	for name, content := range map[string]string{"tracked": "edited\n", "new source.go": "new\n", "ignored": "private\n", ".git/private-marker": "private history\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(root, "deleted")); err != nil {
		t.Fatal(err)
	}
	copyRoot, err := snapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(copyRoot) })
	if copyRoot == root {
		t.Fatal("snapshot reused original checkout")
	}
	for name, want := range map[string]string{"tracked": "edited\n", "new source.go": "new\n"} {
		if b, err := os.ReadFile(filepath.Join(copyRoot, name)); err != nil || string(b) != want {
			t.Fatalf("%s = %q, %v", name, b, err)
		}
	}
	for _, name := range []string{"ignored", "deleted", ".git/private-marker"} {
		if _, err := os.Stat(filepath.Join(copyRoot, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("snapshot contains %s: %v", name, err)
		}
	}
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = copyRoot
	if b, err := cmd.Output(); err != nil || len(b) != 0 {
		t.Fatalf("snapshot is not independently committed: %q %v", b, err)
	}
	if err := os.WriteFile(filepath.Join(copyRoot, "tracked"), []byte("mutant"), 0644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "tracked")); string(b) != "edited\n" {
		t.Fatal("snapshot modified original")
	}
}

func TestSnapshotKeepsOriginallyTrackedIgnoredFileInItsInventory(t *testing.T) {
	root := gitFixture(t)
	if err := os.WriteFile(filepath.Join(root, "ignored"), []byte("tracked despite ignore\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "-f", "ignored")
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add ignored fixture: %v %s", err, b)
	}
	copyRoot, err := snapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(copyRoot) })
	cmd = exec.Command("git", "ls-files", "--error-unmatch", "ignored")
	cmd.Dir = copyRoot
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("snapshot lost tracked ignored source: %v %s", err, b)
	}
}

func TestSnapshotRejectsSymlinkAndSymlinkParent(t *testing.T) {
	for _, parent := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "parent"}[parent], func(t *testing.T) {
			root := gitFixture(t)
			external := t.TempDir()
			if parent {
				if err := os.Mkdir(filepath.Join(root, "dir"), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "dir", "child"), []byte("tracked"), 0644); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("git", "add", "dir/child")
				cmd.Dir = root
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git add: %v %s", err, output)
				}
				if err := os.RemoveAll(filepath.Join(root, "dir")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, filepath.Join(root, "dir")); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("symlink privilege unavailable: %v", err)
					}
					t.Fatal(err)
				}
			} else if err := os.Symlink(external, filepath.Join(root, "link")); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("symlink privilege unavailable: %v", err)
				}
				t.Fatal(err)
			}
			if _, err := snapshot(context.Background(), root); err == nil {
				t.Fatal("snapshot accepted symlink")
			}
		})
	}
}

func TestSuiteUsesRealGoJSONAndSavesLog(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"go.mod": "module mutationfixture\n\ngo 1.26\n", "fixture_test.go": "package fixture\nimport \"testing\"\nfunc TestFailure(t *testing.T) { t.Fatal(\"expected failure\") }\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(t.TempDir(), "run.log")
	r := runSuite(context.Background(), root, log, 90*time.Second, 2*time.Minute)
	if r.Status != "killed" || r.ExitCode != 1 || !reflect.DeepEqual(r.FailingTests, []string{"TestFailure"}) {
		t.Fatalf("result = %+v", r)
	}
	if b, err := os.ReadFile(log); err != nil || !strings.Contains(string(b), "expected failure") {
		t.Fatalf("log = %q %v", b, err)
	}
}

func TestCatalogueRejectsUnsafePathsBeforeCreatingAReport(t *testing.T) {
	for _, file := range []string{"/absolute", "../outside", ".git/config", "a/../../outside", "a//b", `a\b`} {
		path := filepath.Join(t.TempDir(), "mutations.json")
		content := fmt.Sprintf(`{"schema_version":1,"mutations":[{"name":"unsafe","file":%q,"before":"a","after":"b"}]}`, file)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadCatalogue(path); err == nil {
			t.Fatalf("accepted unsafe report path %q", file)
		}
	}
}

func TestRunnerRestoresBetweenMutationsAndReportsNonKills(t *testing.T) {
	root := gitFixture(t)
	for name, content := range map[string]string{
		"go.mod":         "module mutationfixture\n\ngo 1.26\n",
		"answer.go":      "package fixture\nfunc answer() int { return 42 }\n",
		"answer_test.go": "package fixture\nimport \"testing\"\nfunc TestAnswer(t *testing.T) { if answer() != 42 { t.Fatal(\"wrong answer\") } }\n",
		"mutations.json": `{"schema_version":1,"mutations":[
		{"name":"wrong_answer","file":"answer.go","before":"return 42","after":"return 0"},
		{"name":"same_answer","file":"answer.go","before":"return 42","after":"return 6 * 7"},
		{"name":"compile_error","file":"answer.go","before":"return 42","after":"return missing"},
		{"name":"stale","file":"answer.go","before":"return 999","after":"return 0"}]}`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	if status := run([]string{"--root", root, "--mutations", "mutations.json", "--out", out}, &stdout, &stderr); status != 1 {
		t.Fatalf("status = %d, output %s %s", status, &stdout, &stderr)
	}
	data, err := os.ReadFile(filepath.Join(out, "results.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Baseline struct {
			Status string `json:"status"`
		} `json:"baseline"`
		Results []struct{ Name, File, Status, Log string } `json:"results"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Baseline.Status != "survived" {
		t.Fatalf("baseline = %+v; stderr %s", got.Baseline, &stderr)
	}
	var statuses []string
	for _, r := range got.Results {
		statuses = append(statuses, r.Status)
		if filepath.IsAbs(r.File) || filepath.IsAbs(r.Log) {
			t.Fatalf("absolute report path: %+v", r)
		}
		if _, err := os.Stat(filepath.Join(out, r.Log)); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(statuses, []string{"killed", "survived", "invalid", "stale"}) {
		t.Fatalf("statuses = %v; stderr %s", statuses, &stderr)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "answer.go")); string(b) != "package fixture\nfunc answer() int { return 42 }\n" {
		t.Fatalf("original source changed: %q", b)
	}
	if strings.Contains(string(data), root) || strings.Contains(string(data), "survex-mutations-") {
		t.Fatalf("report leaked absolute source or snapshot path: %s", data)
	}
}

func TestSuiteCannotRedirectGitWritesOutsideSnapshot(t *testing.T) {
	external := gitFixture(t)
	index := filepath.Join(external, ".git", "index")
	original, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "tracked"), []byte("unstaged external change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root := gitFixture(t)
	for name, content := range map[string]string{
		"go.mod": "module mutationfixture\n\ngo 1.26\n",
		"git_test.go": `package fixture
import ("os"; "os/exec"; "strings"; "testing")
func TestGitIsolation(t *testing.T) {
 if b, err := exec.Command("git", "add", "--all").CombinedOutput(); err != nil { t.Fatalf("git add: %v %s", err, b) }
 for _, e := range os.Environ() { if strings.HasPrefix(e, "GIT_") { t.Fatalf("inherited Git override: %s", e) } }
}

`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GIT_DIR", filepath.Join(external, ".git"))
	t.Setenv("GIT_WORK_TREE", external)
	t.Setenv("GIT_INDEX_FILE", index)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "user.name")
	t.Setenv("GIT_CONFIG_VALUE_0", "Redirected Test")
	r := runSuite(context.Background(), root, filepath.Join(t.TempDir(), "suite.log"), 90*time.Second, 2*time.Minute)
	if r.Status != "survived" {
		t.Errorf("suite inherited external Git configuration: %+v", r)
	}
	if got, err := os.ReadFile(index); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("external Git index changed: %v", err)
	}
}

func TestGitEnvironmentRemovesOverridesWithWindowsKeyCasing(t *testing.T) {
	got := sanitizedGitEnvironment([]string{"PATH=bin", "git_dir=outside", "GiT_InDeX_FiLe=outside-index", "GIT_CONFIG_COUNT=1", "GOCACHE=cache", "NOT_GIT_DIR=keep"})
	want := []string{"PATH=bin", "GOCACHE=cache", "NOT_GIT_DIR=keep"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
}
